package shop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"

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

// StripeWebhook verifies Stripe's signature and records payment. It must read
// the raw body, so it deliberately does not use DecodeJSON.
func (h *Handler) StripeWebhook(w http.ResponseWriter, r *http.Request) {
	if h.stripeSettings.WebhookSecret == "" {
		h.infoLog.Printf("stripe webhook received but no signing secret is configured")
		h.responder.ClientError(w, http.StatusServiceUnavailable)
		return
	}

	event, ok := h.verifyWebhook(w, r)
	if !ok {
		return
	}

	if event.Type != checkoutEventType && event.Type != checkoutExpiredEventType {
		w.WriteHeader(http.StatusOK)
		return
	}

	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		h.infoLog.Printf("failed to decode checkout session from webhook: %v", err)
		h.responder.BadRequest(w)
		return
	}

	if event.Type == checkoutExpiredEventType {
		h.expireReservedOrder(session.ID)
		w.WriteHeader(http.StatusOK)
		return
	}

	h.handlePaidSession(w, r, &session)
}

// verifyWebhook reads and signature-checks the raw request body.
func (h *Handler) verifyWebhook(w http.ResponseWriter, r *http.Request) (stripe.Event, bool) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBody))
	if err != nil {
		h.responder.BadRequest(w)
		return stripe.Event{}, false
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
		return stripe.Event{}, false
	}

	return event, true
}

// handlePaidSession records a confirmed payment and fulfils or refunds it.
func (h *Handler) handlePaidSession(w http.ResponseWriter, r *http.Request, session *stripe.CheckoutSession) {
	paymentIntentID := ""
	if session.PaymentIntent != nil {
		paymentIntentID = session.PaymentIntent.ID
	}

	viewToken, viewTokenHash, err := shopdata.NewOrderViewToken()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	details := paidDetailsFrom(session)
	details.ViewTokenHash = viewTokenHash

	changed, err := h.shop.MarkOrderPaid(session.ID, details)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
	if !changed {
		h.handleDuplicatePaidSession(w, r, session.ID, paymentIntentID)
		return
	}

	h.infoLog.Printf("STRIPE_WEBHOOK paid session=%s", session.ID)

	order, err := h.shop.GetOrderBySessionID(session.ID)
	if err != nil {
		h.infoLog.Printf("paid order %s could not be reloaded: %v", session.ID, err)
		w.WriteHeader(http.StatusOK)
		return
	}

	h.fulfillPaidOrder(w, r, session.ID, order, viewToken, paymentIntentID)
}

// handleDuplicatePaidSession handles a repeated delivery. It is normally a
// no-op. The exception is an order whose refund was requested but not
// confirmed: retry it so a Stripe redelivery can finish the refund.
func (h *Handler) handleDuplicatePaidSession(w http.ResponseWriter, r *http.Request, sessionID, paymentIntentID string) {
	order, err := h.shop.GetOrderBySessionID(sessionID)
	if err != nil {
		h.infoLog.Printf("duplicate order %s could not be reloaded: %v", sessionID, err)
		w.WriteHeader(http.StatusOK)
		return
	}

	if order.Status == shopdata.OrderStatusRefundPending {
		if err = h.refundOversoldOrder(r.Context(), order, sessionID, paymentIntentID); err != nil {
			h.responder.ServerError(w, err)
			return
		}
	} else {
		h.infoLog.Printf("STRIPE_WEBHOOK duplicate session=%s", sessionID)
	}

	w.WriteHeader(http.StatusOK)
}

// fulfillPaidOrder decrements stock for pre-reservation orders and either
// emails the buyer or refunds an oversold order.
func (h *Handler) fulfillPaidOrder(w http.ResponseWriter, r *http.Request, sessionID string, order *shopdata.Order, viewToken, paymentIntentID string) {
	oversold := false
	if order.StockReserved != 1 {
		// The stock was already decremented when the session was created for a
		// reserved order; only pre-reservation orders decrement here.
		oversold = h.decrementOrderStock(order)
	}

	if oversold {
		if _, err := h.shop.MarkOrderRefundPending(sessionID, "oversold"); err != nil {
			h.infoLog.Printf("failed to mark order %s refund pending: %v", sessionID, err)
			h.responder.ServerError(w, err)
			return
		}

		if err := h.refundOversoldOrder(r.Context(), order, sessionID, paymentIntentID); err != nil {
			h.responder.ServerError(w, err)
			return
		}
	} else {
		h.sendOrderEmail(r.Context(), order, viewToken)
		h.sendOrderNotification(r.Context(), order)
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
