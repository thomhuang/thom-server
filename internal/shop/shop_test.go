package shop

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
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

func TestModelEnsureSchemaDropsLegacyMeasurementColumns(t *testing.T) {
	model := newTestModel(t)

	// Simulate a database created before measurements moved into their own
	// table. Fresh test schemas never have these columns, so without this the
	// DropColumns path would never run.
	for _, statement := range []string{
		"ALTER TABLE ShopItems ADD COLUMN PitToPitInches REAL NOT NULL DEFAULT 0",
		"ALTER TABLE ShopItems ADD COLUMN BackLengthInches REAL NOT NULL DEFAULT 0",
		"ALTER TABLE ShopItems ADD COLUMN ShoulderInches REAL NOT NULL DEFAULT 0",
	} {
		if _, err := model.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	if err := model.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	columns := shopItemColumns(t, model.DB)
	for _, legacy := range []string{"PitToPitInches", "BackLengthInches", "ShoulderInches"} {
		if columns[legacy] {
			t.Fatalf("legacy column %s still present after EnsureSchema", legacy)
		}
	}
	if !columns["Category"] {
		t.Fatal("Category column is missing after EnsureSchema")
	}
	if !columns["Size"] {
		t.Fatal("Size column is missing after EnsureSchema")
	}
}

// shopItemColumns returns the column names currently on ShopItems.
func shopItemColumns(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()

	rows, err := db.Query(`PRAGMA table_info(ShopItems)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue sql.NullString
			primaryKey   int
		)

		if err = rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}

	return columns
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

func TestModelEnsureSchemaAddsOrderColumns(t *testing.T) {
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

	// A legacy ShopOrders table without shipping or refund columns must migrate
	// in place, and its existing rows must read back the new columns as empty.
	legacy := `
		CREATE TABLE ShopOrders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			StripeSessionID TEXT NOT NULL UNIQUE,
			Status TEXT NOT NULL DEFAULT 'pending',
			CustomerEmail TEXT NOT NULL DEFAULT '',
			CustomerName TEXT NOT NULL DEFAULT '',
			ShippingAddress TEXT NOT NULL DEFAULT '',
			AmountTotalCents INTEGER NOT NULL DEFAULT 0,
			Currency TEXT NOT NULL DEFAULT 'usd',
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now')),
			UpdatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);
		INSERT INTO ShopOrders (StripeSessionID, Status) VALUES ('cs_legacy', 'paid');`
	if _, err = db.Exec(legacy); err != nil {
		t.Fatal(err)
	}

	model := &Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema on a legacy ShopOrders failed: %v", err)
	}
	if err = model.EnsureSchema(); err != nil {
		t.Fatalf("second EnsureSchema failed: %v", err)
	}

	order, err := model.GetOrderBySessionID("cs_legacy")
	if err != nil {
		t.Fatal(err)
	}
	if order.ShipName != "" ||
		order.ShipLine1 != "" ||
		order.ShipLine2 != "" ||
		order.ShipCity != "" ||
		order.ShipState != "" ||
		order.ShipPostalCode != "" ||
		order.ShipCountry != "" ||
		order.RefundedAt != "" ||
		order.RefundReason != "" {
		t.Fatalf("legacy order = %+v, want empty shipping and refund fields", order)
	}
	if order.Lines == nil {
		t.Fatal("expected an empty lines slice, not nil")
	}
}

func TestModelMeasurementRoundTripPreservesDecimal(t *testing.T) {
	model := newTestModel(t)

	created, err := model.InsertItem(&Item{
		Title:      "Clothing listing",
		Category:   "pants",
		Size:       "36x32",
		PriceCents: 1800,
		Currency:   "usd",
		Stock:      1,
		Measurements: []*Measurement{
			{Label: "Waist", ValueInches: 24.5},
			{Label: "Inseam", ValueInches: 22.5},
			// 18.25 is stored exactly even though the UI only shows one decimal.
			{Label: "Rise", ValueInches: 18.25},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := measurementValue(created, "Waist"); got != 24.5 {
		t.Fatalf("waist = %v, want 24.5", got)
	}
	if got := measurementValue(created, "Inseam"); got != 22.5 {
		t.Fatalf("inseam = %v, want 22.5", got)
	}
	if got := measurementValue(created, "Rise"); got != 18.25 {
		t.Fatalf("rise = %v, want 18.25", got)
	}
	if created.Category != "pants" {
		t.Fatalf("category = %q, want pants", created.Category)
	}
	if created.Size != "36x32" {
		t.Fatalf("size = %q, want 36x32", created.Size)
	}

	reloadedID, err := strconv.Atoi(created.ID)
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := model.GetItemByID(reloadedID)
	if err != nil {
		t.Fatal(err)
	}
	if got := measurementValue(reloaded, "Waist"); got != 24.5 {
		t.Fatalf("reloaded waist = %v, want 24.5", got)
	}
	if got := measurementValue(reloaded, "Inseam"); got != 22.5 {
		t.Fatalf("reloaded inseam = %v, want 22.5", got)
	}
}

func TestModelMeasurementsDefaultToEmpty(t *testing.T) {
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

	if len(created.Measurements) != 0 {
		t.Fatalf("measurements = %+v, want none", created.Measurements)
	}
}

func TestModelUpdateItemReplacesMeasurements(t *testing.T) {
	model := newTestModel(t)

	item, err := model.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}

	item.Measurements = []*Measurement{
		{Label: "Waist", ValueInches: 21.5},
		{Label: "Inseam", ValueInches: 27.5},
	}

	updated, err := model.UpdateItem(1, item)
	if err != nil {
		t.Fatal(err)
	}

	if got := measurementValue(updated, "Waist"); got != 21.5 {
		t.Fatalf("waist = %v, want 21.5", got)
	}
	if got := measurementValue(updated, "Inseam"); got != 27.5 {
		t.Fatalf("inseam = %v, want 27.5", got)
	}

	// A later update with an empty set clears them.
	updated.Measurements = nil
	cleared, err := model.UpdateItem(1, updated)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.Measurements) != 0 {
		t.Fatalf("measurements = %+v, want cleared", cleared.Measurements)
	}
}

// measurementValue returns the stored value for a label, or -1 when absent.
func measurementValue(item *Item, label string) float64 {
	for _, measurement := range item.Measurements {
		if strings.EqualFold(measurement.Label, label) {
			return measurement.ValueInches
		}
	}

	return -1
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
