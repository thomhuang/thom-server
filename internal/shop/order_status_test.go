package shop

import "testing"

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

func TestMarkOrderExpiredOnlyForPendingOrders(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.InsertPendingOrder("cs_expire", &Order{Currency: "usd"}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.InsertPendingOrder("cs_paid", &Order{Currency: "usd"}); err != nil {
		t.Fatal(err)
	}

	changed, err := model.MarkOrderExpired("cs_expire")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first MarkOrderExpired should update the row")
	}

	changed, err = model.MarkOrderExpired("cs_expire")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("a repeated expiry should be a no-op")
	}

	if _, err = model.MarkOrderPaid("cs_paid", PaidDetails{Currency: "usd"}); err != nil {
		t.Fatal(err)
	}
	changed, err = model.MarkOrderExpired("cs_paid")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("a paid order must not expire")
	}

	paid, err := model.GetOrderBySessionID("cs_paid")
	if err != nil {
		t.Fatal(err)
	}
	if paid.Status != OrderStatusPaid {
		t.Fatalf("status = %q, want %q", paid.Status, OrderStatusPaid)
	}
}
