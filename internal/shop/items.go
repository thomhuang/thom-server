package shop

import (
	"strconv"
	"sync"

	"thom-server/internal/data"
)

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

	measurements, images, err := m.getItemChildren(itemID)
	if err != nil {
		return nil, err
	}

	item.Measurements = measurements
	item.Images = images

	return item, nil
}

// getItemChildren loads measurements and images together: they are independent
// reads of the same row's children, so running them in parallel avoids two
// sequential D1 round trips.
func (m *Model) getItemChildren(itemID int) ([]*Measurement, []*Image, error) {
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
		return nil, nil, measureErr
	}
	if imageErr != nil {
		return nil, nil, imageErr
	}

	return measurements, images, nil
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
