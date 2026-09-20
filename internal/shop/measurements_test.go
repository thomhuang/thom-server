package shop

import (
	"strconv"
	"strings"
	"testing"
)

func TestModelMeasurementRoundTripPreservesDecimal(t *testing.T) {
	model := newTestModel(t)

	created, err := model.InsertItem(&Item{
		Title:      "Clothing listing",
		Category:   "pants",
		Size:       "36x32",
		PriceCents: 1800,
		Currency:   "usd",
		Stock:      1,
		Measurements: []*Measurement{
			{Label: "Waist", ValueInches: 24.5},
			{Label: "Inseam", ValueInches: 22.5},
			// 18.25 is stored exactly even though the UI only shows one decimal.
			{Label: "Rise", ValueInches: 18.25},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := measurementValue(created, "Waist"); got != 24.5 {
		t.Fatalf("waist = %v, want 24.5", got)
	}
	if got := measurementValue(created, "Inseam"); got != 22.5 {
		t.Fatalf("inseam = %v, want 22.5", got)
	}
	if got := measurementValue(created, "Rise"); got != 18.25 {
		t.Fatalf("rise = %v, want 18.25", got)
	}
	if created.Category != "pants" {
		t.Fatalf("category = %q, want pants", created.Category)
	}
	if created.Size != "36x32" {
		t.Fatalf("size = %q, want 36x32", created.Size)
	}

	reloadedID, err := strconv.Atoi(created.ID)
	if err != nil {
		t.Fatal(err)
	}

	reloaded, err := model.GetItemByID(reloadedID)
	if err != nil {
		t.Fatal(err)
	}
	if got := measurementValue(reloaded, "Waist"); got != 24.5 {
		t.Fatalf("reloaded waist = %v, want 24.5", got)
	}
	if got := measurementValue(reloaded, "Inseam"); got != 22.5 {
		t.Fatalf("reloaded inseam = %v, want 22.5", got)
	}
}

func TestModelMeasurementsDefaultToEmpty(t *testing.T) {
	model := newTestModel(t)

	// A non-clothing listing simply omits the measurements.
	created, err := model.InsertItem(&Item{
		Title:      "Non-clothing listing",
		PriceCents: 500,
		Currency:   "usd",
		Stock:      1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(created.Measurements) != 0 {
		t.Fatalf("measurements = %+v, want none", created.Measurements)
	}
}

func TestModelUpdateItemReplacesMeasurements(t *testing.T) {
	model := newTestModel(t)

	item, err := model.GetItemByID(1)
	if err != nil {
		t.Fatal(err)
	}

	item.Measurements = []*Measurement{
		{Label: "Waist", ValueInches: 21.5},
		{Label: "Inseam", ValueInches: 27.5},
	}

	updated, err := model.UpdateItem(1, item)
	if err != nil {
		t.Fatal(err)
	}

	if got := measurementValue(updated, "Waist"); got != 21.5 {
		t.Fatalf("waist = %v, want 21.5", got)
	}
	if got := measurementValue(updated, "Inseam"); got != 27.5 {
		t.Fatalf("inseam = %v, want 27.5", got)
	}

	// A later update with an empty set clears them.
	updated.Measurements = nil
	cleared, err := model.UpdateItem(1, updated)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared.Measurements) != 0 {
		t.Fatalf("measurements = %+v, want cleared", cleared.Measurements)
	}
}

// measurementValue returns the stored value for a label, or -1 when absent.
func measurementValue(item *Item, label string) float64 {
	for _, measurement := range item.Measurements {
		if strings.EqualFold(measurement.Label, label) {
			return measurement.ValueInches
		}
	}

	return -1
}
