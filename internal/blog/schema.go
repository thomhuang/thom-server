package blog

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
		);

		CREATE INDEX IF NOT EXISTS BlogPostsCategoryIndex
		ON BlogPosts(CategoryID, id DESC);`

	_, err := m.DB.Exec(stmt)
	return err
}
