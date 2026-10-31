package blog

import "database/sql"

// Draft is the admin's in-progress "new post" form. There is one slot (id 1),
// so autosaving a new draft replaces the previous one.
type Draft struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Category  string `json:"category"`
	Published bool   `json:"published"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// GetDraft returns the saved draft, or an empty draft when none has been saved.
func (m *Model) GetDraft() (*Draft, error) {
	row := m.DB.QueryRow(
		`SELECT Title, Body, Category, Published, UpdatedAt FROM BlogDraft WHERE id = 1`,
	)

	draft := &Draft{}
	var published int
	err := row.Scan(&draft.Title, &draft.Body, &draft.Category, &published, &draft.UpdatedAt)
	if err == sql.ErrNoRows {
		return draft, nil
	}
	if err != nil {
		return nil, err
	}

	draft.Published = published == 1

	return draft, nil
}

// SaveDraft upserts the single draft slot.
func (m *Model) SaveDraft(draft *Draft) error {
	published := 0
	if draft.Published {
		published = 1
	}

	_, err := m.DB.Exec(`
		INSERT INTO BlogDraft (id, Title, Body, Category, Published, UpdatedAt)
		VALUES (1, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(id) DO UPDATE SET
			Title = excluded.Title,
			Body = excluded.Body,
			Category = excluded.Category,
			Published = excluded.Published,
			UpdatedAt = excluded.UpdatedAt`,
		draft.Title, draft.Body, draft.Category, published,
	)
	return err
}

// DeleteDraft clears the single draft slot. Deleting a missing row is not an
// error.
func (m *Model) DeleteDraft() error {
	_, err := m.DB.Exec(`DELETE FROM BlogDraft WHERE id = 1`)
	return err
}
