package shop

import (
	"database/sql"
	"errors"
	"strconv"
	"testing"

	_ "modernc.org/sqlite"

	"thom-server/internal/data"
)

func TestModelEnsureSchemaIsIdempotent(t *testing.T) {
	model := newTestModel(t)

	if err := model.EnsureSchema(); err != nil {
		t.Fatalf("second EnsureSchema failed: %v", err)
	}
}

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

func TestModelEnsureSchemaAddsBrandColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})
	db.SetMaxOpenConns(1)

	// A pre-brand ShopItems table must migrate without recreating the table.
	legacy := `
		CREATE TABLE ShopItems (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			Title TEXT NOT NULL,
			Description TEXT NOT NULL DEFAULT '',
			PriceCents INTEGER NOT NULL DEFAULT 0,
			Currency TEXT NOT NULL DEFAULT 'usd',
			Stock INTEGER NOT NULL DEFAULT 0,
			IsPublished INTEGER NOT NULL DEFAULT 0,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now')),
			UpdatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);
		INSERT INTO ShopItems (Title, PriceCents) VALUES ('Legacy mug', 1500);`
	if _, err = db.Exec(legacy); err != nil {
		t.Fatal(err)
	}

	model := &Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema on a legacy ShopItems failed: %v", err)
	}
	if err = model.EnsureSchema(); err != nil {
		t.Fatalf("second EnsureSchema failed: %v", err)
	}

	item, err := model.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.BrandID != "" || item.Brand != "" {
		t.Fatalf("expected an empty brand, got %q/%q", item.BrandID, item.Brand)
	}

	brands, err := model.GetBrands()
	if err != nil {
		t.Fatal(err)
	}
	if len(brands) != 0 {
		t.Fatalf("expected no brands, got %d", len(brands))
	}
}

func TestModelMeasurementRoundTripPreservesDecimal(t *testing.T) {
	model := newTestModel(t)

	created, err := model.InsertItem(&Item{
		Title:            "Clothing listing",
		PriceCents:       1800,
		Currency:         "usd",
		Stock:            1,
		PitToPitInches:   24.5,
		BackLengthInches: 22.5,
		ShoulderInches:   18.25,
	})
	if err != nil {
		t.Fatal(err)
	}

	if created.PitToPitInches != 24.5 {
		t.Fatalf("pit-to-pit = %v, want 24.5", created.PitToPitInches)
	}
	if created.BackLengthInches != 22.5 {
		t.Fatalf("back length = %v, want 22.5", created.BackLengthInches)
	}
	// 18.25 is stored exactly even though the UI only shows one decimal.
	if created.ShoulderInches != 18.25 {
		t.Fatalf("shoulder = %v, want 18.25", created.ShoulderInches)
	}

	reloadedID, err := strconv.Atoi(created.ID)
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := model.GetItemByID(reloadedID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.PitToPitInches != 24.5 || reloaded.BackLengthInches != 22.5 {
		t.Fatalf(
			"reloaded measurements = %v/%v, want 24.5/22.5",
			reloaded.PitToPitInches,
			reloaded.BackLengthInches,
		)
	}
}

func TestModelMeasurementsDefaultToZero(t *testing.T) {
	model := newTestModel(t)

	// A non-clothing listing simply omits the measurements.
	created, err := model.InsertItem(&Item{
		Title:      "Non-clothing listing",
		PriceCents: 500,
		Currency:   "usd",
		Stock:      1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if created.PitToPitInches != 0 || created.BackLengthInches != 0 || created.ShoulderInches != 0 {
		t.Fatalf(
			"expected zero measurements, got %v/%v/%v",
			created.PitToPitInches,
			created.BackLengthInches,
			created.ShoulderInches,
		)
	}
}

func TestModelUpdateItemPersistsMeasurements(t *testing.T) {
	model := newTestModel(t)

	item, err := model.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}

	item.PitToPitInches = 21.5
	item.BackLengthInches = 27.5
	item.ShoulderInches = 19.5

	updated, err := model.UpdateItem(1, item)
	if err != nil {
		t.Fatal(err)
	}

	if updated.PitToPitInches != 21.5 ||
		updated.BackLengthInches != 27.5 ||
		updated.ShoulderInches != 19.5 {
		t.Fatalf(
			"updated measurements = %v/%v/%v, want 21.5/27.5/19.5",
			updated.PitToPitInches,
			updated.BackLengthInches,
			updated.ShoulderInches,
		)
	}
}

func TestModelGetBrands(t *testing.T) {
	model := newTestModel(t)

	brands, err := model.GetBrands()
	if err != nil {
		t.Fatal(err)
	}

	if len(brands) != 1 {
		t.Fatalf("expected 1 seeded brand, got %d", len(brands))
	}
	if brands[0].ID != "acme" {
		t.Fatalf("expected brand acme, got %q", brands[0].ID)
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

func TestModelAddImageRejectsDuplicateObjectKey(t *testing.T) {
	model := newTestModel(t)

	_, err := model.AddImage(2, &Image{ObjectKey: "shop/1/first.jpg"})
	if err == nil {
		t.Fatal("expected a unique constraint error for a reused object key")
	}
}

func TestModelDeleteImageScopesToItem(t *testing.T) {
	model := newTestModel(t)

	// Image 1 belongs to item 1, so asking for it through item 2 must miss.
	_, err := model.DeleteImage(2, 1)
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}

	image, err := model.DeleteImage(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if image.ObjectKey != "shop/1/first.jpg" {
		t.Fatalf("expected the deleted image key back, got %q", image.ObjectKey)
	}

	count, err := model.CountImages(1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 image left on item 1, got %d", count)
	}
}

func TestModelCountImages(t *testing.T) {
	model := newTestModel(t)

	testCases := []struct {
		itemID int
		want   int
	}{
		{itemID: 1, want: 2},
		{itemID: 2, want: 0},
	}

	for _, testCase := range testCases {
		count, err := model.CountImages(testCase.itemID)
		if err != nil {
			t.Fatal(err)
		}
		if count != testCase.want {
			t.Fatalf("CountImages(%d) = %d, want %d", testCase.itemID, count, testCase.want)
		}
	}
}

func newTestModel(t *testing.T) *Model {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}

	model := &Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	fixture := `
		INSERT INTO ShopBrands (id, Brand, SortOrder)
		VALUES ('acme', 'Acme', 10);

		INSERT INTO ShopItems (id, Title, Description, BrandID, Brand, PriceCents, Currency, Stock, IsPublished)
		VALUES
			(1, 'Test mug', 'A mug', 'acme', 'Acme', 1800, 'usd', 5, 1),
			(2, 'Test beans', 'A bag of beans', '', '', 2200, 'usd', 10, 1),
			(3, 'Draft item', 'Not published yet', '', '', 900, 'usd', 1, 0);

		INSERT INTO ShopItemImages (id, ItemID, ObjectKey, AltText, SortOrder)
		VALUES
			(1, 1, 'shop/1/first.jpg', 'First', 2000),
			(2, 1, 'shop/1/second.jpg', 'Second', 1000);`

	if _, err = db.Exec(fixture); err != nil {
		t.Fatal(err)
	}

	return model
}
