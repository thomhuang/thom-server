package shop

import (
	"errors"
	"testing"

	"thom-server/internal/data"
)

func TestModelGetItemsHidesUnpublishedByDefault(t *testing.T) {
	model := newTestModel(t)

	published, err := model.GetItems(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 2 {
		t.Fatalf("expected 2 published items, got %d", len(published))
	}

	all, err := model.GetItems(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 items including drafts, got %d", len(all))
	}
}

func TestModelGetItemsReturnsNewestFirst(t *testing.T) {
	model := newTestModel(t)

	items, err := model.GetItems(true)
	if err != nil {
		t.Fatal(err)
	}

	if items[0].ID != "3" {
		t.Fatalf("expected newest item first, got ID %q", items[0].ID)
	}
}

func TestModelGetItemsIncludesPrimaryImageKey(t *testing.T) {
	model := newTestModel(t)

	items, err := model.GetItems(true)
	if err != nil {
		t.Fatal(err)
	}

	var itemWithImages *ItemSummary
	for _, item := range items {
		if item.ID == "1" {
			itemWithImages = item
		}
	}
	if itemWithImages == nil {
		t.Fatal("expected item 1 in the list")
	}

	// Item 1 has two images; sort order decides which is primary.
	if itemWithImages.PrimaryImageKey != "shop/1/second.jpg" {
		t.Fatalf("expected the lowest sort order image, got %q", itemWithImages.PrimaryImageKey)
	}
}

func TestModelGetItemsOmitsPrimaryImageKeyWhenUnset(t *testing.T) {
	model := newTestModel(t)

	items, err := model.GetItems(true)
	if err != nil {
		t.Fatal(err)
	}

	for _, item := range items {
		if item.ID == "3" && item.PrimaryImageKey != "" {
			t.Fatalf("expected no primary image for item 3, got %q", item.PrimaryImageKey)
		}
	}
}

func TestModelGetItemByIDOrdersImagesBySortOrder(t *testing.T) {
	model := newTestModel(t)

	item, err := model.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}

	if len(item.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(item.Images))
	}
	if item.Images[0].ObjectKey != "shop/1/second.jpg" {
		t.Fatalf("expected the lowest sort order first, got %q", item.Images[0].ObjectKey)
	}
	if item.Images[1].ObjectKey != "shop/1/first.jpg" {
		t.Fatalf("expected the highest sort order second, got %q", item.Images[1].ObjectKey)
	}
}

func TestModelGetItemByIDNoRecord(t *testing.T) {
	model := newTestModel(t)

	_, err := model.GetItemByID(999)
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestModelGetItemsIncludesBrand(t *testing.T) {
	model := newTestModel(t)

	items, err := model.GetItems(true)
	if err != nil {
		t.Fatal(err)
	}

	var branded *ItemSummary
	for _, item := range items {
		if item.ID == "1" {
			branded = item
		}
	}
	if branded == nil {
		t.Fatal("expected item 1 in the list")
	}
	if branded.Brand != "Acme" || branded.BrandID != "acme" {
		t.Fatalf("expected brand Acme/acme, got %q/%q", branded.Brand, branded.BrandID)
	}
}

func TestModelInsertItemCreatesBrand(t *testing.T) {
	model := newTestModel(t)

	created, err := model.InsertItem(&Item{Title: "New mug", PriceCents: 1800, Brand: "1zpresso"})
	if err != nil {
		t.Fatal(err)
	}

	if created.BrandID != "1zpresso" {
		t.Fatalf("expected derived brand id 1zpresso, got %q", created.BrandID)
	}

	brand, err := model.GetBrandByID("1zpresso")
	if err != nil {
		t.Fatal(err)
	}
	if brand.Brand != "1zpresso" {
		t.Fatalf("expected brand name 1zpresso, got %q", brand.Brand)
	}
}

func TestModelInsertItemDefaultsCurrency(t *testing.T) {
	model := newTestModel(t)

	created, err := model.InsertItem(&Item{Title: "New mug", PriceCents: 1800})
	if err != nil {
		t.Fatal(err)
	}

	if created.Currency != DefaultCurrency {
		t.Fatalf("expected currency %q, got %q", DefaultCurrency, created.Currency)
	}
	if created.Images == nil {
		t.Fatal("expected an empty image slice, not nil")
	}
}

func TestModelUpdateItem(t *testing.T) {
	model := newTestModel(t)

	item, err := model.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}

	item.Title = "Updated title"
	item.PriceCents = 2400
	item.Stock = 3
	item.IsPublished = false

	updated, err := model.UpdateItem(1, item)
	if err != nil {
		t.Fatal(err)
	}

	if updated.Title != "Updated title" {
		t.Fatalf("expected updated title, got %q", updated.Title)
	}
	if updated.PriceCents != 2400 {
		t.Fatalf("expected price 2400, got %d", updated.PriceCents)
	}
	if updated.Stock != 3 {
		t.Fatalf("expected stock 3, got %d", updated.Stock)
	}
	if updated.IsPublished {
		t.Fatal("expected the item to be unpublished")
	}
}

func TestModelUpdateItemNoRecord(t *testing.T) {
	model := newTestModel(t)

	_, err := model.UpdateItem(999, &Item{Title: "Missing", PriceCents: 100})
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestModelDeleteItemReturnsImagesAndCascades(t *testing.T) {
	model := newTestModel(t)

	images, err := model.DeleteItem(1)
	if err != nil {
		t.Fatal(err)
	}

	if len(images) != 2 {
		t.Fatalf("expected the deleted item's 2 images back, got %d", len(images))
	}

	if _, err = model.GetItemByID(1); !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected the item to be gone, got %v", err)
	}

	var remaining int
	if err = model.DB.QueryRow(`SELECT COUNT(*) FROM ShopItemImages WHERE ItemID = 1`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("expected the item's images to be deleted with it, found %d", remaining)
	}
}

func TestModelDeleteItemNoRecord(t *testing.T) {
	model := newTestModel(t)

	_, err := model.DeleteItem(999)
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestModelDeleteItemLeavesOtherItemsAlone(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.DeleteItem(1); err != nil {
		t.Fatal(err)
	}

	if _, err := model.GetItemByID(2); err != nil {
		t.Fatalf("expected item 2 to survive, got %v", err)
	}
}
