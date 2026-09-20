package shop

import (
	"testing"
	"time"
)

func TestInsertPendingOrderSnapshotsLines(t *testing.T) {
	model := newTestModel(t)

	order, err := model.InsertPendingOrder("cs_test_1", &Order{
		Currency: "usd",
		Lines: []*OrderLine{
			{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 2},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if order.Status != OrderStatusPending {
		t.Fatalf("status = %q, want %q", order.Status, OrderStatusPending)
	}
	if len(order.Lines) != 1 {
		t.Fatalf("lines = %+v, want a single line", order.Lines)
	}
	if order.Lines[0].Title != "Test mug" || order.Lines[0].UnitPriceCents != 1800 || order.Lines[0].Quantity != 2 {
		t.Fatalf("line = %+v, want the title and price snapshot", order.Lines[0])
	}

	loaded, err := model.GetOrderBySessionID("cs_test_1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != order.ID {
		t.Fatalf("loaded ID = %q, want %q", loaded.ID, order.ID)
	}
}

func TestListOrdersIncludesLinesNewestFirst(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.InsertPendingOrder("cs_first", &Order{
		Currency: "usd",
		Lines:    []*OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 2}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.InsertPendingOrder("cs_second", &Order{
		Currency: "usd",
		Lines:    []*OrderLine{{ItemID: "2", Title: "Test beans", UnitPriceCents: 2200, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	orders, _, err := model.ListOrdersPage(20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 2 {
		t.Fatalf("len(orders) = %d, want 2", len(orders))
	}
	if orders[0].StripeSessionID != "cs_second" {
		t.Fatalf("first order = %q, want the newest cs_second", orders[0].StripeSessionID)
	}
	if len(orders[0].Lines) != 1 || orders[0].Lines[0].Title != "Test beans" {
		t.Fatalf("lines = %+v, want the newest order's line", orders[0].Lines)
	}
}

func TestGetOrderBySessionIDReportsMissing(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.GetOrderBySessionID("cs_missing"); err == nil {
		t.Fatal("err = nil, want an error for a missing order")
	}
}

func TestListOrdersPagePagination(t *testing.T) {
	model := newTestModel(t)

	for _, sessionID := range []string{"cs_page_1", "cs_page_2", "cs_page_3"} {
		if _, err := model.InsertPendingOrder(sessionID, &Order{
			Currency: "usd",
			Lines:    []*OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
		}); err != nil {
			t.Fatal(err)
		}
	}

	first, nextCursor, err := model.ListOrdersPage(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("first page len = %d, want 2", len(first))
	}
	if first[0].StripeSessionID != "cs_page_3" || first[1].StripeSessionID != "cs_page_2" {
		t.Fatalf(
			"first page = %q/%q, want the two newest",
			first[0].StripeSessionID,
			first[1].StripeSessionID,
		)
	}
	if nextCursor == 0 {
		t.Fatal("nextCursor = 0, want a cursor for the second page")
	}

	second, nextCursor, err := model.ListOrdersPage(2, nextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 {
		t.Fatalf("second page len = %d, want 1", len(second))
	}
	if second[0].StripeSessionID != "cs_page_1" {
		t.Fatalf("second page = %q, want the oldest", second[0].StripeSessionID)
	}
	if nextCursor != 0 {
		t.Fatalf("nextCursor = %d, want 0 on the last page", nextCursor)
	}
}

func TestInsertPendingOrderPersistsReservation(t *testing.T) {
	model := newTestModel(t)

	expiresAt := time.Now().Add(time.Hour).Unix()
	order, err := model.InsertPendingOrder("cs_reserved", &Order{
		Currency:      "usd",
		StockReserved: 1,
		ExpiresAt:     expiresAt,
		Lines:         []*OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != OrderStatusPending {
		t.Fatalf("status = %q, want %q", order.Status, OrderStatusPending)
	}

	loaded, err := model.GetOrderBySessionID("cs_reserved")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.StockReserved != 1 {
		t.Fatalf("stockReserved = %d, want 1", loaded.StockReserved)
	}
	if loaded.ExpiresAt != expiresAt {
		t.Fatalf("expiresAt = %d, want %d", loaded.ExpiresAt, expiresAt)
	}

	legacy, err := model.InsertPendingOrder("cs_legacy_reservation", &Order{
		Currency: "usd",
		Lines:    []*OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if legacy.StockReserved != 0 || legacy.ExpiresAt != 0 {
		t.Fatalf("legacy order = %d/%d, want the zero defaults", legacy.StockReserved, legacy.ExpiresAt)
	}
}
