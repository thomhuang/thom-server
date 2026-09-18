package shop

import (
	"strconv"
	"testing"
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

func TestGetOrderBySessionIDReportsMissing(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.GetOrderBySessionID("cs_missing"); err == nil {
		t.Fatal("err = nil, want an error for a missing order")
	}
}

func TestMarkOrderPaidIsIdempotent(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.InsertPendingOrder("cs_test_2", &Order{
		Currency: "usd",
		Lines:    []*OrderLine{{ItemID: "1", Title: "Test mug", UnitPriceCents: 1800, Quantity: 2}},
	}); err != nil {
		t.Fatal(err)
	}

	details := PaidDetails{
		CustomerEmail:    "buyer@example.com",
		CustomerName:     "Buyer",
		AmountTotalCents: 3600,
		Currency:         "usd",
	}

	changed, err := model.MarkOrderPaid("cs_test_2", details)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first MarkOrderPaid should update the row")
	}

	changed, err = model.MarkOrderPaid("cs_test_2", details)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("a repeated webhook delivery should be a no-op")
	}

	order, err := model.GetOrderBySessionID("cs_test_2")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != OrderStatusPaid {
		t.Fatalf("status = %q, want %q", order.Status, OrderStatusPaid)
	}
	if order.CustomerEmail != "buyer@example.com" || order.AmountTotalCents != 3600 {
		t.Fatalf("order = %+v, want the paid details recorded", order)
	}
}

func TestDecrementStockOnlyWhenAvailable(t *testing.T) {
	model := newTestModel(t)

	item, err := model.InsertItem(&Item{
		Title:       "Limited run",
		PriceCents:  1000,
		Currency:    "usd",
		Stock:       4,
		IsPublished: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ok, err := model.DecrementStock(item.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("decrement of 3 from 4 should succeed")
	}

	ok, err = model.DecrementStock(item.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("decrement should fail when only 1 remains")
	}

	itemID, err := strconv.Atoi(item.ID)
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := model.GetItemByID(itemID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Stock != 1 {
		t.Fatalf("stock = %d, want 1", reloaded.Stock)
	}
}
