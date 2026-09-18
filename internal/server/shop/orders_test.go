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

	shopdata "thom-server/internal/shop"
)

const testWebhookSecret = "whsec_test"

type fakeStripeClient struct {
	lastParams CheckoutParams
	session    *CheckoutSession
	err        error
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

func TestCheckoutUsesDatabasePriceAndRecordsPendingOrder(t *testing.T) {
	handler, _ := newTestHandler(t)
	stripeClient := &fakeStripeClient{}
	handler.WithStripe(stripeClient, StripeSettings{
		SuccessURL:    "http://localhost:3000/shop/order",
		CancelURL:     "http://localhost:3000/shop",
		ShippingCents: 500,
	})

	// The client sends a price, but it must be ignored in favour of the row.
	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","priceCents":1}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusCreated, rr.Body.String())
	}

	if stripeClient.lastParams.UnitPriceCents != 1800 {
		t.Fatalf("unit price = %d, want 1800 from the database", stripeClient.lastParams.UnitPriceCents)
	}
	if stripeClient.lastParams.ShippingCents != 500 {
		t.Fatalf("shipping cents = %d, want 500", stripeClient.lastParams.ShippingCents)
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_123")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != shopdata.OrderStatusPending {
		t.Fatalf("status = %q, want pending", order.Status)
	}
	if len(order.Lines) != 1 || order.Lines[0].UnitPriceCents != 1800 {
		t.Fatalf("lines = %+v, want the price snapshot", order.Lines)
	}
}

func TestCheckoutWithoutStripeReturnsServiceUnavailable(t *testing.T) {
	handler, _ := newTestHandler(t)

	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1"}`)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestCheckoutRejectsUnpublishedItem(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{})

	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"3"}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestCheckoutRejectsInsufficientStock(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{})

	// Item 1 has a stock of 5.
	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":6}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusConflict)
	}
}

func TestCheckoutRejectsNonPositiveQuantity(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{})

	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":0}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGetOrderReturnsOrderBySessionID(t *testing.T) {
	handler, _ := newTestHandler(t)

	if _, err := handler.shop.InsertPendingOrder("cs_lookup", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	rr := serve(handler.GetOrder, http.MethodGet, "/shop/orders/cs_lookup", "", "sessionId", "cs_lookup")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var order shopdata.Order
	if err := json.Unmarshal(rr.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	if order.StripeSessionID != "cs_lookup" {
		t.Fatalf("session id = %q, want cs_lookup", order.StripeSessionID)
	}
}

func TestStripeWebhookRejectsBadSignature(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret})

	rr := serve(handler.StripeWebhook, http.MethodPost, "/shop/webhooks/stripe", `{"type":"checkout.session.completed"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestStripeWebhookWithoutSecretReturnsServiceUnavailable(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{})

	rr := serve(handler.StripeWebhook, http.MethodPost, "/shop/webhooks/stripe", `{}`)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestStripeWebhookMarksPaidAndDecrementsStock(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret})

	if _, err := handler.shop.InsertPendingOrder("cs_test_paid", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 2}},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutCompletedPayload(t, "cs_test_paid", 3600)
	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_paid")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != shopdata.OrderStatusPaid {
		t.Fatalf("status = %q, want paid", order.Status)
	}
	if order.CustomerEmail != "buyer@example.com" {
		t.Fatalf("email = %q, want buyer@example.com", order.CustomerEmail)
	}

	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 3 {
		t.Fatalf("stock = %d, want 3 after selling 2 of 5", item.Stock)
	}
}

func TestStripeWebhookIsIdempotent(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret})

	if _, err := handler.shop.InsertPendingOrder("cs_test_again", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "2", Title: "Test beans", UnitPriceCents: 2200, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutCompletedPayload(t, "cs_test_again", 2200)

	for delivery := 0; delivery < 2; delivery++ {
		recorder := httptest.NewRecorder()
		handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))
		if recorder.Code != http.StatusOK {
			t.Fatalf("delivery %d status = %d, want %d", delivery, recorder.Code, http.StatusOK)
		}
	}

	// Stock is decremented once, not once per delivery.
	item, err := handler.shop.GetItemByID(2)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 9 {
		t.Fatalf("stock = %d, want 9 after a single decrement", item.Stock)
	}
}

func TestStripeWebhookIgnoresOtherEventTypes(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret})

	payload := []byte(`{"id":"evt_other","object":"event","type":"payment_intent.created","data":{"object":{}}}`)

	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func webhookRequest(t *testing.T, payload []byte, secret string) *http.Request {
	t.Helper()

	now := time.Now()
	header := fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(stripe.ComputeSignature(now, payload, secret)))

	request := httptest.NewRequest(http.MethodPost, "/shop/webhooks/stripe", bytes.NewReader(payload))
	request.Header.Set("Stripe-Signature", header)

	return request
}

func checkoutCompletedPayload(t *testing.T, sessionID string, amountTotal int64) []byte {
	t.Helper()

	session := stripe.CheckoutSession{
		ID:          sessionID,
		AmountTotal: amountTotal,
		Currency:    "usd",
		CustomerDetails: &stripe.CheckoutSessionCustomerDetails{
			Email: "buyer@example.com",
			Name:  "Buyer",
		},
	}
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
