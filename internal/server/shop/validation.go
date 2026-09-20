package shop

import (
	"errors"
	"fmt"
	"math"
	"strings"

	shopdata "thom-server/internal/shop"
)

const (
	maxTitleLength       = 200
	maxDescriptionLength = 5000

	// maxMeasurementInches bounds a garment measurement. It is far above any real
	// garment, so it only catches obvious typos and unit mix-ups.
	maxMeasurementInches      = 100.0
	maxMeasurementLabelLength = 60
	maxItemMeasurements       = 20
	maxCategoryLength         = 40
	maxSizeLength             = 40
)

type itemPatch struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	BrandID     *string `json:"brandId"`
	Brand       *string `json:"brand"`
	Category    *string `json:"category"`
	Size        *string `json:"size"`
	PriceCents  *int    `json:"priceCents"`
	Currency    *string `json:"currency"`
	Stock       *int    `json:"stock"`
	IsPublished *bool   `json:"isPublished"`
	// A pointer so an omitted field leaves the stored measurements untouched; an
	// empty array clears them.
	Measurements *[]*shopdata.Measurement `json:"measurements"`
}

func normalizeItem(item *shopdata.Item) {
	item.Title = strings.TrimSpace(item.Title)
	item.Description = strings.TrimSpace(item.Description)
	item.BrandID = strings.TrimSpace(item.BrandID)
	item.Brand = strings.TrimSpace(item.Brand)
	item.Category = strings.TrimSpace(item.Category)
	item.Size = strings.TrimSpace(item.Size)
	item.Currency = strings.ToLower(strings.TrimSpace(item.Currency))

	item.Measurements = normalizeMeasurements(item.Measurements)

	if item.Brand == "" {
		item.BrandID = ""
	} else if item.BrandID == "" {
		item.BrandID = shopdata.SlugifyName(item.Brand)
	}

	if item.Currency == "" {
		item.Currency = shopdata.DefaultCurrency
	}
}

// normalizeMeasurements trims labels, rounds values to a tenth, drops blank
// labels, and keeps only the first row for a repeated label so what is stored
// matches what the admin form shows.
func normalizeMeasurements(measurements []*shopdata.Measurement) []*shopdata.Measurement {
	normalized := make([]*shopdata.Measurement, 0, len(measurements))
	seen := make(map[string]bool, len(measurements))

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		label := strings.TrimSpace(measurement.Label)
		if label == "" {
			continue
		}

		key := strings.ToLower(label)
		if seen[key] {
			continue
		}
		seen[key] = true

		normalized = append(normalized, &shopdata.Measurement{
			Label:       label,
			ValueInches: roundToTenth(measurement.ValueInches),
		})
	}

	return normalized
}

func applyItemPatch(item *shopdata.Item, patch *itemPatch) {
	if patch.Title != nil {
		item.Title = *patch.Title
	}
	if patch.Description != nil {
		item.Description = *patch.Description
	}
	if patch.Brand != nil {
		item.Brand = *patch.Brand
		if patch.BrandID == nil {
			item.BrandID = ""
		}
	}
	if patch.BrandID != nil {
		item.BrandID = *patch.BrandID
	}
	if patch.Category != nil {
		item.Category = *patch.Category
	}
	if patch.Size != nil {
		item.Size = *patch.Size
	}
	if patch.PriceCents != nil {
		item.PriceCents = *patch.PriceCents
	}
	if patch.Currency != nil {
		item.Currency = *patch.Currency
	}
	if patch.Stock != nil {
		item.Stock = *patch.Stock
	}
	if patch.IsPublished != nil {
		item.IsPublished = *patch.IsPublished
	}
	if patch.Measurements != nil {
		item.Measurements = *patch.Measurements
	}
}

// roundToTenth rounds a measurement to one decimal place. The UI shows a single
// decimal (24.5), so storing more precision would only invite values that
// display differently from how they were entered.
func roundToTenth(value float64) float64 {
	return math.Round(value*10) / 10
}

func isValidItem(item *shopdata.Item) error {
	if item.Title == "" {
		return errors.New("title is required")
	}
	if len([]rune(item.Title)) > maxTitleLength {
		return fmt.Errorf("title must be at most %d characters", maxTitleLength)
	}
	if len([]rune(item.Description)) > maxDescriptionLength {
		return fmt.Errorf("description must be at most %d characters", maxDescriptionLength)
	}
	if item.PriceCents <= 0 {
		return fmt.Errorf("priceCents must be greater than zero, got %d", item.PriceCents)
	}
	if item.Stock < 0 {
		return fmt.Errorf("stock must be non-negative, got %d", item.Stock)
	}
	if !validCurrency(item.Currency) {
		return fmt.Errorf("invalid currency %q", item.Currency)
	}
	if len([]rune(item.Category)) > maxCategoryLength {
		return fmt.Errorf("category must be at most %d characters", maxCategoryLength)
	}
	if len([]rune(item.Size)) > maxSizeLength {
		return fmt.Errorf("size must be at most %d characters", maxSizeLength)
	}
	if err := validMeasurements(item.Measurements); err != nil {
		return err
	}

	return nil
}

// validMeasurements bounds the number of rows and validates each label and
// value. Absence is expressed by omitting a row, so there is no zero sentinel.
func validMeasurements(measurements []*shopdata.Measurement) error {
	if len(measurements) > maxItemMeasurements {
		return fmt.Errorf("at most %d measurements are allowed, got %d", maxItemMeasurements, len(measurements))
	}

	for _, measurement := range measurements {
		if measurement == nil {
			return errors.New("measurement must not be null")
		}

		label := strings.TrimSpace(measurement.Label)
		if label == "" {
			return errors.New("measurement label is required")
		}
		if len([]rune(label)) > maxMeasurementLabelLength {
			return fmt.Errorf("measurement label must be at most %d characters", maxMeasurementLabelLength)
		}
		if err := validMeasurementValue(label, measurement.ValueInches); err != nil {
			return err
		}
	}

	return nil
}

// validMeasurementValue accepts a plausible garment measurement in inches. NaN
// and infinity would otherwise reach the database and break JSON encoding.
func validMeasurementValue(label string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%s must be a finite number", label)
	}
	if value <= 0 {
		return fmt.Errorf("%s must be greater than zero, got %v", label, value)
	}
	if value > maxMeasurementInches {
		return fmt.Errorf("%s must be at most %v inches, got %v", label, maxMeasurementInches, value)
	}

	return nil
}

func validCurrency(currency string) bool {
	if len(currency) != 3 {
		return false
	}

	for _, character := range currency {
		if character < 'a' || character > 'z' {
			return false
		}
	}

	return true
}

func normalizeBrand(brand *shopdata.Brand) {
	brand.ID = strings.TrimSpace(brand.ID)
	brand.Brand = strings.TrimSpace(brand.Brand)

	if brand.ID == "" && brand.Brand != "" {
		brand.ID = shopdata.SlugifyName(brand.Brand)
	}
}

func isValidBrand(brand *shopdata.Brand) error {
	if brand.ID == "" {
		return errors.New("brand id is required")
	}
	if brand.Brand == "" {
		return errors.New("brand name is required")
	}

	return nil
}
