package shop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"

	"thom-server/internal/mail"
	"thom-server/internal/server/response"
	shopdata "thom-server/internal/shop"
)

// maxWebhookBody bounds how much of a webhook body is read before verifying it.
const maxWebhookBody = 1 << 20

// The two Stripe events this server acts on. Other types are acknowledged and
// ignored.
const (
	checkoutEventType        = "checkout.session.completed"
	checkoutExpiredEventType = "checkout.session.expired"
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

// WithMailer attaches the transactional email sender and the site URL used to
// build the buyer's order link. A nil sender leaves order email disabled.
func (h *Handler) WithMailer(sender mail.Sender, siteURL string) *Handler {
	h.mailer = sender
	h.siteURL = strings.TrimSuffix(strings.TrimSpace(siteURL), "/")

	return h
}

// WithOrderNotifications sets the operator address that receives a copy of every
// paid order. An empty address leaves admin notifications disabled.
func (h *Handler) WithOrderNotifications(email string) *Handler {
	h.notificationEmail = strings.TrimSpace(email)

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

	// Expired holds are freed lazily here so abandoned reservations never
	// strand stock even when the expired webhook is missed.
	if err := h.shop.ReleaseExpiredReservations(time.Now().Unix()); err != nil {
		h.infoLog.Printf("failed to release expired reservations: %v", err)
	}

	item, err := h.shop.GetItemByID(itemID)
	if h.responder.HandleDataError(w, err) {
		return
	}
	if !item.IsPublished {
		h.responder.NotFound(w)
		return
	}

	ok, err := h.shop.DecrementStock(item.ID, quantity)
	if err != nil {
		h.infoLog.Printf("failed to reserve stock for item %s: %v", item.ID, err)
		h.responder.ServerError(w, err)
		return
	}
	if !ok {
		h.infoLog.Printf("checkout for item %d exceeds available stock", itemID)
		h.responder.ClientError(w, http.StatusConflict)
		return
	}

	expiresAt := time.Now().Add(reservationHold + reservationExpiryBuffer)

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
		return
	}

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
		// The stock is restored and the session expired so a checkout that
		// could not be recorded can never be paid without the reservation.
		h.infoLog.Printf("failed to record order for session %s: %v", session.ID, err)
		h.restoreReservation(item.ID, quantity)
		if expireErr := h.stripe.ExpireCheckoutSession(r.Context(), session.ID); expireErr != nil {
			h.infoLog.Printf("failed to expire session %s: %v", session.ID, expireErr)
		}
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
	}, response.NoStore()); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

// GetOrderByViewToken returns the full order to a buyer who followed the magic
// link emailed after payment. The token is the credential, so this response
// includes the customer and shipping fields; the session-keyed confirmation
// endpoint stays redacted. The response is marked no-store so the token and the
// personal data are not cached.
func (h *Handler) GetOrderByViewToken(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		h.responder.BadRequest(w)
		return
	}

	order, err := h.shop.GetOrderByViewToken(shopdata.HashOrderViewToken(token))
	if h.responder.HandleDataError(w, err) {
		return
	}

	headers := http.Header{
		"Cache-Control":   {"no-store"},
		"Referrer-Policy": {"no-referrer"},
		"X-Robots-Tag":    {"noindex"},
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, order, headers); err != nil {
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

	if event.Type != checkoutEventType && event.Type != checkoutExpiredEventType {
		w.WriteHeader(http.StatusOK)
		return
	}

	var session stripe.CheckoutSession
	if err = json.Unmarshal(event.Data.Raw, &session); err != nil {
		h.infoLog.Printf("failed to decode checkout session from webhook: %v", err)
		h.responder.BadRequest(w)
		return
	}

	if event.Type == checkoutExpiredEventType {
		h.expireReservedOrder(session.ID)
		w.WriteHeader(http.StatusOK)
		return
	}

	paymentIntentID := ""
	if session.PaymentIntent != nil {
		paymentIntentID = session.PaymentIntent.ID
	}

	viewToken, viewTokenHash, err := shopdata.NewOrderViewToken()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	details := paidDetailsFrom(&session)
	details.ViewTokenHash = viewTokenHash

	changed, err := h.shop.MarkOrderPaid(session.ID, details)
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

	oversold := false
	if order.StockReserved == 1 {
		// The stock was already decremented when the session was created.
	} else {
		oversold = h.decrementOrderStock(order)
	}
	if oversold {
		if _, err = h.shop.MarkOrderRefundPending(session.ID, "oversold"); err != nil {
			h.infoLog.Printf("failed to mark order %s refund pending: %v", session.ID, err)
			h.responder.ServerError(w, err)
			return
		}

		if err = h.refundOversoldOrder(r.Context(), order, session.ID, paymentIntentID); err != nil {
			h.responder.ServerError(w, err)
			return
		}
	} else {
		h.sendOrderEmail(r.Context(), order, viewToken)
		h.sendOrderNotification(r.Context(), order)
	}

	w.WriteHeader(http.StatusOK)
}

// sendOrderEmail mails the buyer a link to the full order. Sending is
// best-effort: the payment is already recorded, so a failure is logged rather
// than returned, which would make Stripe redeliver the webhook.
func (h *Handler) sendOrderEmail(ctx context.Context, order *shopdata.Order, viewToken string) {
	if h.mailer == nil || h.siteURL == "" || order.CustomerEmail == "" || viewToken == "" {
		return
	}

	link := h.siteURL + "/shop/order/view?token=" + viewToken
	text, markup := orderEmailBody(order, link)

	err := h.mailer.Send(ctx, mail.Message{
		To:      order.CustomerEmail,
		Subject: "Your order #" + order.ID,
		Text:    text,
		HTML:    markup,
	})
	if err != nil {
		h.infoLog.Printf("failed to send order email for %s: %v", order.ID, err)
		return
	}

	h.infoLog.Printf("ORDER_EMAIL order=%s to=%s", order.ID, order.CustomerEmail)
}

// orderEmailBody renders the order summary and link into the plain-text and
// HTML parts of one email. Titles and addresses are escaped because they are
// admin or buyer input.
func orderEmailBody(order *shopdata.Order, link string) (string, string) {
	var plain, markup strings.Builder

	plain.WriteString("Thanks for your order.\n\n")
	fmt.Fprintf(&plain, "Order #%s\n\n", order.ID)

	markup.WriteString("<p>Thanks for your order.</p>\n")
	fmt.Fprintf(&markup, "<p><strong>Order #%s</strong></p>\n", html.EscapeString(order.ID))

	markup.WriteString("<ul>\n")
	for _, line := range order.Lines {
		fmt.Fprintf(&plain, "%d x %s\n", line.Quantity, line.Title)
		fmt.Fprintf(
			&markup,
			"<li>%d &times; %s</li>\n",
			line.Quantity,
			html.EscapeString(line.Title),
		)
	}
	markup.WriteString("</ul>\n")

	amount := formatAmount(order.AmountTotalCents, order.Currency)
	fmt.Fprintf(&plain, "\nTotal: %s\n", amount)
	fmt.Fprintf(&markup, "<p>Total: %s</p>\n", html.EscapeString(amount))

	if order.ShippingAddress != "" {
		fmt.Fprintf(&plain, "\nShipping to:\n%s\n", order.ShippingAddress)
		fmt.Fprintf(&markup, "<p>Shipping to:<br>%s</p>\n", html.EscapeString(order.ShippingAddress))
	}

	fmt.Fprintf(&plain, "\nView your order: %s\n", link)
	fmt.Fprintf(&markup, "<p><a href=\"%s\">View your order</a></p>\n", html.EscapeString(link))

	return plain.String(), markup.String()
}

// sendOrderNotification mails the operator a copy of a paid order so a sale is
// noticed without polling the orders page. Like the buyer email it is
// best-effort: a failure is logged rather than returned, because the payment is
// already recorded and a returned error would make Stripe redeliver.
func (h *Handler) sendOrderNotification(ctx context.Context, order *shopdata.Order) {
	to := strings.TrimSpace(h.notificationEmail)
	if h.mailer == nil || to == "" {
		return
	}

	text, markup := orderNotificationBody(order, h.siteURL)

	err := h.mailer.Send(ctx, mail.Message{
		To:      to,
		Subject: "New order #" + order.ID,
		Text:    text,
		HTML:    markup,
	})
	if err != nil {
		h.infoLog.Printf("failed to send order notification for %s: %v", order.ID, err)
		return
	}

	h.infoLog.Printf("ORDER_NOTIFICATION order=%s to=%s", order.ID, to)
}

// orderNotificationBody renders the operator's copy of an order. Buyer-entered
// values are escaped because they are untrusted input.
func orderNotificationBody(order *shopdata.Order, siteURL string) (string, string) {
	var plain, markup strings.Builder

	plain.WriteString("New paid order.\n\n")
	fmt.Fprintf(&plain, "Order #%s\n", order.ID)

	markup.WriteString("<p>New paid order.</p>\n")
	fmt.Fprintf(&markup, "<p><strong>Order #%s</strong></p>\n", html.EscapeString(order.ID))

	customer := strings.TrimSpace(order.CustomerName)
	if order.CustomerEmail != "" {
		if customer != "" {
			customer += " <" + order.CustomerEmail + ">"
		} else {
			customer = order.CustomerEmail
		}
	}
	if customer != "" {
		fmt.Fprintf(&plain, "Customer: %s\n", customer)
		fmt.Fprintf(&markup, "<p>Customer: %s</p>\n", html.EscapeString(customer))
	}

	markup.WriteString("<ul>\n")
	for _, line := range order.Lines {
		fmt.Fprintf(&plain, "%d x %s\n", line.Quantity, line.Title)
		fmt.Fprintf(
			&markup,
			"<li>%d &times; %s</li>\n",
			line.Quantity,
			html.EscapeString(line.Title),
		)
	}
	markup.WriteString("</ul>\n")

	amount := formatAmount(order.AmountTotalCents, order.Currency)
	fmt.Fprintf(&plain, "Total: %s\n", amount)
	fmt.Fprintf(&markup, "<p>Total: %s</p>\n", html.EscapeString(amount))

	if order.ShippingAddress != "" {
		fmt.Fprintf(&plain, "\nShip to:\n%s\n", order.ShippingAddress)
		fmt.Fprintf(&markup, "<p>Ship to:<br>%s</p>\n", html.EscapeString(order.ShippingAddress))
	}

	if siteURL != "" {
		ordersLink := siteURL + "/shop/orders"
		fmt.Fprintf(&plain, "\nView orders: %s\n", ordersLink)
		fmt.Fprintf(&markup, "<p><a href=\"%s\">View orders</a></p>\n", html.EscapeString(ordersLink))
	}

	return plain.String(), markup.String()
}

// formatAmount renders cents as a currency string, for example "USD 18.00".
func formatAmount(cents int, currency string) string {
	return fmt.Sprintf("%s %.2f", strings.ToUpper(currency), float64(cents)/100)
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

// expireReservedOrder releases the stock an abandoned Checkout Session held and
// marks its order expired. The status guard on MarkOrderExpired makes this
// idempotent, so a redelivered event or a race with the lazy sweep restores
// stock exactly once.
func (h *Handler) expireReservedOrder(sessionID string) {
	order, err := h.shop.GetOrderBySessionID(sessionID)
	if err != nil {
		h.infoLog.Printf("expired session %s has no matching order: %v", sessionID, err)
		return
	}

	changed, err := h.shop.MarkOrderExpired(sessionID)
	if err != nil {
		h.infoLog.Printf("failed to mark order %s expired: %v", sessionID, err)
		return
	}
	if !changed {
		h.infoLog.Printf("STRIPE_WEBHOOK duplicate expired session=%s", sessionID)
		return
	}

	h.infoLog.Printf("STRIPE_WEBHOOK expired session=%s", sessionID)

	if order.StockReserved == 1 {
		h.restoreStock(order, order.Lines)
	}
}

// restoreReservation best-effort restores the stock a failed checkout reserved.
func (h *Handler) restoreReservation(itemID string, quantity int) {
	if err := h.shop.RestoreStock(itemID, quantity); err != nil {
		h.infoLog.Printf("failed to restore stock for item %s: %v", itemID, err)
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
	if params.ExpiresAt > 0 {
		sessionParams.ExpiresAt = stripe.Int64(params.ExpiresAt)
	}

	session, err := c.client.V1CheckoutSessions.Create(ctx, sessionParams)
	if err != nil {
		return nil, err
	}

	return &CheckoutSession{ID: session.ID, URL: session.URL}, nil
}

// ExpireCheckoutSession kills a Checkout Session that must not be paid, for
// example one whose order could not be recorded after its stock was reserved.
func (c *stripeClient) ExpireCheckoutSession(ctx context.Context, sessionID string) error {
	if c.client == nil {
		return ErrStripeNotConfigured
	}

	_, err := c.client.V1CheckoutSessions.Expire(ctx, sessionID, nil)

	return err
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
