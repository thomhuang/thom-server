package shop

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	shopdata "thom-server/internal/shop"
)

func TestStripeWebhookEmailsOrderViewLink(t *testing.T) {
	handler, _ := newTestHandler(t)
	mailer := &fakeMailer{}
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret}).
		WithMailer(mailer, "https://www.example.com/")

	if _, err := handler.shop.InsertPendingOrder("cs_email", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutCompletedPayload(t, "cs_email", 1800)
	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("sent %d emails, want exactly 1", len(mailer.messages))
	}

	message := mailer.messages[0]
	if message.To != "buyer@example.com" {
		t.Fatalf("recipient = %q, want buyer@example.com", message.To)
	}
	if message.Text == "" || message.HTML == "" {
		t.Fatalf("message = %+v, want both text and html parts", message)
	}

	// The emailed link is the only copy of the raw token, so the view endpoint
	// can only be exercised by extracting it from the message.
	token := viewTokenFromEmail(t, message.Text)
	rr := serve(handler.GetOrderByViewToken, http.MethodGet, "/shop/orders/view/"+token, "", "token", token)
	if rr.Code != http.StatusOK {
		t.Fatalf("view status = %d, want %d (%s)", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("Test mug")) {
		t.Fatalf("view response is missing the order line: %s", rr.Body.String())
	}
}

func TestStripeWebhookNotifiesOperator(t *testing.T) {
	handler, _ := newTestHandler(t)
	mailer := &fakeMailer{}
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret}).
		WithMailer(mailer, "https://www.example.com").
		WithOrderNotifications("owner@example.com")

	if _, err := handler.shop.InsertPendingOrder("cs_notify", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutCompletedPayload(t, "cs_notify", 1800)
	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if len(mailer.messages) != 2 {
		t.Fatalf("sent %d emails, want the buyer plus the operator", len(mailer.messages))
	}

	var buyer, operator bool
	for _, message := range mailer.messages {
		switch message.To {
		case "buyer@example.com":
			buyer = true
		case "owner@example.com":
			operator = true
			if !strings.Contains(message.Subject, "New order") {
				t.Fatalf("operator subject = %q, want a new-order subject", message.Subject)
			}
			if !strings.Contains(message.Text, "Test mug") {
				t.Fatalf("operator body is missing the line item: %s", message.Text)
			}
		}
	}
	if !buyer || !operator {
		t.Fatalf("recipients = %+v, want both the buyer and the operator", mailer.messages)
	}
}

func TestStripeWebhookSkipsOperatorNotificationWhenUnset(t *testing.T) {
	handler, _ := newTestHandler(t)
	mailer := &fakeMailer{}
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret}).
		WithMailer(mailer, "https://www.example.com")

	if _, err := handler.shop.InsertPendingOrder("cs_no_notify", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutCompletedPayload(t, "cs_no_notify", 1800)
	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if len(mailer.messages) != 1 {
		t.Fatalf("sent %d emails, want only the buyer when no operator address is set", len(mailer.messages))
	}
}

func TestStripeWebhookEmailsOnceAcrossDuplicateDeliveries(t *testing.T) {
	handler, _ := newTestHandler(t)
	mailer := &fakeMailer{}
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret}).
		WithMailer(mailer, "https://www.example.com")

	if _, err := handler.shop.InsertPendingOrder("cs_email_dup", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutCompletedPayload(t, "cs_email_dup", 1800)
	for delivery := 0; delivery < 2; delivery++ {
		recorder := httptest.NewRecorder()
		handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))
		if recorder.Code != http.StatusOK {
			t.Fatalf("delivery %d status = %d, want %d", delivery, recorder.Code, http.StatusOK)
		}
	}

	if len(mailer.messages) != 1 {
		t.Fatalf("sent %d emails across duplicate deliveries, want 1", len(mailer.messages))
	}
}

func TestStripeWebhookStillSucceedsWhenEmailFails(t *testing.T) {
	handler, _ := newTestHandler(t)
	mailer := &fakeMailer{err: errors.New("smtp down")}
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{WebhookSecret: testWebhookSecret}).
		WithMailer(mailer, "https://www.example.com")

	if _, err := handler.shop.InsertPendingOrder("cs_email_fail", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	payload := checkoutCompletedPayload(t, "cs_email_fail", 1800)
	recorder := httptest.NewRecorder()
	handler.StripeWebhook(recorder, webhookRequest(t, payload, testWebhookSecret))

	// A mail failure must not make Stripe redeliver the webhook: the payment is
	// already recorded and a retry would not resend anyway.
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	order, err := handler.shop.GetOrderBySessionID("cs_email_fail")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != shopdata.OrderStatusPaid {
		t.Fatalf("status = %q, want paid", order.Status)
	}
}

func viewTokenFromEmail(t *testing.T, body string) string {
	t.Helper()

	const marker = "/shop/order/view?token="
	index := strings.Index(body, marker)
	if index < 0 {
		t.Fatalf("email body has no order link: %s", body)
	}

	token := body[index+len(marker):]
	if end := strings.IndexAny(token, " \n\r\t"); end >= 0 {
		token = token[:end]
	}
	if token == "" {
		t.Fatal("email link has an empty token")
	}

	return token
}
