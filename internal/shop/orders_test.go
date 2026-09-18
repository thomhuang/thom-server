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

func TestMarkOrderPaidPersistsShippingFields(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.InsertPendingOrder("cs_ship", &Order{Currency: "usd"}); err != nil {
		t.Fatal(err)
	}

	details := PaidDetails{
		CustomerEmail:    "buyer@example.com",
		CustomerName:     "Buyer",
		ShippingAddress:  "1 Example St, Testville, TS 12345, US",
		ShipName:         "Buyer",
		ShipLine1:        "1 Example St",
		ShipLine2:        "Apt 4",
		ShipCity:         "Testville",
		ShipState:        "TS",
		ShipPostalCode:   "12345",
		ShipCountry:      "US",
		AmountTotalCents: 1800,
		Currency:         "usd",
	}

	changed, err := model.MarkOrderPaid("cs_ship", details)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("MarkOrderPaid should update a pending order")
	}

	order, err := model.GetOrderBySessionID("cs_ship")
	if err != nil {
		t.Fatal(err)
	}
	if order.ShipName != "Buyer" ||
		order.ShipLine1 != "1 Example St" ||
		order.ShipLine2 != "Apt 4" ||
		order.ShipCity != "Testville" ||
		order.ShipState != "TS" ||
		order.ShipPostalCode != "12345" ||
		order.ShipCountry != "US" {
		t.Fatalf("shipping fields = %+v, want the paid details", order)
	}
}

func TestMarkOrderRefundPendingThenRefunded(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.InsertPendingOrder("cs_refund", &Order{Currency: "usd"}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.MarkOrderPaid("cs_refund", PaidDetails{Currency: "usd"}); err != nil {
		t.Fatal(err)
	}

	changed, err := model.MarkOrderRefundPending("cs_refund", "customer changed their mind")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("a paid order should move to refund_pending")
	}

	order, err := model.GetOrderBySessionID("cs_refund")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != OrderStatusRefundPending {
		t.Fatalf("status = %q, want %q", order.Status, OrderStatusRefundPending)
	}
	if order.RefundReason != "customer changed their mind" {
		t.Fatalf("refundReason = %q, want the recorded reason", order.RefundReason)
	}

	changed, err = model.MarkOrderRefunded("cs_refund")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("a refund_pending order should move to refunded")
	}

	order, err = model.GetOrderBySessionID("cs_refund")
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != OrderStatusRefunded {
		t.Fatalf("status = %q, want %q", order.Status, OrderStatusRefunded)
	}
	if order.RefundedAt == "" {
		t.Fatal("refundedAt is empty, want the refund timestamp")
	}
}

func TestMarkOrderRefundPendingOnlyFromPaid(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.InsertPendingOrder("cs_unpaid", &Order{Currency: "usd"}); err != nil {
		t.Fatal(err)
	}

	changed, err := model.MarkOrderRefundPending("cs_unpaid", "reason")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("a pending order must not move to refund_pending")
	}
}

func TestRestoreStock(t *testing.T) {
	model := newTestModel(t)

	item, err := model.InsertItem(&Item{Title: "Restock me", PriceCents: 1000, Currency: "usd", Stock: 2})
	if err != nil {
		t.Fatal(err)
	}

	if err = model.RestoreStock(item.ID, 3); err != nil {
		t.Fatal(err)
	}

	itemID, err := strconv.Atoi(item.ID)
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := model.GetItemByID(itemID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Stock != 5 {
		t.Fatalf("stock = %d, want 5", reloaded.Stock)
	}

	// A non-positive quantity is a no-op.
	if err = model.RestoreStock(item.ID, 0); err != nil {
		t.Fatal(err)
	}

	reloaded, err = model.GetItemByID(itemID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Stock != 5 {
		t.Fatalf("stock = %d, want 5 after a zero restore", reloaded.Stock)
	}
}
