package shop

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"thom-server/internal/data"
)

const (
	// DefaultCurrency is used when an item does not set one.
	DefaultCurrency = "usd"

	// MaxItemImages bounds how many images one listing can hold.
	MaxItemImages = 8

	// DefaultImageSortOrder places new images after existing ones.
	DefaultImageSortOrder = 1000
)

// Image is one uploaded listing photo. ObjectKey is the R2 key; URL is filled
// in by the HTTP layer, which knows the public delivery host.
type Image struct {
	ID        string `json:"id"`
	ObjectKey string `json:"objectKey"`
	URL       string `json:"url"`
	AltText   string `json:"altText"`
	SortOrder int    `json:"sortOrder"`
}

// Brand is a maker label listings can reference.
type Brand struct {
	ID        string `json:"id"`
	Brand     string `json:"brand"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// Item is a shop listing.
type Item struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	BrandID     string   `json:"brandId"`
	Brand       string   `json:"brand"`
	PriceCents  int      `json:"priceCents"`
	Currency    string   `json:"currency"`
	Stock       int      `json:"stock"`
	IsPublished bool     `json:"isPublished"`
	Images      []*Image `json:"images"`
	CreatedAt   string   `json:"createdAt,omitempty"`
	UpdatedAt   string   `json:"updatedAt,omitempty"`
}

// ItemSummary is the list representation. PrimaryImageKey stays internal so the
// HTTP layer can turn it into a URL.
type ItemSummary struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	BrandID         string `json:"brandId"`
	Brand           string `json:"brand"`
	PriceCents      int    `json:"priceCents"`
	Currency        string `json:"currency"`
	Stock           int    `json:"stock"`
	IsPublished     bool   `json:"isPublished"`
	PrimaryImageKey string `json:"-"`
	PrimaryImageURL string `json:"primaryImageUrl"`
}

// Model wraps the application database.
type Model struct {
	DB *sql.DB
}

// EnsureSchema creates the shop tables when they are missing.
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

		CREATE TABLE IF NOT EXISTS ShopOrders (
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

	// BrandID is added by ensureShopItemColumns, so its index can only be
	// created after the column exists.
	_, err = m.DB.Exec(
		`CREATE INDEX IF NOT EXISTS ShopItemsBrandIndex ON ShopItems(BrandID)`,
	)

	return err
}

// ensureShopItemColumns adds listing columns that predate the current schema.
func (m *Model) ensureShopItemColumns() error {
	rows, err := m.DB.Query(`PRAGMA table_info(ShopItems)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	existingColumns := make(map[string]bool)
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
			return err
		}
		existingColumns[name] = true
	}
	if err = rows.Err(); err != nil {
		return err
	}

	columns := []struct {
		name       string
		definition string
	}{
		{name: "BrandID", definition: "BrandID TEXT NOT NULL DEFAULT ''"},
		{name: "Brand", definition: "Brand TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if existingColumns[column.name] {
			continue
		}
		if _, err = m.DB.Exec("ALTER TABLE ShopItems ADD COLUMN " + column.definition); err != nil {
			return err
		}
	}

	return nil
}

// GetItems returns listings newest first. Unpublished items are included only
// when requested by an authenticated admin.
func (m *Model) GetItems(includeUnpublished bool) ([]*ItemSummary, error) {
	stmt := `
		SELECT i.id, i.Title, i.BrandID, i.Brand, i.PriceCents, i.Currency, i.Stock, i.IsPublished,
			COALESCE((
				SELECT image.ObjectKey
				FROM ShopItemImages image
				WHERE image.ItemID = i.id
				ORDER BY image.SortOrder ASC, image.id ASC
				LIMIT 1
			), '')
		FROM ShopItems i`
	if !includeUnpublished {
		stmt += `
		WHERE i.IsPublished = 1`
	}
	stmt += `
		ORDER BY i.id DESC`

	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]*ItemSummary, 0)
	for rows.Next() {
		item := &ItemSummary{}
		var id, isPublished int

		if err = rows.Scan(
			&id,
			&item.Title,
			&item.BrandID,
			&item.Brand,
			&item.PriceCents,
			&item.Currency,
			&item.Stock,
			&isPublished,
			&item.PrimaryImageKey,
		); err != nil {
			return nil, err
		}

		item.ID = strconv.Itoa(id)
		item.IsPublished = isPublished != 0
		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

// GetItemByID returns one listing with its images in display order.
func (m *Model) GetItemByID(id int) (*Item, error) {
	stmt := `
		SELECT id, Title, Description, BrandID, Brand, PriceCents, Currency, Stock, IsPublished,
			CreatedAt, UpdatedAt
		FROM ShopItems
		WHERE id = ?`

	item := &Item{}
	var itemID, isPublished int

	err := m.DB.QueryRow(stmt, id).Scan(
		&itemID,
		&item.Title,
		&item.Description,
		&item.BrandID,
		&item.Brand,
		&item.PriceCents,
		&item.Currency,
		&item.Stock,
		&isPublished,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, data.ErrNoRecord
		}
		return nil, err
	}

	item.ID = strconv.Itoa(itemID)
	item.IsPublished = isPublished != 0

	images, err := m.getImages(itemID)
	if err != nil {
		return nil, err
	}
	item.Images = images

	return item, nil
}

// InsertItem stores a new listing and returns the saved row.
func (m *Model) InsertItem(item *Item) (*Item, error) {
	brand, err := m.upsertBrand(&Brand{ID: item.BrandID, Brand: item.Brand})
	if err != nil {
		return nil, err
	}
	item.BrandID = brand.ID
	item.Brand = brand.Brand

	stmt := `
		INSERT INTO ShopItems (Title, Description, BrandID, Brand, PriceCents, Currency, Stock, IsPublished)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := m.DB.Exec(
		stmt,
		item.Title,
		item.Description,
		item.BrandID,
		item.Brand,
		item.PriceCents,
		currencyOr(item.Currency),
		item.Stock,
		boolToInt(item.IsPublished),
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return m.GetItemByID(int(id))
}

// UpdateItem replaces the editable fields of a listing.
func (m *Model) UpdateItem(id int, item *Item) (*Item, error) {
	brand, err := m.upsertBrand(&Brand{ID: item.BrandID, Brand: item.Brand})
	if err != nil {
		return nil, err
	}
	item.BrandID = brand.ID
	item.Brand = brand.Brand

	stmt := `
		UPDATE ShopItems
		SET Title = ?,
			Description = ?,
			BrandID = ?,
			Brand = ?,
			PriceCents = ?,
			Currency = ?,
			Stock = ?,
			IsPublished = ?,
			UpdatedAt = datetime('now')
		WHERE id = ?`

	result, err := m.DB.Exec(
		stmt,
		item.Title,
		item.Description,
		item.BrandID,
		item.Brand,
		item.PriceCents,
		currencyOr(item.Currency),
		item.Stock,
		boolToInt(item.IsPublished),
		id,
	)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, data.ErrNoRecord
	}

	return m.GetItemByID(id)
}

// DeleteItem removes a listing and returns the images that were attached, so
// the caller can delete the matching R2 objects.
func (m *Model) DeleteItem(id int) ([]*Image, error) {
	images, err := m.getImages(id)
	if err != nil {
		return nil, err
	}

	result, err := m.DB.Exec(`DELETE FROM ShopItems WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, data.ErrNoRecord
	}

	return images, nil
}

// AddImage attaches an uploaded object to a listing.
func (m *Model) AddImage(itemID int, image *Image) (*Image, error) {
	stmt := `
		INSERT INTO ShopItemImages (ItemID, ObjectKey, AltText, SortOrder)
		VALUES (?, ?, ?, ?)`

	result, err := m.DB.Exec(stmt, itemID, image.ObjectKey, image.AltText, image.SortOrder)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return m.getImageByID(int(id))
}

// DeleteImage detaches an image from a listing and returns it so the caller can
// delete the R2 object.
func (m *Model) DeleteImage(itemID, imageID int) (*Image, error) {
	stmt := `
		SELECT id, ObjectKey, AltText, SortOrder
		FROM ShopItemImages
		WHERE id = ? AND ItemID = ?`

	image := &Image{}
	var id int

	err := m.DB.QueryRow(stmt, imageID, itemID).Scan(
		&id,
		&image.ObjectKey,
		&image.AltText,
		&image.SortOrder,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, data.ErrNoRecord
		}
		return nil, err
	}
	image.ID = strconv.Itoa(id)

	if _, err = m.DB.Exec(`DELETE FROM ShopItemImages WHERE id = ?`, imageID); err != nil {
		return nil, err
	}

	return image, nil
}

// CountImages reports how many images a listing currently has.
func (m *Model) CountImages(itemID int) (int, error) {
	var count int
	err := m.DB.QueryRow(
		`SELECT COUNT(*) FROM ShopItemImages WHERE ItemID = ?`, itemID,
	).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (m *Model) getImages(itemID int) ([]*Image, error) {
	stmt := `
		SELECT id, ObjectKey, AltText, SortOrder
		FROM ShopItemImages
		WHERE ItemID = ?
		ORDER BY SortOrder ASC, id ASC`

	rows, err := m.DB.Query(stmt, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	images := make([]*Image, 0)
	for rows.Next() {
		image := &Image{}
		var id int

		if err = rows.Scan(&id, &image.ObjectKey, &image.AltText, &image.SortOrder); err != nil {
			return nil, err
		}

		image.ID = strconv.Itoa(id)
		images = append(images, image)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return images, nil
}

func (m *Model) getImageByID(id int) (*Image, error) {
	stmt := `
		SELECT id, ObjectKey, AltText, SortOrder
		FROM ShopItemImages
		WHERE id = ?`

	image := &Image{}
	var imageID int

	err := m.DB.QueryRow(stmt, id).Scan(
		&imageID,
		&image.ObjectKey,
		&image.AltText,
		&image.SortOrder,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, data.ErrNoRecord
		}
		return nil, err
	}
	image.ID = strconv.Itoa(imageID)

	return image, nil
}

func (m *Model) GetBrands() ([]*Brand, error) {
	rows, err := m.DB.Query(
		`SELECT id, Brand, CreatedAt
		 FROM ShopBrands
		 ORDER BY SortOrder ASC, Brand COLLATE NOCASE ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	brands := make([]*Brand, 0)
	for rows.Next() {
		brand := &Brand{}
		if err = rows.Scan(&brand.ID, &brand.Brand, &brand.CreatedAt); err != nil {
			return nil, err
		}

		brands = append(brands, brand)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return brands, nil
}

func (m *Model) UpsertBrand(brand *Brand) (*Brand, error) {
	return m.upsertBrand(brand)
}

// upsertBrand registers a listing's maker. Brands are optional, so an empty
// name resolves to an empty brand rather than an error.
func (m *Model) upsertBrand(brand *Brand) (*Brand, error) {
	brand.Brand = strings.TrimSpace(brand.Brand)
	if brand.Brand == "" {
		return &Brand{}, nil
	}

	existingBrand, err := getBrandByName(m.DB, brand.Brand)
	if err == nil {
		return existingBrand, nil
	}
	if !errors.Is(err, data.ErrNoRecord) {
		return nil, err
	}

	if brand.ID == "" {
		brand.ID = SlugifyName(brand.Brand)
	}

	existingBrand, err = getBrandByID(m.DB, brand.ID)
	if err == nil {
		return existingBrand, nil
	}
	if !errors.Is(err, data.ErrNoRecord) {
		return nil, err
	}

	stmt := `
		INSERT INTO ShopBrands (id, Brand)
		VALUES (?, ?)`

	if _, err = m.DB.Exec(stmt, brand.ID, brand.Brand); err != nil {
		return nil, err
	}

	return getBrandByID(m.DB, brand.ID)
}

func (m *Model) GetBrandByName(name string) (*Brand, error) {
	return getBrandByName(m.DB, name)
}

type querier interface {
	QueryRow(query string, args ...any) *sql.Row
}

func getBrandByName(q querier, name string) (*Brand, error) {
	stmt := `
		SELECT id, Brand, CreatedAt
		FROM ShopBrands
		WHERE Brand = ? COLLATE NOCASE`

	brand := &Brand{}
	err := q.QueryRow(stmt, name).Scan(&brand.ID, &brand.Brand, &brand.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, data.ErrNoRecord
		}
		return nil, err
	}

	return brand, nil
}

func (m *Model) GetBrandByID(id string) (*Brand, error) {
	return getBrandByID(m.DB, id)
}

func getBrandByID(q querier, id string) (*Brand, error) {
	stmt := `
		SELECT id, Brand, CreatedAt
		FROM ShopBrands
		WHERE id = ?`

	brand := &Brand{}
	err := q.QueryRow(stmt, id).Scan(&brand.ID, &brand.Brand, &brand.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, data.ErrNoRecord
		}
		return nil, err
	}

	return brand, nil
}

// SlugifyName turns a display name into the stable id used by the brand
// lookup: lowercase alphanumerics separated by single dashes.
func SlugifyName(value string) string {
	var builder strings.Builder
	lastWasDash := false

	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastWasDash = false
			continue
		}

		if builder.Len() > 0 && !lastWasDash {
			builder.WriteByte('-')
			lastWasDash = true
		}
	}

	return strings.Trim(builder.String(), "-")
}

func currencyOr(currency string) string {
	currency = strings.ToLower(strings.TrimSpace(currency))
	if currency == "" {
		return DefaultCurrency
	}

	return currency
}

func boolToInt(value bool) int {
	if value {
		return 1
	}

	return 0
}
