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
	RefundPayment(ctx context.Context, paymentIntentID string, idempotencyKey string) error
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

// publicOrder is the buyer-facing view of an order. It deliberately omits the
// customer name, email, and every shipping field: the session id can leak
// through browser history, shared links, or logs, so it is not sufficient
// authorization for personal data. Full orders are only returned by the
// authenticated admin list.
type publicOrder struct {
	ID               string                `json:"id"`
	Status           string                `json:"status"`
	AmountTotalCents int                   `json:"amountTotalCents"`
	Currency         string                `json:"currency"`
	Lines            []*shopdata.OrderLine `json:"lines"`
	RefundedAt       string                `json:"refundedAt"`
	CreatedAt        string                `json:"createdAt"`
	UpdatedAt        string                `json:"updatedAt"`
}

// GetOrder returns the buyer-facing view of one order by its unguessable
// session id. It never includes personal data.
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

	if err = h.responder.WriteJSON(w, http.StatusOK, publicOrder{
		ID:               order.ID,
		Status:           order.Status,
		AmountTotalCents: order.AmountTotalCents,
		Currency:         order.Currency,
		Lines:            order.Lines,
		RefundedAt:       order.RefundedAt,
		CreatedAt:        order.CreatedAt,
		UpdatedAt:        order.UpdatedAt,
	}, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

// orderListResponse wraps a page of orders with the cursor for the next page.
type orderListResponse struct {
	Orders     []*shopdata.Order `json:"orders"`
	NextCursor string            `json:"nextCursor"`
}

// ListOrders returns a page of orders, newest first, for the admin view.
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			h.responder.BadRequest(w)
			return
		}
		limit = parsed
	}

	cursor := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("cursor")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			h.responder.BadRequest(w)
			return
		}
		cursor = parsed
	}

	orders, next, err := h.shop.ListOrdersPage(limit, cursor)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	nextCursor := ""
	if next > 0 {
		nextCursor = strconv.Itoa(next)
	}

	err = h.responder.WriteJSON(w, http.StatusOK, orderListResponse{
		Orders:     orders,
		NextCursor: nextCursor,
	}, nil)
	if err != nil {
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

	paymentIntentID := ""
	if session.PaymentIntent != nil {
		paymentIntentID = session.PaymentIntent.ID
	}

	changed, err := h.shop.MarkOrderPaid(session.ID, paidDetailsFrom(&session))
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
	if !changed {
		// A repeated delivery is normally a no-op. The exception is an order
		// whose refund was requested but not confirmed: retry it so a Stripe
		// redelivery can finish the refund.
		order, loadErr := h.shop.GetOrderBySessionID(session.ID)
		if loadErr != nil {
			h.infoLog.Printf("duplicate order %s could not be reloaded: %v", session.ID, loadErr)
			w.WriteHeader(http.StatusOK)
			return
		}
		if order.Status == shopdata.OrderStatusRefundPending {
			if err = h.refundOversoldOrder(r.Context(), order, session.ID, paymentIntentID); err != nil {
				h.responder.ServerError(w, err)
				return
			}
		} else {
			h.infoLog.Printf("STRIPE_WEBHOOK duplicate session=%s", session.ID)
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	h.infoLog.Printf("STRIPE_WEBHOOK paid session=%s", session.ID)

	order, err := h.shop.GetOrderBySessionID(session.ID)
	if err != nil {
		h.infoLog.Printf("paid order %s could not be reloaded: %v", session.ID, err)
		w.WriteHeader(http.StatusOK)
		return
	}

	if h.decrementOrderStock(order) {
		if _, err = h.shop.MarkOrderRefundPending(session.ID, "oversold"); err != nil {
			h.infoLog.Printf("failed to mark order %s refund pending: %v", session.ID, err)
			h.responder.ServerError(w, err)
			return
		}

		if err = h.refundOversoldOrder(r.Context(), order, session.ID, paymentIntentID); err != nil {
			h.responder.ServerError(w, err)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

// decrementOrderStock lowers stock for each purchased line. D1 has no
// interactive transactions, so the model's conditional UPDATE is what prevents
// overselling. It reports whether any line was oversold and, in that case,
// restores the lines that were already decremented.
func (h *Handler) decrementOrderStock(order *shopdata.Order) bool {
	applied := make([]*shopdata.OrderLine, 0, len(order.Lines))
	for _, line := range order.Lines {
		ok, err := h.shop.DecrementStock(line.ItemID, line.Quantity)
		if err != nil {
			h.infoLog.Printf("failed to decrement stock for item %s: %v", line.ItemID, err)
			continue
		}
		if !ok {
			h.infoLog.Printf("order %s oversold item %s", order.ID, line.ItemID)
			h.restoreStock(order, applied)
			return true
		}
		applied = append(applied, line)
	}

	return false
}

// restoreStock puts back the stock an oversold order had already decremented so
// a partial decrement does not leave listings short.
func (h *Handler) restoreStock(order *shopdata.Order, applied []*shopdata.OrderLine) {
	for _, line := range applied {
		if err := h.shop.RestoreStock(line.ItemID, line.Quantity); err != nil {
			h.infoLog.Printf("failed to restore stock for order %s item %s: %v", order.ID, line.ItemID, err)
		}
	}
}

// refundOversoldOrder refunds a paid order that could not be fulfilled. With no
// Stripe client or payment intent it leaves the order refund_pending for an
// operator to handle manually.
func (h *Handler) refundOversoldOrder(ctx context.Context, order *shopdata.Order, sessionID, paymentIntentID string) error {
	if h.stripe == nil || strings.TrimSpace(paymentIntentID) == "" {
		h.infoLog.Printf("order %s is oversold but has no payment intent to refund", sessionID)
		return nil
	}

	if err := h.stripe.RefundPayment(ctx, paymentIntentID, sessionID+":oversold"); err != nil {
		h.infoLog.Printf("failed to refund oversold order %s: %v", sessionID, err)
		return err
	}

	if _, err := h.shop.MarkOrderRefunded(sessionID); err != nil {
		h.infoLog.Printf("failed to mark order %s refunded: %v", sessionID, err)
		return err
	}

	h.infoLog.Printf("STRIPE_REFUND order=%s session=%s paymentIntent=%s", order.ID, sessionID, paymentIntentID)

	return nil
}

func paidDetailsFrom(session *stripe.CheckoutSession) shopdata.PaidDetails {
	details := shopdata.PaidDetails{
		AmountTotalCents: int(session.AmountTotal),
		Currency:         string(session.Currency),
	}

	if session.CustomerDetails != nil {
		details.CustomerEmail = session.CustomerDetails.Email
		details.CustomerName = session.CustomerDetails.Name
	}

	// Shipping is collected by Checkout and lives under collected information;
	// the billing address is deliberately not used as the shipping address.
	if collected := session.CollectedInformation; collected != nil && collected.ShippingDetails != nil {
		details.ShipName = collected.ShippingDetails.Name
		if address := collected.ShippingDetails.Address; address != nil {
			details.ShipLine1 = address.Line1
			details.ShipLine2 = address.Line2
			details.ShipCity = address.City
			details.ShipState = address.State
			details.ShipPostalCode = address.PostalCode
			details.ShipCountry = address.Country
			details.ShippingAddress = formatAddress(address)
		}
	}

	if details.ShipName == "" && session.CustomerDetails != nil {
		details.ShipName = session.CustomerDetails.Name
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
		// Shipping is US-only.
		ShippingAddressCollection: &stripe.CheckoutSessionCreateShippingAddressCollectionParams{
			AllowedCountries: []*string{stripe.String("US")},
		},
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

// RefundPayment refunds the payment behind a Checkout Session. Checkout
// captures payment immediately, so the refund path is normally taken; only an
// uncaptured intent is voided instead.
func (c *stripeClient) RefundPayment(ctx context.Context, paymentIntentID string, idempotencyKey string) error {
	if c.client == nil {
		return ErrStripeNotConfigured
	}
	if strings.TrimSpace(paymentIntentID) == "" {
		return errors.New("shop: missing payment intent id")
	}

	intent, err := c.client.V1PaymentIntents.Retrieve(ctx, paymentIntentID, nil)
	if err != nil {
		return err
	}

	if intent.Status == stripe.PaymentIntentStatusRequiresCapture {
		_, err = c.client.V1PaymentIntents.Cancel(ctx, paymentIntentID, nil)
		return err
	}

	params := &stripe.RefundCreateParams{PaymentIntent: stripe.String(paymentIntentID)}
	params.SetIdempotencyKey(idempotencyKey)

	_, err = c.client.V1Refunds.Create(ctx, params)

	return err
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
