package shop

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	shopdata "thom-server/internal/shop"
)

// ErrStripeNotConfigured is returned when checkout is requested without an API key.
var ErrStripeNotConfigured = errors.New("shop: stripe is not configured")

// StripeClient is the slice of the Stripe API the shop uses. It is an interface
// so handler tests run without network access.
type StripeClient interface {
	CreateCheckoutSession(ctx context.Context, params CheckoutParams) (*CheckoutSession, error)
	RefundPayment(ctx context.Context, paymentIntentID string, idempotencyKey string) error
	ExpireCheckoutSession(ctx context.Context, sessionID string) error
}

// StripeSettings is separate from Config so the handler does not depend on the
// server package.
type StripeSettings struct {
	WebhookSecret string
	TaxEnabled    bool
	ShippingCents int
	SuccessURL    string
	CancelURL     string
}

// CheckoutLine is one line item in the Checkout Session being created.
type CheckoutLine struct {
	Title          string
	UnitPriceCents int
	Quantity       int64
}

// CheckoutParams describes the Checkout Session to create. Currency is shared by
// every line, matching Stripe's single-currency sessions.
type CheckoutParams struct {
	Currency      string
	Lines         []CheckoutLine
	SuccessURL    string
	CancelURL     string
	ShippingCents int
	TaxEnabled    bool
}

// CheckoutSession is the part of a Stripe session the server needs to keep.
type CheckoutSession struct {
	ID  string
	URL string
}

type checkoutRequest struct {
	Items    []checkoutItemRequest `json:"items"`
	ItemID   string                `json:"itemId"`
	Quantity *int                  `json:"quantity"`
}

type checkoutItemRequest struct {
	ItemID   string `json:"itemId"`
	Quantity *int   `json:"quantity"`
}

type checkoutResponse struct {
	SessionID string `json:"sessionId"`
	URL       string `json:"url"`
}

// checkoutConflict explains a 409 so the storefront can tell a buyer what is
// available. Stock is only decremented when a payment lands, so the reported
// count reflects paid inventory, never an in-progress checkout.
type checkoutConflict struct {
	Error     string `json:"error"`
	ItemID    string `json:"itemId"`
	Title     string `json:"title"`
	Available int    `json:"available"`
}

// checkoutLine is a validated request line ready to load a listing for.
type checkoutLine struct {
	itemID   int
	quantity int
}

// checkoutItem pairs a loaded listing with the quantity the buyer requested.
type checkoutItem struct {
	item     *shopdata.Item
	quantity int
}

// WithStripe attaches Stripe so checkout routes work. A nil client leaves the
// storefront read-only and checkout returns 503.
func (h *Handler) WithStripe(client StripeClient, settings StripeSettings) *Handler {
	h.stripe = client
	h.stripeSettings = settings

	return h
}

// Checkout creates a Checkout Session for one or more listings. Stock is not
// taken here: it is decremented only when the payment webhook arrives, so an
// abandoned checkout never blocks a listing. Prices are read from the database,
// never from the request.
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	if h.stripe == nil {
		h.infoLog.Printf("checkout requested but Stripe is not configured")
		h.responder.ClientError(w, http.StatusServiceUnavailable)
		return
	}

	lines, ok := h.decodeCheckoutRequest(w, r)
	if !ok {
		return
	}

	items, ok := h.loadCheckoutItems(w, lines)
	if !ok {
		return
	}

	session, ok := h.createCheckoutSession(w, r, items)
	if !ok {
		return
	}

	order, ok := h.recordPendingOrder(w, r, session, items)
	if !ok {
		return
	}

	h.infoLog.Printf("CREATE_CHECKOUT session=%s items=%d order=%s", session.ID, len(items), order.ID)

	err := h.responder.WriteJSON(w, http.StatusCreated, checkoutResponse{
		SessionID: session.ID,
		URL:       session.URL,
	}, nil)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

// decodeCheckoutRequest reads and validates the requested lines. A cart sends
// items; the legacy single-item fields stay accepted so an older storefront
// keeps working against a newer server. It writes the error response itself.
func (h *Handler) decodeCheckoutRequest(w http.ResponseWriter, r *http.Request) ([]checkoutLine, bool) {
	var request checkoutRequest
	if err := h.responder.DecodeJSON(w, r.Body, &request); err != nil {
		h.infoLog.Printf("failed to decode checkout JSON: %v", err)
		h.responder.BadRequest(w)
		return nil, false
	}

	var items []checkoutItemRequest
	if len(request.Items) > 0 {
		items = request.Items
	} else if strings.TrimSpace(request.ItemID) != "" {
		quantity := 1
		if request.Quantity != nil {
			quantity = *request.Quantity
		}
		items = []checkoutItemRequest{{ItemID: request.ItemID, Quantity: &quantity}}
	}

	if len(items) == 0 {
		h.responder.BadRequest(w)
		return nil, false
	}

	lines := make([]checkoutLine, 0, len(items))
	for _, item := range items {
		itemID, err := strconv.Atoi(strings.TrimSpace(item.ItemID))
		if err != nil || itemID < 1 {
			h.responder.BadRequest(w)
			return nil, false
		}

		quantity := 1
		if item.Quantity != nil {
			quantity = *item.Quantity
		}
		if quantity < 1 {
			h.responder.BadRequest(w)
			return nil, false
		}

		lines = append(lines, checkoutLine{itemID: itemID, quantity: quantity})
	}

	return lines, true
}

// loadCheckoutItems verifies each requested listing is published and has enough
// stock, without taking any. Stock is decremented only on payment, so this read
// never holds a listing for an abandoned checkout. It writes the error response
// itself.
func (h *Handler) loadCheckoutItems(w http.ResponseWriter, lines []checkoutLine) ([]*checkoutItem, bool) {
	items := make([]*checkoutItem, 0, len(lines))
	for _, line := range lines {
		item, err := h.shop.GetItemByID(line.itemID)
		if h.responder.HandleDataError(w, err) {
			return nil, false
		}
		if !item.IsPublished {
			h.responder.NotFound(w)
			return nil, false
		}
		if item.Stock < line.quantity {
			h.infoLog.Printf("checkout for item %d exceeds available stock", line.itemID)
			h.writeStockConflict(w, item)
			return nil, false
		}

		items = append(items, &checkoutItem{item: item, quantity: line.quantity})
	}

	return items, true
}

// createCheckoutSession asks Stripe for a session. Stock is untouched on
// failure, so there is nothing to roll back.
func (h *Handler) createCheckoutSession(w http.ResponseWriter, r *http.Request, items []*checkoutItem) (*CheckoutSession, bool) {
	currency := items[0].item.Currency
	lines := make([]CheckoutLine, 0, len(items))
	for _, ci := range items {
		lines = append(lines, CheckoutLine{
			Title:          ci.item.Title,
			UnitPriceCents: ci.item.PriceCents,
			Quantity:       int64(ci.quantity),
		})
	}

	session, err := h.stripe.CreateCheckoutSession(r.Context(), CheckoutParams{
		Currency:      currency,
		Lines:         lines,
		SuccessURL:    h.stripeSettings.SuccessURL,
		CancelURL:     h.stripeSettings.CancelURL,
		ShippingCents: h.stripeSettings.ShippingCents,
		TaxEnabled:    h.stripeSettings.TaxEnabled,
	})
	if err != nil {
		h.infoLog.Printf("failed to create checkout session: %v", err)
		h.responder.ServerError(w, err)
		return nil, false
	}

	return session, true
}

// recordPendingOrder persists the checkout as a pending order. On failure it
// expires the session so a checkout that could not be recorded can never be
// paid without an order row.
func (h *Handler) recordPendingOrder(w http.ResponseWriter, r *http.Request, session *CheckoutSession, items []*checkoutItem) (*shopdata.Order, bool) {
	currency := items[0].item.Currency
	lines := make([]*shopdata.OrderLine, 0, len(items))
	for _, ci := range items {
		lines = append(lines, &shopdata.OrderLine{
			ItemID:         ci.item.ID,
			Title:          ci.item.Title,
			UnitPriceCents: ci.item.PriceCents,
			Quantity:       ci.quantity,
		})
	}

	order, err := h.shop.InsertPendingOrder(session.ID, &shopdata.Order{
		Currency: currency,
		Lines:    lines,
	})
	if err != nil {
		h.infoLog.Printf("failed to record order for session %s: %v", session.ID, err)
		if expireErr := h.stripe.ExpireCheckoutSession(r.Context(), session.ID); expireErr != nil {
			h.infoLog.Printf("failed to expire session %s: %v", session.ID, expireErr)
		}
		h.responder.ServerError(w, err)
		return nil, false
	}

	return order, true
}

// writeStockConflict writes the 409 body for a line whose stock could not cover
// the requested quantity. Because stock is only decremented on payment, the
// reported count is the item's true paid inventory.
func (h *Handler) writeStockConflict(w http.ResponseWriter, item *shopdata.Item) {
	conflict := checkoutConflict{
		Error:     "insufficient_stock",
		ItemID:    item.ID,
		Title:     item.Title,
		Available: item.Stock,
	}

	if err := h.responder.WriteJSON(w, http.StatusConflict, conflict, nil); err != nil {
		h.responder.ServerError(w, err)
	}
}
