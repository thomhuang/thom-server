package shop

import (
	"thom-server/internal/data"
)

func (m *Model) EnsureSchema() error {
	stmt := `
		CREATE TABLE IF NOT EXISTS ShopBrands (
			id TEXT PRIMARY KEY,
			Brand TEXT NOT NULL COLLATE NOCASE UNIQUE,
			SortOrder INTEGER NOT NULL DEFAULT 1000,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS ShopBrandsSortIndex
		ON ShopBrands(SortOrder ASC, Brand COLLATE NOCASE ASC);

		CREATE TABLE IF NOT EXISTS ShopItems (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			Title TEXT NOT NULL,
			Description TEXT NOT NULL DEFAULT '',
			BrandID TEXT NOT NULL DEFAULT '',
			Brand TEXT NOT NULL DEFAULT '',
			Category TEXT NOT NULL DEFAULT '',
			Size TEXT NOT NULL DEFAULT '',
			PriceCents INTEGER NOT NULL DEFAULT 0,
			Currency TEXT NOT NULL DEFAULT 'usd',
			Stock INTEGER NOT NULL DEFAULT 0,
			IsPublished INTEGER NOT NULL DEFAULT 0,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now')),
			UpdatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS ShopItemsPublishedIndex
		ON ShopItems(IsPublished DESC, id DESC);

		CREATE INDEX IF NOT EXISTS ShopItemsPriceIndex
		ON ShopItems(PriceCents);

		CREATE TABLE IF NOT EXISTS ShopItemImages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ItemID INTEGER NOT NULL REFERENCES ShopItems(id) ON DELETE CASCADE,
			ObjectKey TEXT NOT NULL UNIQUE,
			AltText TEXT NOT NULL DEFAULT '',
			SortOrder INTEGER NOT NULL DEFAULT 1000,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS ShopItemImagesItemIndex
		ON ShopItemImages(ItemID, SortOrder ASC, id ASC);

		CREATE TABLE IF NOT EXISTS ShopItemMeasurements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ItemID INTEGER NOT NULL REFERENCES ShopItems(id) ON DELETE CASCADE,
			Label TEXT NOT NULL,
			ValueInches REAL NOT NULL,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS ShopItemMeasurementsItemIndex
		ON ShopItemMeasurements(ItemID, id ASC);

		CREATE TABLE IF NOT EXISTS ShopOrders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			StripeSessionID TEXT NOT NULL UNIQUE,
			Status TEXT NOT NULL DEFAULT 'pending',
			StockReserved INTEGER NOT NULL DEFAULT 0,
			ExpiresAt INTEGER NOT NULL DEFAULT 0,
			CustomerEmail TEXT NOT NULL DEFAULT '',
			CustomerName TEXT NOT NULL DEFAULT '',
			ShippingAddress TEXT NOT NULL DEFAULT '',
			ShipName TEXT NOT NULL DEFAULT '',
			ShipLine1 TEXT NOT NULL DEFAULT '',
			ShipLine2 TEXT NOT NULL DEFAULT '',
			ShipCity TEXT NOT NULL DEFAULT '',
			ShipState TEXT NOT NULL DEFAULT '',
			ShipPostalCode TEXT NOT NULL DEFAULT '',
			ShipCountry TEXT NOT NULL DEFAULT '',
			ViewTokenHash TEXT NOT NULL DEFAULT '',
			RefundedAt TEXT NOT NULL DEFAULT '',
			RefundReason TEXT NOT NULL DEFAULT '',
			AmountTotalCents INTEGER NOT NULL DEFAULT 0,
			Currency TEXT NOT NULL DEFAULT 'usd',
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now')),
			UpdatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE INDEX IF NOT EXISTS ShopOrdersCreatedIndex
		ON ShopOrders(id DESC);

		CREATE TABLE IF NOT EXISTS ShopOrderLines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			OrderID INTEGER NOT NULL REFERENCES ShopOrders(id) ON DELETE CASCADE,
			ItemID TEXT NOT NULL DEFAULT '',
			Title TEXT NOT NULL,
			UnitPriceCents INTEGER NOT NULL,
			Quantity INTEGER NOT NULL
		);

		CREATE INDEX IF NOT EXISTS ShopOrderLinesOrderIndex
		ON ShopOrderLines(OrderID, id ASC);`

	_, err := m.DB.Exec(stmt)
	if err != nil {
		return err
	}

	if err = m.ensureShopItemColumns(); err != nil {
		return err
	}

	if err = m.ensureShopOrderColumns(); err != nil {
		return err
	}

	// BrandID is added by ensureShopItemColumns and ViewTokenHash by
	// ensureShopOrderColumns, so their indexes can only be created after the
	// columns exist.
	if _, err = m.DB.Exec(
		`CREATE INDEX IF NOT EXISTS ShopItemsBrandIndex ON ShopItems(BrandID)`,
	); err != nil {
		return err
	}

	_, err = m.DB.Exec(
		`CREATE INDEX IF NOT EXISTS ShopOrdersViewTokenIndex ON ShopOrders(ViewTokenHash)`,
	)

	return err
}

// ensureShopItemColumns adds listing columns that predate the current schema and
// retires the flat measurement columns that the ShopItemMeasurements table
// replaced.
func (m *Model) ensureShopItemColumns() error {
	if err := data.EnsureColumns(m.DB, "ShopItems", []data.Column{
		{Name: "BrandID", Definition: "BrandID TEXT NOT NULL DEFAULT ''"},
		{Name: "Brand", Definition: "Brand TEXT NOT NULL DEFAULT ''"},
		{Name: "Category", Definition: "Category TEXT NOT NULL DEFAULT ''"},
		{Name: "Size", Definition: "Size TEXT NOT NULL DEFAULT ''"},
	}); err != nil {
		return err
	}

	return data.DropColumns(
		m.DB,
		"ShopItems",
		"PitToPitInches",
		"BackLengthInches",
		"ShoulderInches",
	)
}

// ensureShopOrderColumns adds the stock-reservation columns and the order
// shipping and refund columns that predate the current schema.
func (m *Model) ensureShopOrderColumns() error {
	return data.EnsureColumns(m.DB, "ShopOrders", []data.Column{
		{Name: "StockReserved", Definition: "StockReserved INTEGER NOT NULL DEFAULT 0"},
		{Name: "ExpiresAt", Definition: "ExpiresAt INTEGER NOT NULL DEFAULT 0"},
		{Name: "ShipName", Definition: "ShipName TEXT NOT NULL DEFAULT ''"},
		{Name: "ShipLine1", Definition: "ShipLine1 TEXT NOT NULL DEFAULT ''"},
		{Name: "ShipLine2", Definition: "ShipLine2 TEXT NOT NULL DEFAULT ''"},
		{Name: "ShipCity", Definition: "ShipCity TEXT NOT NULL DEFAULT ''"},
		{Name: "ShipState", Definition: "ShipState TEXT NOT NULL DEFAULT ''"},
		{Name: "ShipPostalCode", Definition: "ShipPostalCode TEXT NOT NULL DEFAULT ''"},
		{Name: "ShipCountry", Definition: "ShipCountry TEXT NOT NULL DEFAULT ''"},
		{Name: "ViewTokenHash", Definition: "ViewTokenHash TEXT NOT NULL DEFAULT ''"},
		{Name: "RefundedAt", Definition: "RefundedAt TEXT NOT NULL DEFAULT ''"},
		{Name: "RefundReason", Definition: "RefundReason TEXT NOT NULL DEFAULT ''"},
	})
}
