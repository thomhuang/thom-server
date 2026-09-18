package shop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"

	shopdata "thom-server/internal/shop"
)

// maxWebhookBody bounds how much of a webhook body is read before verifying it.
const maxWebhookBody = 1 << 20

// checkoutEventType is the only Stripe event this server acts on.
const checkoutEventType = "checkout.session.completed"

// ErrStripeNotConfigured is returned when checkout is requested without an API key.
var ErrStripeNotConfigured = errors.New("shop: stripe is not configured")

// StripeClient is the slice of the Stripe API the shop uses. It is an interface
// so handler tests run without network access.
type StripeClient interface {
	CreateCheckoutSession(ctx context.Context, params CheckoutParams) (*CheckoutSession, error)
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

// Checkout creates a Checkout Session for one listing and records a pending
// order. The price is read from the database, never from the request.
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	if h.stripe == nil {
		h.infoLog.Printf("checkout requested but Stripe is not configured")
		h.responder.ClientError(w, http.StatusServiceUnavailable)
		return
	}

	var request checkoutRequest
	if err := h.responder.DecodeJSON(w, r.Body, &request); err != nil {
		h.infoLog.Printf("failed to decode checkout JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	itemID, err := strconv.Atoi(strings.TrimSpace(request.ItemID))
	if err != nil || itemID < 1 {
		h.responder.BadRequest(w)
		return
	}

	quantity := 1
	if request.Quantity != nil {
		quantity = *request.Quantity
	}
	if quantity < 1 {
		h.responder.BadRequest(w)
		return
	}

	item, err := h.shop.GetItemByID(itemID)
	if h.responder.HandleDataError(w, err) {
		return
	}
	if !item.IsPublished {
		h.responder.NotFound(w)
		return
	}
	if item.Stock < quantity {
		h.infoLog.Printf("checkout for item %d exceeds available stock", itemID)
		h.responder.ClientError(w, http.StatusConflict)
		return
	}

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
	})
	if err != nil {
		h.infoLog.Printf("failed to create checkout session: %v", err)
		h.responder.ServerError(w, err)
		return
	}

	order, err := h.shop.InsertPendingOrder(session.ID, &shopdata.Order{
		Currency: item.Currency,
		Lines: []*shopdata.OrderLine{{
			ItemID:         item.ID,
			Title:          item.Title,
			UnitPriceCents: item.PriceCents,
			Quantity:       quantity,
		}},
	})
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	h.infoLog.Printf("CREATE_CHECKOUT session=%s item=%s order=%s", session.ID, item.ID, order.ID)

	err = h.responder.WriteJSON(w, http.StatusCreated, checkoutResponse{
		SessionID: session.ID,
		URL:       session.URL,
	}, nil)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

// GetOrder returns one order by its unguessable session id. A predictable id
// would let anyone enumerate orders and harvest addresses.
func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionId"))
	if sessionID == "" {
		h.responder.BadRequest(w)
		return
	}

	order, err := h.shop.GetOrderBySessionID(sessionID)
	if h.responder.HandleDataError(w, err) {
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, order, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

// ListOrders returns every order, newest first, for the admin view.
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := h.shop.ListOrders()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, orders, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

// StripeWebhook verifies Stripe's signature and records payment. It must read
// the raw body, so it deliberately does not use DecodeJSON.
func (h *Handler) StripeWebhook(w http.ResponseWriter, r *http.Request) {
	if h.stripeSettings.WebhookSecret == "" {
		h.infoLog.Printf("stripe webhook received but no signing secret is configured")
		h.responder.ClientError(w, http.StatusServiceUnavailable)
		return
	}

	payload, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		h.responder.BadRequest(w)
		return
	}

	// The account's event API version can drift from the SDK's expected version.
	// The signature is still verified, so tolerate that mismatch rather than
	// rejecting every delivery.
	event, err := webhook.ConstructEventWithOptions(
		payload,
		r.Header.Get("Stripe-Signature"),
		h.stripeSettings.WebhookSecret,
		webhook.ConstructEventOptions{IgnoreAPIVersionMismatch: true},
	)
	if err != nil {
		h.infoLog.Printf("rejected stripe webhook: %v", err)
		h.responder.BadRequest(w)
		return
	}

	if event.Type != checkoutEventType {
		w.WriteHeader(http.StatusOK)
		return
	}

	var session stripe.CheckoutSession
	if err = json.Unmarshal(event.Data.Raw, &session); err != nil {
		h.infoLog.Printf("failed to decode checkout session from webhook: %v", err)
		h.responder.BadRequest(w)
		return
	}

	changed, err := h.shop.MarkOrderPaid(session.ID, paidDetailsFrom(&session))
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
	if !changed {
		h.infoLog.Printf("STRIPE_WEBHOOK duplicate session=%s", session.ID)
		w.WriteHeader(http.StatusOK)
		return
	}

	h.infoLog.Printf("STRIPE_WEBHOOK paid session=%s", session.ID)

	order, err := h.shop.GetOrderBySessionID(session.ID)
	if err != nil {
		h.infoLog.Printf("paid order %s could not be reloaded: %v", session.ID, err)
	} else {
		h.decrementOrderStock(order)
	}

	w.WriteHeader(http.StatusOK)
}

// decrementOrderStock lowers stock for each purchased line. D1 has no
// interactive transactions, so the model's conditional UPDATE is what prevents
// overselling; a line that cannot be satisfied is logged rather than ignored.
func (h *Handler) decrementOrderStock(order *shopdata.Order) {
	for _, line := range order.Lines {
		ok, err := h.shop.DecrementStock(line.ItemID, line.Quantity)
		if err != nil {
			h.infoLog.Printf("failed to decrement stock for item %s: %v", line.ItemID, err)
			continue
		}
		if !ok {
			h.infoLog.Printf("order %s oversold item %s", order.ID, line.ItemID)
		}
	}
}

func paidDetailsFrom(session *stripe.CheckoutSession) shopdata.PaidDetails {
	details := shopdata.PaidDetails{
		AmountTotalCents: int(session.AmountTotal),
		Currency:         string(session.Currency),
	}

	if session.CustomerDetails != nil {
		details.CustomerEmail = session.CustomerDetails.Email
		details.CustomerName = session.CustomerDetails.Name
		details.ShippingAddress = formatAddress(session.CustomerDetails.Address)
	}

	return details
}

func formatAddress(address *stripe.Address) string {
	if address == nil {
		return ""
	}

	parts := make([]string, 0, 5)
	for _, part := range []string{
		address.Line1,
		address.Line2,
		address.City,
		address.State,
		address.PostalCode,
		address.Country,
	} {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}

	return strings.Join(parts, ", ")
}

type stripeClient struct {
	client *stripe.Client
}

// NewStripeClient returns a client, or nil when no API key is configured so the
// caller can leave checkout disabled.
func NewStripeClient(secretKey string) StripeClient {
	if strings.TrimSpace(secretKey) == "" {
		return nil
	}

	return &stripeClient{client: stripe.NewClient(secretKey)}
}

func (c *stripeClient) CreateCheckoutSession(ctx context.Context, params CheckoutParams) (*CheckoutSession, error) {
	if c.client == nil {
		return nil, ErrStripeNotConfigured
	}

	lineItems := []*stripe.CheckoutSessionCreateLineItemParams{
		lineItem(params.Title, params.Currency, params.UnitPriceCents, params.Quantity),
	}
	if params.ShippingCents > 0 {
		lineItems = append(lineItems, lineItem("Shipping", params.Currency, params.ShippingCents, 1))
	}

	sessionParams := &stripe.CheckoutSessionCreateParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(params.SuccessURL),
		CancelURL:  stripe.String(params.CancelURL),
		LineItems:  lineItems,
		Metadata:   map[string]string{"itemId": params.ItemID},
	}
	if params.TaxEnabled {
		sessionParams.AutomaticTax = &stripe.CheckoutSessionCreateAutomaticTaxParams{
			Enabled: stripe.Bool(true),
		}
	}

	session, err := c.client.V1CheckoutSessions.Create(ctx, sessionParams)
	if err != nil {
		return nil, err
	}

	return &CheckoutSession{ID: session.ID, URL: session.URL}, nil
}

func lineItem(name, currency string, unitAmountCents int, quantity int64) *stripe.CheckoutSessionCreateLineItemParams {
	return &stripe.CheckoutSessionCreateLineItemParams{
		Quantity: stripe.Int64(quantity),
		PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
			Currency:   stripe.String(currency),
			UnitAmount: stripe.Int64(int64(unitAmountCents)),
			ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
				Name: stripe.String(name),
			},
		},
	}
}
