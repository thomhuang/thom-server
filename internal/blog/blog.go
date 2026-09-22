package blog

import (
	"database/sql"

	"thom-server/internal/data"
)

// Category is a post category. Its id is a slug derived from the display name,
// so category links are stable and readable.
type Category struct {
	ID        string `json:"id"`
	Category  string `json:"category"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// Post is a plain-text blog post. CategoryID always points at a BlogCategories
// row; Category is the canonical display name for convenience.
type Post struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	CategoryID string `json:"categoryId"`
	Category   string `json:"category"`
	Published  bool   `json:"published"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type Model struct {
	DB *sql.DB
}

type scanner interface {
	Scan(dest ...any) error
}

// SlugifyName turns a display name into the stable id used by the category
// lookup: lowercase alphanumerics separated by single dashes.
func SlugifyName(value string) string {
	return data.SlugifyName(value)
}
