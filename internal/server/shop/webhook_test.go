package shop

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v86"

	shopdata "thom-server/internal/shop"
)

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

func TestStripeWebhookExpiredReleasesReservedStock(t *testing.T) {
	handler, _ := newTestHandler(t)
	mailer := &fakeMailer{}
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret}).
		WithMailer(mailer, "https://www.example.com")

	if ok, err := handler.shop.DecrementStock("1", 2); err != nil || !ok {
		t.Fatalf("setup decrement failed: %v", err)
	}
	if _, err := handler.shop.InsertPendingOrder("cs_test_expired", &shopdata.Order{
		Currency:      "usd",
		StockReserved: 1,
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
		Lines:         []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 2}},
	}); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, checkoutExpiredPayload(t, "cs_test_expired"), testWebhookSecret))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_expired")
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
		t.Fatalf("stock = %d, want 5 after releasing the reservation", item.Stock)
	}

	if len(mailer.messages) != 0 {
		t.Fatalf("emails = %d, want none for an expired order", len(mailer.messages))
	}
}

func TestStripeWebhookExpiredIsIdempotent(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret})

	if ok, err := handler.shop.DecrementStock("1", 2); err != nil || !ok {
		t.Fatalf("setup decrement failed: %v", err)
	}
	if _, err := handler.shop.InsertPendingOrder("cs_test_expired_twice", &shopdata.Order{
		Currency:      "usd",
		StockReserved: 1,
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
		Lines:         []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 2}},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutExpiredPayload(t, "cs_test_expired_twice")
	for delivery := 0; delivery < 2; delivery++ {
		recorder := httptest.NewRecorder()
		handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))
		if recorder.Code != http.StatusOK {
			t.Fatalf("delivery %d status = %d, want %d", delivery, recorder.Code, http.StatusOK)
		}
	}

	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 5 {
		t.Fatalf("stock = %d, want 5 with a single release", item.Stock)
	}
}

func TestStripeWebhookExpiredLegacyOrderDoesNotRestoreStock(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret})

	// A pre-reservation pending order never decremented stock at checkout.
	if _, err := handler.shop.InsertPendingOrder("cs_test_legacy_expired", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 2}},
	}); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, checkoutExpiredPayload(t, "cs_test_legacy_expired"), testWebhookSecret))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_legacy_expired")
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
		t.Fatalf("stock = %d, want 5 untouched", item.Stock)
	}
}

func TestStripeWebhookCompletedForReservedOrderSkipsDecrement(t *testing.T) {
	handler, _ := newTestHandler(t)
	mailer := &fakeMailer{}
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret}).
		WithMailer(mailer, "https://www.example.com")

	if ok, err := handler.shop.DecrementStock("1", 2); err != nil || !ok {
		t.Fatalf("setup decrement failed: %v", err)
	}
	if _, err := handler.shop.InsertPendingOrder("cs_test_reserved_paid", &shopdata.Order{
		Currency:      "usd",
		StockReserved: 1,
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
		Lines:         []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 2}},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutCompletedPayload(t, "cs_test_reserved_paid", 3600)
	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_reserved_paid")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != shopdata.OrderStatusPaid {
		t.Fatalf("status = %q, want paid", order.Status)
	}

	// The reservation already decremented stock; the webhook must not again.
	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 3 {
		t.Fatalf("stock = %d, want 3 with no second decrement", item.Stock)
	}

	if len(mailer.messages) != 1 {
		t.Fatalf("emails = %d, want one order email", len(mailer.messages))
	}
}

func TestStripeWebhookExpiredDoesNotRevivePaidReservedOrder(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret})

	if ok, err := handler.shop.DecrementStock("1", 2); err != nil || !ok {
		t.Fatalf("setup decrement failed: %v", err)
	}
	if _, err := handler.shop.InsertPendingOrder("cs_test_paid_then_expired", &shopdata.Order{
		Currency:      "usd",
		StockReserved: 1,
		ExpiresAt:     time.Now().Add(time.Hour).Unix(),
		Lines:         []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 2}},
	}); err != nil {
		t.Fatal(err)
	}

	paid := checkoutCompletedPayload(t, "cs_test_paid_then_expired", 3600)
	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, paid, testWebhookSecret))
	if recorder.Code != http.StatusOK {
		t.Fatalf("paid delivery status = %d, want %d", recorder.Code, http.StatusOK)
	}

	expired := checkoutExpiredPayload(t, "cs_test_paid_then_expired")
	recorder = httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, expired, testWebhookSecret))
	if recorder.Code != http.StatusOK {
		t.Fatalf("expired delivery status = %d, want %d", recorder.Code, http.StatusOK)
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_paid_then_expired")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != shopdata.OrderStatusPaid {
		t.Fatalf("status = %q, want paid after a late expired event", order.Status)
	}

	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 3 {
		t.Fatalf("stock = %d, want 3 with no release for a paid order", item.Stock)
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

func TestStripeWebhookRedeliveryDoesNotReviveRefundedOrder(t *testing.T) {
	handler, _ := newTestHandler(t)
	stripeClient := &fakeStripeClient{}
	mailer := &fakeMailer{}
	handler.WithStripe(stripeClient, StripeSettings{WebhookSecret: testWebhookSecret}).
		WithMailer(mailer, "https://www.example.com")

	// Item 1 has stock 5 and the order sells 99, so the first delivery detects
	// the oversell and refunds the payment.
	if _, err := handler.shop.InsertPendingOrder("cs_test_redeliver_refund", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 99}},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutCompletedPayloadForSession(t, &stripe.CheckoutSession{
		ID:            "cs_test_redeliver_refund",
		AmountTotal:   1800,
		Currency:      "usd",
		PaymentIntent: &stripe.PaymentIntent{ID: "pi_test_redeliver_refund"},
		CustomerDetails: &stripe.CheckoutSessionCustomerDetails{
			Email: "buyer@example.com",
		},
	})

	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))
	if recorder.Code != http.StatusOK {
		t.Fatalf("first delivery status = %d, want %d (%s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	// A restock after the refund makes the oversold line buyable again, so a
	// redelivered completed event would decrement stock if it revived the order.
	if err := handler.shop.RestoreStock("1", 200); err != nil {
		t.Fatal(err)
	}
	mailer.messages = nil

	recorder = httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))
	if recorder.Code != http.StatusOK {
		t.Fatalf("second delivery status = %d, want %d", recorder.Code, http.StatusOK)
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_redeliver_refund")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != shopdata.OrderStatusRefunded {
		t.Fatalf("status = %q, want refunded after redelivery", order.Status)
	}

	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 205 {
		t.Fatalf("stock = %d, want 205 with no second decrement", item.Stock)
	}

	if len(mailer.messages) != 0 {
		t.Fatalf("emails after redelivery = %d, want 0", len(mailer.messages))
	}

	if len(stripeClient.refundPaymentIntents) != 1 {
		t.Fatalf("refunds = %v, want exactly one refund", stripeClient.refundPaymentIntents)
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
