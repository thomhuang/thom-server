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

// CheckoutParams describes the single-listing Checkout Session to create.
type CheckoutParams struct {
	ItemID         string
	Title          string
	UnitPriceCents int
	Currency       string
	Quantity       int64
	SuccessURL     string
	CancelURL      string
	ShippingCents  int
	TaxEnabled     bool
	ExpiresAt      int64
}

// CheckoutSession is the part of a Stripe session the server needs to keep.
type CheckoutSession struct {
	ID  string
	URL string
}

type checkoutRequest struct {
	ItemID   string `json:"itemId"`
	Quantity *int   `json:"quantity"`
}

type checkoutResponse struct {
	SessionID string `json:"sessionId"`
	URL       string `json:"url"`
}

// WithStripe attaches Stripe so checkout routes work. A nil client leaves the
// storefront read-only and checkout returns 503.
func (h *Handler) WithStripe(client StripeClient, settings StripeSettings) *Handler {
	h.stripe = client
	h.stripeSettings = settings

	return h
}

// Checkout creates a Checkout Session for one listing and reserves its stock
// for the hold window, so two buyers cannot pay for the same last item. The
// price is read from the database, never from the request.
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	if h.stripe == nil {
		h.infoLog.Printf("checkout requested but Stripe is not configured")
		h.responder.ClientError(w, http.StatusServiceUnavailable)
		return
	}

	itemID, quantity, ok := h.decodeCheckoutRequest(w, r)
	if !ok {
		return
	}

	item, ok := h.reserveCheckoutStock(w, itemID, quantity)
	if !ok {
		return
	}

	expiresAt := time.Now().Add(reservationHold + reservationExpiryBuffer)

	session, ok := h.createCheckoutSession(w, r, item, quantity, expiresAt)
	if !ok {
		return
	}

	order, ok := h.recordPendingOrder(w, r, session, item, quantity, expiresAt)
	if !ok {
		return
	}

	h.infoLog.Printf("CREATE_CHECKOUT session=%s item=%s order=%s", session.ID, item.ID, order.ID)

	err := h.responder.WriteJSON(w, http.StatusCreated, checkoutResponse{
		SessionID: session.ID,
		URL:       session.URL,
	}, nil)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

// decodeCheckoutRequest reads and validates the requested item and quantity. It
// writes the error response itself.
func (h *Handler) decodeCheckoutRequest(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	var request checkoutRequest
	if err := h.responder.DecodeJSON(w, r.Body, &request); err != nil {
		h.infoLog.Printf("failed to decode checkout JSON: %v", err)
		h.responder.BadRequest(w)
		return 0, 0, false
	}

	itemID, err := strconv.Atoi(strings.TrimSpace(request.ItemID))
	if err != nil || itemID < 1 {
		h.responder.BadRequest(w)
		return 0, 0, false
	}

	quantity := 1
	if request.Quantity != nil {
		quantity = *request.Quantity
	}
	if quantity < 1 {
		h.responder.BadRequest(w)
		return 0, 0, false
	}

	return itemID, quantity, true
}

// reserveCheckoutStock frees stale holds, loads the listing, and takes its
// stock for the requested quantity. It writes the error response itself.
func (h *Handler) reserveCheckoutStock(w http.ResponseWriter, itemID, quantity int) (*shopdata.Item, bool) {
	// Expired holds are freed lazily here so abandoned reservations never
	// strand stock even when the expired webhook is missed.
	if err := h.shop.ReleaseExpiredReservations(time.Now().Unix()); err != nil {
		h.infoLog.Printf("failed to release expired reservations: %v", err)
	}

	item, err := h.shop.GetItemByID(itemID)
	if h.responder.HandleDataError(w, err) {
		return nil, false
	}
	if !item.IsPublished {
		h.responder.NotFound(w)
		return nil, false
	}

	ok, err := h.shop.DecrementStock(item.ID, quantity)
	if err != nil {
		h.infoLog.Printf("failed to reserve stock for item %s: %v", item.ID, err)
		h.responder.ServerError(w, err)
		return nil, false
	}
	if !ok {
		h.infoLog.Printf("checkout for item %d exceeds available stock", itemID)
		h.responder.ClientError(w, http.StatusConflict)
		return nil, false
	}

	return item, true
}

// createCheckoutSession asks Stripe for a session and restores the reservation
// if that fails.
func (h *Handler) createCheckoutSession(w http.ResponseWriter, r *http.Request, item *shopdata.Item, quantity int, expiresAt time.Time) (*CheckoutSession, bool) {
	session, err := h.stripe.CreateCheckoutSession(r.Context(), CheckoutParams{
		ItemID:         item.ID,
		Title:          item.Title,
		UnitPriceCents: item.PriceCents,
		Currency:       item.Currency,
		Quantity:       int64(quantity),
		SuccessURL:     h.stripeSettings.SuccessURL,
		CancelURL:      h.stripeSettings.CancelURL,
		ShippingCents:  h.stripeSettings.ShippingCents,
		TaxEnabled:     h.stripeSettings.TaxEnabled,
		ExpiresAt:      expiresAt.Unix(),
	})
	if err != nil {
		h.infoLog.Printf("failed to create checkout session: %v", err)
		h.restoreReservation(item.ID, quantity)
		h.responder.ServerError(w, err)
		return nil, false
	}

	return session, true
}

// recordPendingOrder persists the reservation as an order. On failure it
// restores the reservation and expires the session so a checkout that could
// not be recorded can never be paid without the reservation.
func (h *Handler) recordPendingOrder(w http.ResponseWriter, r *http.Request, session *CheckoutSession, item *shopdata.Item, quantity int, expiresAt time.Time) (*shopdata.Order, bool) {
	order, err := h.shop.InsertPendingOrder(session.ID, &shopdata.Order{
		Currency:      item.Currency,
		StockReserved: 1,
		ExpiresAt:     expiresAt.Unix(),
		Lines: []*shopdata.OrderLine{{
			ItemID:         item.ID,
			Title:          item.Title,
			UnitPriceCents: item.PriceCents,
			Quantity:       quantity,
		}},
	})
	if err != nil {
		h.infoLog.Printf("failed to record order for session %s: %v", session.ID, err)
		h.restoreReservation(item.ID, quantity)
		if expireErr := h.stripe.ExpireCheckoutSession(r.Context(), session.ID); expireErr != nil {
			h.infoLog.Printf("failed to expire session %s: %v", session.ID, expireErr)
		}
		h.responder.ServerError(w, err)
		return nil, false
	}

	return order, true
}

// restoreReservation best-effort restores the stock a failed checkout reserved.
func (h *Handler) restoreReservation(itemID string, quantity int) {
	if err := h.shop.RestoreStock(itemID, quantity); err != nil {
		h.infoLog.Printf("failed to restore stock for item %s: %v", itemID, err)
	}
}
