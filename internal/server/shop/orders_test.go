package shop

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	refundErr             error
	refundPaymentIntents  []string
	refundIdempotencyKeys []string
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
	if order.ShipName != "Buyer" {
		t.Fatalf("shipName = %q, want the customer name fallback", order.ShipName)
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

func TestStripeWebhookRefundsOversoldOrder(t *testing.T) {
	handler, _ := newTestHandler(t)
	stripeClient := &fakeStripeClient{}
	handler.WithStripe(stripeClient, StripeSettings{WebhookSecret: testWebhookSecret})

	// Item 1 has stock 5 and sells 2 successfully; item 2 has stock 10 and
	// cannot sell 999. The oversold line must undo the first line and refund.
	if _, err := handler.shop.InsertPendingOrder("cs_test_oversold", &shopdata.Order{
		Currency: "usd",
		Lines: []*shopdata.OrderLine{
			{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 2},
			{ItemID: "2", Title: "Test beans", UnitPriceCents: 2200, Quantity: 999},
		},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutCompletedPayloadForSession(t, &stripe.CheckoutSession{
		ID:            "cs_test_oversold",
		AmountTotal:   4400,
		Currency:      "usd",
		PaymentIntent: &stripe.PaymentIntent{ID: "pi_test_oversold"},
		CustomerDetails: &stripe.CheckoutSessionCustomerDetails{
			Email: "buyer@example.com",
			Name:  "Buyer",
		},
	})

	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_oversold")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != shopdata.OrderStatusRefunded {
		t.Fatalf("status = %q, want refunded", order.Status)
	}
	if len(stripeClient.refundPaymentIntents) != 1 || stripeClient.refundPaymentIntents[0] != "pi_test_oversold" {
		t.Fatalf("refund payment intents = %v, want [pi_test_oversold]", stripeClient.refundPaymentIntents)
	}
	wantKey := "cs_test_oversold:oversold"
	if len(stripeClient.refundIdempotencyKeys) != 1 || stripeClient.refundIdempotencyKeys[0] != wantKey {
		t.Fatalf("refund idempotency keys = %v, want [%s]", stripeClient.refundIdempotencyKeys, wantKey)
	}

	mug, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if mug.Stock != 5 {
		t.Fatalf("mug stock = %d, want 5 after restoring the applied line", mug.Stock)
	}
	beans, err := handler.shop.GetItemByID(2)
	if err != nil {
		t.Fatal(err)
	}
	if beans.Stock != 10 {
		t.Fatalf("beans stock = %d, want 10 untouched", beans.Stock)
	}
}

func TestStripeWebhookRefundErrorLeavesOrderRefundPending(t *testing.T) {
	handler, _ := newTestHandler(t)
	stripeClient := &fakeStripeClient{refundErr: errors.New("stripe unavailable")}
	handler.WithStripe(stripeClient, StripeSettings{WebhookSecret: testWebhookSecret})

	if _, err := handler.shop.InsertPendingOrder("cs_test_refund_fail", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 99}},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutCompletedPayloadForSession(t, &stripe.CheckoutSession{
		ID:            "cs_test_refund_fail",
		AmountTotal:   1800,
		Currency:      "usd",
		PaymentIntent: &stripe.PaymentIntent{ID: "pi_test_refund_fail"},
		CustomerDetails: &stripe.CheckoutSessionCustomerDetails{
			Email: "buyer@example.com",
		},
	})

	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_refund_fail")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != shopdata.OrderStatusRefundPending {
		t.Fatalf("status = %q, want refund_pending", order.Status)
	}
}

func TestStripeWebhookPersistsCollectedShippingAddress(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret})

	if _, err := handler.shop.InsertPendingOrder("cs_test_shipping", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	// The billing address must not be used as the shipping address.
	payload := checkoutCompletedPayloadForSession(t, &stripe.CheckoutSession{
		ID:          "cs_test_shipping",
		AmountTotal: 1800,
		Currency:    "usd",
		CustomerDetails: &stripe.CheckoutSessionCustomerDetails{
			Email: "buyer@example.com",
			Name:  "Billing Name",
			Address: &stripe.Address{
				Line1: "999 Billing Rd",
				City:  "Billingville",
			},
		},
		CollectedInformation: &stripe.CheckoutSessionCollectedInformation{
			ShippingDetails: &stripe.CheckoutSessionCollectedInformationShippingDetails{
				Name: "Ship To Name",
				Address: &stripe.Address{
					Line1:      "1 Main St",
					Line2:      "Apt 2",
					City:       "Portland",
					State:      "OR",
					PostalCode: "97201",
					Country:    "US",
				},
			},
		},
	})

	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_shipping")
	if err != nil {
		t.Fatal(err)
	}
	if order.ShipName != "Ship To Name" {
		t.Fatalf("shipName = %q, want Ship To Name", order.ShipName)
	}
	if order.ShipLine1 != "1 Main St" || order.ShipLine2 != "Apt 2" {
		t.Fatalf("shipping lines = %q/%q, want 1 Main St/Apt 2", order.ShipLine1, order.ShipLine2)
	}
	if order.ShipCity != "Portland" || order.ShipState != "OR" ||
		order.ShipPostalCode != "97201" || order.ShipCountry != "US" {
		t.Fatalf("shipping fields = %+v, want the collected address", order)
	}
	wantAddress := "1 Main St, Apt 2, Portland, OR, 97201, US"
	if order.ShippingAddress != wantAddress {
		t.Fatalf("shippingAddress = %q, want %q", order.ShippingAddress, wantAddress)
	}
	if order.CustomerEmail != "buyer@example.com" {
		t.Fatalf("email = %q, want buyer@example.com", order.CustomerEmail)
	}
}

func TestListOrdersPaginates(t *testing.T) {
	handler, _ := newTestHandler(t)

	for _, sessionID := range []string{"cs_page_1", "cs_page_2", "cs_page_3"} {
		if _, err := handler.shop.InsertPendingOrder(sessionID, &shopdata.Order{
			Currency: "usd",
			Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	first := serve(handler.ListOrders, http.MethodGet, "/shop/orders?limit=2", "")
	if first.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", first.Code, http.StatusOK)
	}

	var firstPage orderListResponse
	if err := json.Unmarshal(first.Body.Bytes(), &firstPage); err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Orders) != 2 {
		t.Fatalf("orders = %d, want 2", len(firstPage.Orders))
	}
	if firstPage.Orders[0].StripeSessionID != "cs_page_3" || firstPage.Orders[1].StripeSessionID != "cs_page_2" {
		t.Fatalf(
			"order sessions = %q/%q, want newest first",
			firstPage.Orders[0].StripeSessionID,
			firstPage.Orders[1].StripeSessionID,
		)
	}
	if firstPage.NextCursor == "" {
		t.Fatal("expected a next cursor")
	}

	second := serve(handler.ListOrders, http.MethodGet, "/shop/orders?limit=2&cursor="+firstPage.NextCursor, "")
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", second.Code, http.StatusOK)
	}

	var secondPage orderListResponse
	if err := json.Unmarshal(second.Body.Bytes(), &secondPage); err != nil {
		t.Fatal(err)
	}
	if len(secondPage.Orders) != 1 || secondPage.Orders[0].StripeSessionID != "cs_page_1" {
		t.Fatalf("second page = %+v, want only cs_page_1", secondPage.Orders)
	}
	if secondPage.NextCursor != "" {
		t.Fatalf("nextCursor = %q, want empty on the last page", secondPage.NextCursor)
	}
}

func TestListOrdersRejectsInvalidPagination(t *testing.T) {
	handler, _ := newTestHandler(t)

	for _, target := range []string{
		"/shop/orders?limit=0",
		"/shop/orders?limit=101",
		"/shop/orders?limit=abc",
		"/shop/orders?cursor=abc",
		"/shop/orders?cursor=0",
	} {
		t.Run(target, func(t *testing.T) {
			rr := serve(handler.ListOrders, http.MethodGet, target, "")
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
			}
		})
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
