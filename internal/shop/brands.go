package shop

import (
	"strings"

	"thom-server/internal/data"
)

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
