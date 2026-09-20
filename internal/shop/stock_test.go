package shop

import (
	"strconv"
	"testing"
	"time"
)

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

func TestReleaseExpiredReservationsRestoresStockOnce(t *testing.T) {
	model := newTestModel(t)
	now := time.Now().Unix()

	item, err := model.InsertItem(&Item{Title: "Held item", PriceCents: 1000, Currency: "usd", Stock: 5, IsPublished: true})
	if err != nil {
		t.Fatal(err)
	}

	if ok, err := model.DecrementStock(item.ID, 2); err != nil || !ok {
		t.Fatalf("setup decrement failed: %v", err)
	}
	if _, err := model.InsertPendingOrder("cs_stale", &Order{
		Currency:      "usd",
		StockReserved: 1,
		ExpiresAt:     now - 60,
		Lines:         []*OrderLine{{ItemID: item.ID, Title: item.Title, UnitPriceCents: item.PriceCents, Quantity: 2}},
	}); err != nil {
		t.Fatal(err)
	}

	if ok, err := model.DecrementStock(item.ID, 1); err != nil || !ok {
		t.Fatalf("setup decrement failed: %v", err)
	}
	if _, err := model.InsertPendingOrder("cs_fresh", &Order{
		Currency:      "usd",
		StockReserved: 1,
		ExpiresAt:     now + 3600,
		Lines:         []*OrderLine{{ItemID: item.ID, Title: item.Title, UnitPriceCents: item.PriceCents, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	// A paid order whose hold window has passed must not be released.
	if ok, err := model.DecrementStock(item.ID, 1); err != nil || !ok {
		t.Fatalf("setup decrement failed: %v", err)
	}
	if _, err := model.InsertPendingOrder("cs_paid_stale", &Order{
		Currency:      "usd",
		StockReserved: 1,
		ExpiresAt:     now - 60,
		Lines:         []*OrderLine{{ItemID: item.ID, Title: item.Title, UnitPriceCents: item.PriceCents, Quantity: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.MarkOrderPaid("cs_paid_stale", PaidDetails{Currency: "usd"}); err != nil {
		t.Fatal(err)
	}

	if err := model.ReleaseExpiredReservations(now); err != nil {
		t.Fatal(err)
	}
	if err := model.ReleaseExpiredReservations(now); err != nil {
		t.Fatal(err)
	}

	stale, err := model.GetOrderBySessionID("cs_stale")
	if err != nil {
		t.Fatal(err)
	}
	if stale.Status != OrderStatusExpired {
		t.Fatalf("stale hold status = %q, want expired", stale.Status)
	}

	fresh, err := model.GetOrderBySessionID("cs_fresh")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status != OrderStatusPending {
		t.Fatalf("fresh hold status = %q, want pending", fresh.Status)
	}

	itemID, err := strconv.Atoi(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := model.GetItemByID(itemID)
	if err != nil {
		t.Fatal(err)
	}
	// 5 - 2 (stale, restored) - 1 (fresh) - 1 (paid) = 3.
	if reloaded.Stock != 3 {
		t.Fatalf("stock = %d, want 3 after releasing the stale hold once", reloaded.Stock)
	}
}
