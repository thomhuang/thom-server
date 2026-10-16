package shop

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

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

func TestCheckoutRecordsMultipleItemsWithoutDecrementing(t *testing.T) {
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

	// Stock is not taken until the payment webhook arrives.
	mug, err := handler.shop.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	beans, err := handler.shop.GetItemByID(2)
	if err != nil {
		t.Fatal(err)
	}
	if mug.Stock != 5 || beans.Stock != 10 {
		t.Fatalf("stock = mug %d / beans %d, want 5 / 10 untouched", mug.Stock, beans.Stock)
	}
}

func TestCheckoutLeavesStockUntouchedWhenAnItemIsUnavailable(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{})

	// Item 1 has stock 5; requesting it alongside the draft item (3) must fail
	// without touching item 1's stock, because nothing was taken yet.
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
		t.Fatalf("stock = %d, want 5 untouched after the failed checkout", item.Stock)
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
}

func TestCheckoutRejectsNonPositiveQuantity(t *testing.T) {
	handler, _ := newTestHandler(t)
	handler.WithStripe(&fakeStripeClient{}, StripeSettings{})

	rr := serve(handler.Checkout, http.MethodPost, "/shop/checkout", `{"itemId":"1","quantity":0}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestCheckoutLeavesStockUntouchedWhenSessionCreationFails(t *testing.T) {
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
		t.Fatalf("stock = %d, want 5 untouched after the failed session", item.Stock)
	}
}

func TestCheckoutExpiresSessionWhenOrderInsertFails(t *testing.T) {
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
	if item.Stock != 5 {
		t.Fatalf("stock = %d, want 5 untouched after the failed insert", item.Stock)
	}

	if len(stripeClient.expiredSessions) != 1 || stripeClient.expiredSessions[0] != "cs_dup" {
		t.Fatalf("expired sessions = %v, want [cs_dup]", stripeClient.expiredSessions)
	}
}
