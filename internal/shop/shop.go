package shop

import (
	"database/sql"
	"strconv"
	"strings"
	"sync"

	"thom-server/internal/data"
)

const (
	DefaultCurrency = "usd"

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

type Brand struct {
	ID        string `json:"id"`
	Brand     string `json:"brand"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type Item struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	BrandID     string `json:"brandId"`
	Brand       string `json:"brand"`
	// Category is a free-form hint (for example "tops" or "pants") that the
	// storefront uses to suggest measurement labels. It is not enforced against a
	// fixed set, so a listing can always be filed under something new.
	Category string `json:"category"`
	// Size is a free-form label (for example "Large" or "36x32") shown on the
	// listing. Like Category, it is not validated against a fixed vocabulary.
	Size        string `json:"size"`
	PriceCents  int    `json:"priceCents"`
	Currency    string `json:"currency"`
	Stock       int    `json:"stock"`
	IsPublished bool   `json:"isPublished"`
	// Measurements are arbitrary garment measurements in inches. They are stored
	// as rows rather than columns so a listing can carry any set of labels; an
	// absent measurement is simply a missing row. The UI converts to centimetres
	// for display.
	Measurements []*Measurement `json:"measurements"`
	Images       []*Image       `json:"images"`
	CreatedAt    string         `json:"createdAt,omitempty"`
	UpdatedAt    string         `json:"updatedAt,omitempty"`
}

// Measurement is one labelled garment measurement in inches. Label is free-form
// data, not a key into a fixed vocabulary, so "Waist" and "Inseam" need no
// schema change to exist.
type Measurement struct {
	ID          string  `json:"id,omitempty"`
	Label       string  `json:"label"`
	ValueInches float64 `json:"valueInches"`
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

type Model struct {
	DB *sql.DB
}

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

// ensureShopOrderColumns adds order shipping and refund columns that predate
// the current schema.
func (m *Model) ensureShopOrderColumns() error {
	return data.EnsureColumns(m.DB, "ShopOrders", []data.Column{
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
		SELECT id, Title, Description, BrandID, Brand, Category, Size, PriceCents, Currency, Stock, IsPublished,
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
		&item.Category,
		&item.Size,
		&item.PriceCents,
		&item.Currency,
		&item.Stock,
		&isPublished,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, data.NoRecord(err)
	}

	item.ID = strconv.Itoa(itemID)
	item.IsPublished = isPublished != 0

	// Measurements and images are independent reads of the same row's children,
	// so run them together instead of paying two sequential D1 round trips.
	var (
		measurements []*Measurement
		images       []*Image
		measureErr   error
		imageErr     error
	)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		measurements, measureErr = m.getMeasurements(itemID)
	}()
	go func() {
		defer wg.Done()
		images, imageErr = m.getImages(itemID)
	}()
	wg.Wait()

	if measureErr != nil {
		return nil, measureErr
	}
	if imageErr != nil {
		return nil, imageErr
	}

	item.Measurements = measurements
	item.Images = images

	return item, nil
}

func (m *Model) InsertItem(item *Item) (*Item, error) {
	brand, err := m.upsertBrand(&Brand{ID: item.BrandID, Brand: item.Brand})
	if err != nil {
		return nil, err
	}
	item.BrandID = brand.ID
	item.Brand = brand.Brand

	stmt := `
		INSERT INTO ShopItems (Title, Description, BrandID, Brand, Category, Size, PriceCents, Currency, Stock, IsPublished)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	result, err := m.DB.Exec(
		stmt,
		item.Title,
		item.Description,
		item.BrandID,
		item.Brand,
		item.Category,
		item.Size,
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

	if err = m.replaceMeasurements(int(id), item.Measurements); err != nil {
		return nil, err
	}

	return m.GetItemByID(int(id))
}

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
			Category = ?,
			Size = ?,
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
		item.Category,
		item.Size,
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

	if err = m.replaceMeasurements(id, item.Measurements); err != nil {
		return nil, err
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

	image.ID = strconv.FormatInt(id, 10)

	return image, nil
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
		return nil, data.NoRecord(err)
	}
	image.ID = strconv.Itoa(id)

	if _, err = m.DB.Exec(`DELETE FROM ShopItemImages WHERE id = ?`, imageID); err != nil {
		return nil, err
	}

	return image, nil
}

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

// ItemImageCount reports whether an item exists and how many images it has, in
// a single round trip.
func (m *Model) ItemImageCount(itemID int) (bool, int, error) {
	var exists, count int
	err := m.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM ShopItems WHERE id = ?),
			(SELECT COUNT(*) FROM ShopItemImages WHERE ItemID = ?)`,
		itemID,
		itemID,
	).Scan(&exists, &count)
	if err != nil {
		return false, 0, err
	}

	return exists != 0, count, nil
}

// ItemExists reports whether a listing exists.
func (m *Model) ItemExists(id int) (bool, error) {
	var exists int
	err := m.DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM ShopItems WHERE id = ?)`, id,
	).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists != 0, nil
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

// getMeasurements returns a listing's measurements in the order they were saved.
func (m *Model) getMeasurements(itemID int) ([]*Measurement, error) {
	rows, err := m.DB.Query(
		`SELECT id, Label, ValueInches
		 FROM ShopItemMeasurements
		 WHERE ItemID = ?
		 ORDER BY id ASC`,
		itemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	measurements := make([]*Measurement, 0)
	for rows.Next() {
		measurement := &Measurement{}
		var id int

		if err = rows.Scan(&id, &measurement.Label, &measurement.ValueInches); err != nil {
			return nil, err
		}

		measurement.ID = strconv.Itoa(id)
		measurements = append(measurements, measurement)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return measurements, nil
}

// replaceMeasurements swaps a listing's whole measurement set for the given
// rows. D1 has no interactive transactions, so this deletes then inserts; the
// statements are independent, which is acceptable because the rows are derived
// from a request that is retried as a whole.
func (m *Model) replaceMeasurements(itemID int, measurements []*Measurement) error {
	if _, err := m.DB.Exec(`DELETE FROM ShopItemMeasurements WHERE ItemID = ?`, itemID); err != nil {
		return err
	}

	if len(measurements) == 0 {
		return nil
	}

	placeholders := make([]string, 0, len(measurements))
	args := make([]any, 0, len(measurements)*3)
	for _, measurement := range measurements {
		placeholders = append(placeholders, "(?, ?, ?)")
		args = append(args, itemID, measurement.Label, measurement.ValueInches)
	}

	stmt := `INSERT INTO ShopItemMeasurements (ItemID, Label, ValueInches) VALUES ` +
		strings.Join(placeholders, ", ")

	_, err := m.DB.Exec(stmt, args...)

	return err
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

	if brand.ID == "" {
		brand.ID = SlugifyName(brand.Brand)
	}

	// One statement covers all three cases: an existing case-insensitive name
	// match, an id that collides with a differently named row, and a fresh
	// insert. The no-op SET keeps the stored name, so a colliding id never
	// overwrites the existing row.
	stmt := `
		INSERT INTO ShopBrands (id, Brand)
		VALUES (?, ?)
		ON CONFLICT DO UPDATE SET Brand = Brand
		RETURNING id, Brand, CreatedAt`

	createdBrand := &Brand{}
	err := m.DB.QueryRow(stmt, brand.ID, brand.Brand).Scan(
		&createdBrand.ID,
		&createdBrand.Brand,
		&createdBrand.CreatedAt,
	)
	if err != nil {
		return nil, data.NoRecord(err)
	}

	return createdBrand, nil
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
		return nil, data.NoRecord(err)
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
		return nil, data.NoRecord(err)
	}

	return brand, nil
}

// SlugifyName turns a display name into the stable id used by the brand
// lookup: lowercase alphanumerics separated by single dashes.
func SlugifyName(value string) string {
	return data.SlugifyName(value)
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
