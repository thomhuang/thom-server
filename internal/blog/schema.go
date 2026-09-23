package blog

import "thom-server/internal/data"

func (m *Model) EnsureSchema() error {
	stmt := `
		CREATE TABLE IF NOT EXISTS BlogCategories (
			id TEXT PRIMARY KEY,
			Name TEXT NOT NULL COLLATE NOCASE UNIQUE,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE IF NOT EXISTS BlogPosts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			Title TEXT NOT NULL,
			Body TEXT NOT NULL DEFAULT '',
			CategoryID TEXT NOT NULL REFERENCES BlogCategories(id),
			Published INTEGER NOT NULL DEFAULT 0,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now')),
			UpdatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);`

	if _, err := m.DB.Exec(stmt); err != nil {
		return err
	}

	if err := m.ensurePostColumns(); err != nil {
		return err
	}

	// CategoryID is added by ensurePostColumns, so its index can only be
	// created after the column exists.
	_, err := m.DB.Exec(
		`CREATE INDEX IF NOT EXISTS BlogPostsCategoryIndex ON BlogPosts(CategoryID, id DESC)`,
	)
	return err
}

// ensurePostColumns adds CategoryID to a BlogPosts table created before posts
// had categories. CREATE TABLE IF NOT EXISTS never alters an existing table,
// so a pre-category table must be migrated in place.
func (m *Model) ensurePostColumns() error {
	return data.EnsureColumns(m.DB, "BlogPosts", []data.Column{
		{Name: "CategoryID", Definition: "CategoryID TEXT NOT NULL DEFAULT ''"},
	})
}
