package shop

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	shopdata "thom-server/internal/shop"
)

// reservationHold is how long a checkout keeps its stock reservation before an
// abandoned session releases it.
const reservationHold = 30 * time.Minute

// reservationExpiryBuffer pads the Stripe session expiry past the hold so the
// expires_at value is never below Stripe's 30-minute minimum once clock skew
// and request latency are applied.
const reservationExpiryBuffer = 2 * time.Minute

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
	ExpiresAt     int64
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

// checkoutLine is a validated request line ready to reserve stock for.
type checkoutLine struct {
	itemID   int
	quantity int
}

// reservedItem pairs a loaded listing with the quantity reserved from it.
type reservedItem struct {
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

// Checkout creates a Checkout Session for one or more listings and reserves
// their stock for the hold window, so two buyers cannot pay for the same last
// item. The prices are read from the database, never from the request.
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

	reserved, ok := h.reserveCheckoutStock(w, lines)
	if !ok {
		return
	}

	expiresAt := time.Now().Add(reservationHold + reservationExpiryBuffer)

	session, ok := h.createCheckoutSession(w, r, reserved, expiresAt)
	if !ok {
		return
	}

	order, ok := h.recordPendingOrder(w, r, session, reserved, expiresAt)
	if !ok {
		return
	}

	h.infoLog.Printf("CREATE_CHECKOUT session=%s items=%d order=%s", session.ID, len(reserved), order.ID)

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

// reserveCheckoutStock frees stale holds, loads each listing, and takes its
// stock for the requested quantity. The reservation is all-or-nothing: if any
// line fails, the ones already reserved are restored. It writes the error
// response itself.
func (h *Handler) reserveCheckoutStock(w http.ResponseWriter, lines []checkoutLine) ([]*reservedItem, bool) {
	// Expired holds are freed lazily here so abandoned reservations never
	// strand stock even when the expired webhook is missed.
	if err := h.shop.ReleaseExpiredReservations(time.Now().Unix()); err != nil {
		h.infoLog.Printf("failed to release expired reservations: %v", err)
	}

	reserved := make([]*reservedItem, 0, len(lines))
	for _, line := range lines {
		item, err := h.shop.GetItemByID(line.itemID)
		if h.responder.HandleDataError(w, err) {
			h.restoreReservations(reserved)
			return nil, false
		}
		if !item.IsPublished {
			h.responder.NotFound(w)
			h.restoreReservations(reserved)
			return nil, false
		}

		ok, err := h.shop.DecrementStock(item.ID, line.quantity)
		if err != nil {
			h.infoLog.Printf("failed to reserve stock for item %s: %v", item.ID, err)
			h.responder.ServerError(w, err)
			h.restoreReservations(reserved)
			return nil, false
		}
		if !ok {
			h.infoLog.Printf("checkout for item %d exceeds available stock", line.itemID)
			h.responder.ClientError(w, http.StatusConflict)
			h.restoreReservations(reserved)
			return nil, false
		}

		reserved = append(reserved, &reservedItem{item: item, quantity: line.quantity})
	}

	return reserved, true
}

// createCheckoutSession asks Stripe for a session and restores the reservation
// if that fails.
func (h *Handler) createCheckoutSession(w http.ResponseWriter, r *http.Request, reserved []*reservedItem, expiresAt time.Time) (*CheckoutSession, bool) {
	currency := reserved[0].item.Currency
	lines := make([]CheckoutLine, 0, len(reserved))
	for _, res := range reserved {
		lines = append(lines, CheckoutLine{
			Title:          res.item.Title,
			UnitPriceCents: res.item.PriceCents,
			Quantity:       int64(res.quantity),
		})
	}

	session, err := h.stripe.CreateCheckoutSession(r.Context(), CheckoutParams{
		Currency:      currency,
		Lines:         lines,
		SuccessURL:    h.stripeSettings.SuccessURL,
		CancelURL:     h.stripeSettings.CancelURL,
		ShippingCents: h.stripeSettings.ShippingCents,
		TaxEnabled:    h.stripeSettings.TaxEnabled,
		ExpiresAt:     expiresAt.Unix(),
	})
	if err != nil {
		h.infoLog.Printf("failed to create checkout session: %v", err)
		h.restoreReservations(reserved)
		h.responder.ServerError(w, err)
		return nil, false
	}

	return session, true
}

// recordPendingOrder persists the reservation as an order. On failure it
// restores the reservation and expires the session so a checkout that could
// not be recorded can never be paid without the reservation.
func (h *Handler) recordPendingOrder(w http.ResponseWriter, r *http.Request, session *CheckoutSession, reserved []*reservedItem, expiresAt time.Time) (*shopdata.Order, bool) {
	currency := reserved[0].item.Currency
	lines := make([]*shopdata.OrderLine, 0, len(reserved))
	for _, res := range reserved {
		lines = append(lines, &shopdata.OrderLine{
			ItemID:         res.item.ID,
			Title:          res.item.Title,
			UnitPriceCents: res.item.PriceCents,
			Quantity:       res.quantity,
		})
	}

	order, err := h.shop.InsertPendingOrder(session.ID, &shopdata.Order{
		Currency:      currency,
		StockReserved: 1,
		ExpiresAt:     expiresAt.Unix(),
		Lines:         lines,
	})
	if err != nil {
		h.infoLog.Printf("failed to record order for session %s: %v", session.ID, err)
		h.restoreReservations(reserved)
		if expireErr := h.stripe.ExpireCheckoutSession(r.Context(), session.ID); expireErr != nil {
			h.infoLog.Printf("failed to expire session %s: %v", session.ID, expireErr)
		}
		h.responder.ServerError(w, err)
		return nil, false
	}

	return order, true
}

// restoreReservations best-effort restores the stock several reserved items
// held after a failed checkout.
func (h *Handler) restoreReservations(reserved []*reservedItem) {
	for _, res := range reserved {
		h.restoreReservation(res.item.ID, res.quantity)
	}
}

// restoreReservation best-effort restores the stock a failed checkout reserved.
func (h *Handler) restoreReservation(itemID string, quantity int) {
	if err := h.shop.RestoreStock(itemID, quantity); err != nil {
		h.infoLog.Printf("failed to restore stock for item %s: %v", itemID, err)
	}
}
