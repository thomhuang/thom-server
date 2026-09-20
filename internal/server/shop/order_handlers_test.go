package shop

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	shopdata "thom-server/internal/shop"
)

func TestGetOrderReturnsPublicViewWithoutPII(t *testing.T) {
	handler, _ := newTestHandler(t)

	if _, err := handler.shop.InsertPendingOrder("cs_lookup", &shopdata.Order{
		Currency: "usd",
		Lines:    []*shopdata.OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	// A paid order carries the buyer's name, email, and shipping address; none of
	// it may appear in the anonymous confirmation response.
	if _, err := handler.shop.MarkOrderPaid("cs_lookup", shopdata.PaidDetails{
		CustomerEmail:    "buyer@example.com",
		CustomerName:     "Ada Lovelace",
		ShippingAddress:  "1 Analytical Way, London",
		ShipName:         "Ada Lovelace",
		ShipLine1:        "1 Analytical Way",
		ShipCity:         "London",
		ShipCountry:      "GB",
		AmountTotalCents: 1800,
		Currency:         "usd",
	}); err != nil {
		t.Fatal(err)
	}

	rr := serve(handler.GetOrder, http.MethodGet, "/shop/orders/cs_lookup", "", "sessionId", "cs_lookup")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.Bytes()
	for _, personal := range []string{
		"buyer@example.com",
		"Ada Lovelace",
		"Analytical Way",
		"London",
		"customerEmail",
		"customerName",
		"shippingAddress",
		"shipLine1",
	} {
		if bytes.Contains(body, []byte(personal)) {
			t.Fatalf("public order response leaked %q: %s", personal, rr.Body.String())
		}
	}

	var order publicOrder
	if err := json.Unmarshal(body, &order); err != nil {
		t.Fatal(err)
	}
	if order.ID == "" || order.Status != shopdata.OrderStatusPaid {
		t.Fatalf("public order = %+v, want an id and paid status", order)
	}
	if len(order.Lines) != 1 || order.Lines[0].Title != "Test mug" {
		t.Fatalf("lines = %+v, want the snapshot line", order.Lines)
	}
}

func TestGetOrderByViewTokenReturnsFullOrder(t *testing.T) {
	handler, _ := newTestHandler(t)

	if _, err := handler.shop.InsertPendingOrder("cs_view", &shopdata.Order{Currency: "usd"}); err != nil {
		t.Fatal(err)
	}

	rawToken, tokenHash, err := shopdata.NewOrderViewToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handler.shop.MarkOrderPaid("cs_view", shopdata.PaidDetails{
		CustomerEmail:    "buyer@example.com",
		CustomerName:     "Ada Lovelace",
		ShipName:         "Ada Lovelace",
		ShipLine1:        "1 Analytical Way",
		ShipCity:         "London",
		ShipCountry:      "GB",
		ShippingAddress:  "1 Analytical Way, London, GB",
		AmountTotalCents: 1800,
		Currency:         "usd",
		ViewTokenHash:    tokenHash,
	}); err != nil {
		t.Fatal(err)
	}

	rr := serve(handler.GetOrderByViewToken, http.MethodGet, "/shop/orders/view/"+rawToken, "", "token", rawToken)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (%s)", rr.Code, http.StatusOK, rr.Body.String())
	}

	// The token is the credential, so this view is allowed to include the
	// customer and shipping fields that the session-keyed view redacts.
	for _, personal := range []string{"buyer@example.com", "Ada Lovelace", "Analytical Way"} {
		if !bytes.Contains(rr.Body.Bytes(), []byte(personal)) {
			t.Fatalf("full order response is missing %q: %s", personal, rr.Body.String())
		}
	}

	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := rr.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want no-referrer", got)
	}
	if got := rr.Header().Get("X-Robots-Tag"); got != "noindex" {
		t.Fatalf("X-Robots-Tag = %q, want noindex", got)
	}
}

func TestGetOrderByViewTokenUnknownAndEmpty(t *testing.T) {
	handler, _ := newTestHandler(t)

	if rr := serve(handler.GetOrderByViewToken, http.MethodGet, "/shop/orders/view/nope", "", "token", "nope"); rr.Code != http.StatusNotFound {
		t.Fatalf("unknown token status = %d, want %d", rr.Code, http.StatusNotFound)
	}

	if rr := serve(handler.GetOrderByViewToken, http.MethodGet, "/shop/orders/view/", "", "token", ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("empty token status = %d, want %d", rr.Code, http.StatusBadRequest)
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
