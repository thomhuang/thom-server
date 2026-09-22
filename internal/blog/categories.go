package blog

import (
	"thom-server/internal/data"
)

// GetCategories returns every category alphabetically, for the public filter
// and the admin form's suggestions.
func (m *Model) GetCategories() ([]*Category, error) {
	stmt := `
		SELECT id, Name, CreatedAt
		FROM BlogCategories
		ORDER BY Name COLLATE NOCASE ASC`

	rows, err := m.DB.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := make([]*Category, 0)
	for rows.Next() {
		category := &Category{}
		if err = rows.Scan(&category.ID, &category.Category, &category.CreatedAt); err != nil {
			return nil, err
		}

		categories = append(categories, category)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return categories, nil
}

// upsertCategory registers a category by name and returns the stored row, so a
// post never holds a dangling category id. The lookup is append-only (there is
// no rename or delete route), matching the coffee roaster and grinder lookups.
func (m *Model) upsertCategory(name string) (*Category, error) {
	id := data.SlugifyName(name)
	if id == "" {
		return nil, data.ErrNoRecord
	}

	if _, err := m.DB.Exec(
		`INSERT OR IGNORE INTO BlogCategories (id, Name) VALUES (?, ?)`,
		id,
		name,
	); err != nil {
		return nil, err
	}

	// The name may already exist under a different id, so resolve the row that
	// actually holds the name rather than trusting the slug.
	category := &Category{}
	err := m.DB.QueryRow(
		`SELECT id, Name FROM BlogCategories WHERE Name = ? COLLATE NOCASE`,
		name,
	).Scan(&category.ID, &category.Category)
	if err != nil {
		return nil, data.NoRecord(err)
	}

	return category, nil
}
