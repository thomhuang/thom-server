package shop

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v86"

	"thom-server/internal/mail"
	shopdata "thom-server/internal/shop"
)

const testWebhookSecret = "whsec_test"

type fakeMailer struct {
	messages []mail.Message
	err      error
}

func (f *fakeMailer) Send(_ context.Context, message mail.Message) error {
	if f.err != nil {
		return f.err
	}

	f.messages = append(f.messages, message)

	return nil
}

type fakeStripeClient struct {
	lastParams CheckoutParams
	session    *CheckoutSession
	err        error

	refundErr             error
	refundPaymentIntents  []string
	refundIdempotencyKeys []string

	expireErr       error
	expiredSessions []string
}

func (f *fakeStripeClient) CreateCheckoutSession(_ context.Context, params CheckoutParams) (*CheckoutSession, error) {
	f.lastParams = params

	if f.err != nil {
		return nil, f.err
	}
	if f.session != nil {
		return f.session, nil
	}

	return &CheckoutSession{ID: "cs_test_123", URL: "https://checkout.stripe.com/c/pay/cs_test_123"}, nil
}

func (f *fakeStripeClient) RefundPayment(_ context.Context, paymentIntentID string, idempotencyKey string) error {
	f.refundPaymentIntents = append(f.refundPaymentIntents, paymentIntentID)
	f.refundIdempotencyKeys = append(f.refundIdempotencyKeys, idempotencyKey)

	return f.refundErr
}

func (f *fakeStripeClient) ExpireCheckoutSession(_ context.Context, sessionID string) error {
	f.expiredSessions = append(f.expiredSessions, sessionID)

	return f.expireErr
}

func webhookRequest(t *testing.T, payload []byte, secret string) *http.Request {
	t.Helper()

	now := time.Now()
	header := fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(stripe.ComputeSignature(now, payload, secret)))

	request := httptest.NewRequest(http.MethodPost, "/shop/webhooks/stripe", bytes.NewReader(payload))
	request.Header.Set("Stripe-Signature", header)

	return request
}

func checkoutExpiredPayload(t *testing.T, sessionID string) []byte {
	t.Helper()

	sessionJSON, err := json.Marshal(&stripe.CheckoutSession{ID: sessionID})
	if err != nil {
		t.Fatal(err)
	}

	event := struct {
		ID     string `json:"id"`
		Object string `json:"object"`
		Type   string `json:"type"`
		Data   struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}{ID: "evt_test", Object: "event", Type: checkoutExpiredEventType}
	event.Data.Object = sessionJSON

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}

	return payload
}

func checkoutCompletedPayload(t *testing.T, sessionID string, amountTotal int64) []byte {
	t.Helper()

	return checkoutCompletedPayloadForSession(t, &stripe.CheckoutSession{
		ID:          sessionID,
		AmountTotal: amountTotal,
		Currency:    "usd",
		CustomerDetails: &stripe.CheckoutSessionCustomerDetails{
			Email: "buyer@example.com",
			Name:  "Buyer",
		},
	})
}

func checkoutCompletedPayloadForSession(t *testing.T, session *stripe.CheckoutSession) []byte {
	t.Helper()

	sessionJSON, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}

	event := struct {
		ID     string `json:"id"`
		Object string `json:"object"`
		Type   string `json:"type"`
		Data   struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}{ID: "evt_test", Object: "event", Type: checkoutEventType}
	event.Data.Object = sessionJSON

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}

	return payload
}

func TestReleaseOrderHoldExpiresSessionAndRestoresStock(t *testing.T) {
	handler, _ := newTestHandler(t)
	stripeClient := &fakeStripeClient{}
	handler.WithStripe(stripeClient, StripeSettings{})

	// Item 1 has a stock of 5; take all of it so the hold is observable.
	if rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":5}`); rr.Code != http.StatusCreated {
		t.Fatalf("setup status = %d, want %d (%s)", rr.Code, http.StatusCreated, rr.Body.String())
	}

	rr := serve(handler.ReleaseOrderHold, http.MethodPost, "/shop/orders/cs_test_123/release", "", "sessionId", "cs_test_123")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusOK, rr.Body.String())
	}

	var released releaseHoldResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &released); err != nil {
		t.Fatal(err)
	}
	if !released.Released {
		t.Fatal("released = false, want true")
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_123")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != shopdata.OrderStatusExpired {
		t.Fatalf("status = %q, want expired", order.Status)
	}

	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 5 {
		t.Fatalf("stock = %d, want 5 after the hold was released", item.Stock)
	}

	if len(stripeClient.expiredSessions) != 1 || stripeClient.expiredSessions[0] != "cs_test_123" {
		t.Fatalf("expired sessions = %v, want [cs_test_123]", stripeClient.expiredSessions)
	}
}

func TestReleaseOrderHoldLeavesNonPendingOrdersAlone(t *testing.T) {
	handler, _ := newTestHandler(t)
	stripeClient := &fakeStripeClient{}
	handler.WithStripe(stripeClient, StripeSettings{})

	if rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":1}`); rr.Code != http.StatusCreated {
		t.Fatalf("setup status = %d, want %d", rr.Code, http.StatusCreated)
	}

	// The order is no longer pending, so there is nothing to release.
	if _, err := handler.shop.MarkOrderExpired("cs_test_123"); err != nil {
		t.Fatal(err)
	}

	rr := serve(handler.ReleaseOrderHold, http.MethodPost, "/shop/orders/cs_test_123/release", "", "sessionId", "cs_test_123")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var released releaseHoldResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &released); err != nil {
		t.Fatal(err)
	}
	if released.Released {
		t.Fatal("released = true for an order that is not pending")
	}
	if len(stripeClient.expiredSessions) != 0 {
		t.Fatalf("expired sessions = %v, want none for a non-pending order", stripeClient.expiredSessions)
	}
}
