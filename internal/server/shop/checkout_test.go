package shop

import (
	"errors"
	"net/http"
	"testing"
	"time"

	shopdata "thom-server/internal/shop"
)

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

func TestCheckoutReservesStockAndSetsSessionExpiry(t *testing.T) {
	handler, _ := newTestHandler(t)
	stripeClient := &fakeStripeClient{}
	handler.WithStripe(stripeClient, StripeSettings{})

	before := time.Now().Unix()

	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":2}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// Stock is reserved immediately, not when the payment webhook arrives.
	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 3 {
		t.Fatalf("stock = %d, want 3 after reserving 2 of 5", item.Stock)
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_123")
	if err != nil {
		t.Fatal(err)
	}
	if order.StockReserved != 1 {
		t.Fatalf("stockReserved = %d, want 1", order.StockReserved)
	}
	if order.ExpiresAt < before+int64(reservationHold/time.Second) ||
		order.ExpiresAt > before+int64((reservationHold+reservationExpiryBuffer+2*time.Minute)/time.Second) {
		t.Fatalf("expiresAt = %d, want roughly now+hold", order.ExpiresAt)
	}
	if stripeClient.lastParams.ExpiresAt != order.ExpiresAt {
		t.Fatalf("session expiresAt = %d, want the stored hold %d", stripeClient.lastParams.ExpiresAt, order.ExpiresAt)
	}
}

func TestCheckoutConflictWhenReservationExhaustsStock(t *testing.T) {
	handler, _ := newTestHandler(t)
	stripeClient := &fakeStripeClient{}
	handler.WithStripe(stripeClient, StripeSettings{})

	// Item 1 has a stock of 5; the first checkout takes all of it.
	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":5}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusCreated, rr.Body.String())
	}

	rr = serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":1}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusConflict)
	}

	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 0 {
		t.Fatalf("stock = %d, want 0 with the reservation held", item.Stock)
	}
	if _, err := handler.shop.GetOrderBySessionID("cs_test_123"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckoutRestoresStockWhenSessionCreationFails(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{err: errors.New("stripe unavailable")}, StripeSettings{})

	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":2}`)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 5 {
		t.Fatalf("stock = %d, want 5 restored after the failed session", item.Stock)
	}
}

func TestCheckoutRestoresStockAndExpiresSessionWhenOrderInsertFails(t *testing.T) {
	handler, _ := newTestHandler(t)
	stripeClient := &fakeStripeClient{session: &CheckoutSession{ID: "cs_dup", URL: "https://checkout.stripe.com/c/pay/cs_dup"}}
	handler.WithStripe(stripeClient, StripeSettings{})

	// The first checkout records the session id; the second one reuses it, so
	// its order insert hits the unique StripeSessionID constraint.
	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":2}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("first status = %d, want %d (%s)", rr.Code, http.StatusCreated, rr.Body.String())
	}

	rr = serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":2}`)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("second status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 3 {
		t.Fatalf("stock = %d, want 3 with the failed reservation restored", item.Stock)
	}

	if len(stripeClient.expiredSessions) != 1 || stripeClient.expiredSessions[0] != "cs_dup" {
		t.Fatalf("expired sessions = %v, want [cs_dup]", stripeClient.expiredSessions)
	}
}

func TestCheckoutReleasesExpiredReservationsFirst(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{})

	// A stale hold takes the last item and has passed its window.
	if ok, err := handler.shop.DecrementStock("1", 1); err != nil || !ok {
		t.Fatalf("setup decrement failed: %v", err)
	}
	if _, err := handler.shop.InsertPendingOrder("cs_hold", &shopdata.Order{
		Currency:      "usd",
		StockReserved: 1,
		ExpiresAt:     time.Now().Add(-time.Minute).Unix(),
		Lines:         []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":5}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusCreated, rr.Body.String())
	}

	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 0 {
		t.Fatalf("stock = %d, want 0 after the freed hold was re-reserved", item.Stock)
	}

	held, err := handler.shop.GetOrderBySessionID("cs_hold")
	if err != nil {
		t.Fatal(err)
	}
	if held.Status != shopdata.OrderStatusExpired {
		t.Fatalf("stale hold status = %q, want expired", held.Status)
	}
}
