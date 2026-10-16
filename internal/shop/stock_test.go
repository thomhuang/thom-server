package shop

import (
	"strconv"
	"testing"
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
