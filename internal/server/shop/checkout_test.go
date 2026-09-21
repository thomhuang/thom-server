package shop

import (
	"encoding/json"
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

	if len(stripeClient.lastParams.Lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(stripeClient.lastParams.Lines))
	}
	if stripeClient.lastParams.Lines[0].UnitPriceCents != 1800 {
		t.Fatalf("unit price = %d, want 1800 from the database", stripeClient.lastParams.Lines[0].UnitPriceCents)
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

func TestCheckoutReservesMultipleItemsInOneOrder(t *testing.T) {
	handler, _ := newTestHandler(t)
	stripeClient := &fakeStripeClient{}
	handler.WithStripe(stripeClient, StripeSettings{
		SuccessURL:    "http://localhost:3000/shop/order",
		CancelURL:     "http://localhost:3000/shop",
		ShippingCents: 500,
	})

	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout",
		`{"items":[{"itemId":"1","quantity":2},{"itemId":"2","quantity":3}]}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusCreated, rr.Body.String())
	}

	if len(stripeClient.lastParams.Lines) != 2 {
		t.Fatalf("stripe lines = %d, want 2", len(stripeClient.lastParams.Lines))
	}
	if stripeClient.lastParams.Lines[0].UnitPriceCents != 1800 ||
		stripeClient.lastParams.Lines[1].UnitPriceCents != 2200 {
		t.Fatalf("stripe lines = %+v, want database prices", stripeClient.lastParams.Lines)
	}

	order, err := handler.shop.GetOrderBySessionID("cs_test_123")
	if err != nil {
		t.Fatal(err)
	}
	if len(order.Lines) != 2 {
		t.Fatalf("order lines = %d, want 2", len(order.Lines))
	}

	mug, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	beans, err := handler.shop.GetItemByID(2)
	if err != nil {
		t.Fatal(err)
	}
	if mug.Stock != 3 || beans.Stock != 7 {
		t.Fatalf("stock = mug %d / beans %d, want 3 / 7", mug.Stock, beans.Stock)
	}
}

func TestCheckoutRestoresAllStockWhenOneItemIsUnavailable(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{})

	// Item 1 has stock 5; requesting 4 for it and 1 for the draft item (3)
	// must restore the 4 already reserved from item 1.
	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout",
		`{"items":[{"itemId":"1","quantity":4},{"itemId":"3","quantity":1}]}`)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}

	item, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Stock != 5 {
		t.Fatalf("stock = %d, want 5 restored after the failed checkout", item.Stock)
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

	var conflict checkoutConflict
	if err := json.Unmarshal(rr.Body.Bytes(), &conflict); err != nil {
		t.Fatal(err)
	}
	if conflict.Error != "insufficient_stock" || conflict.ItemID != "1" || conflict.Available != 5 {
		t.Fatalf("conflict = %+v, want the sold-out item with its available stock", conflict)
	}
	if conflict.ReservedUntil != 0 {
		t.Fatalf("reservedUntil = %d, want 0 when no reservation holds the item", conflict.ReservedUntil)
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
		order.ExpiresAt > before+int64((reservationHold+2*time.Minute)/time.Second) {
		t.Fatalf("hold expiresAt = %d, want roughly now+hold", order.ExpiresAt)
	}
	// The Stripe session outlives the stock hold so the item frees up quickly
	// while the page stays payable for Stripe's 30-minute minimum.
	if stripeClient.lastParams.ExpiresAt < before+int64(sessionLifetime/time.Second) ||
		stripeClient.lastParams.ExpiresAt > before+int64((sessionLifetime+2*time.Minute)/time.Second) {
		t.Fatalf("session expiresAt = %d, want roughly now+session lifetime", stripeClient.lastParams.ExpiresAt)
	}
	if stripeClient.lastParams.ExpiresAt <= order.ExpiresAt {
		t.Fatalf("session expiresAt = %d, want it after the hold %d", stripeClient.lastParams.ExpiresAt, order.ExpiresAt)
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

	// The first checkout's live hold owns the item, so the conflict reports
	// when that hold ends instead of calling the item sold out.
	var conflict checkoutConflict
	if err := json.Unmarshal(rr.Body.Bytes(), &conflict); err != nil {
		t.Fatal(err)
	}
	if conflict.ItemID != "1" || conflict.Available != 0 {
		t.Fatalf("conflict = %+v, want item 1 with 0 available", conflict)
	}
	if conflict.ReservedUntil <= time.Now().Unix() {
		t.Fatalf("reservedUntil = %d, want the active hold's expiry", conflict.ReservedUntil)
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
