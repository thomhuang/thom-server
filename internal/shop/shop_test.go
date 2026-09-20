package shop

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

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
