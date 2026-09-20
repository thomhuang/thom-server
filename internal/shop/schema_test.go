package shop

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
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
