package shop

import (
	"database/sql"
	"strings"

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

type scanner interface {
	Scan(dest ...any) error
}

type querier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// execChanged runs a write and reports whether it changed a row. The status
// guards on the order transitions and the conditional stock decrement both
// depend on that answer.
func (m *Model) execChanged(query string, args ...any) (bool, error) {
	result, err := m.DB.Exec(query, args...)
	if err != nil {
		return false, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rowsAffected > 0, nil
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
