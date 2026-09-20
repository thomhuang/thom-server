package shop

import (
	"math"
	"strings"
	"testing"

	shopdata "thom-server/internal/shop"
)

func TestIsValidItem(t *testing.T) {
	valid := shopdata.Item{Title: "Mug", PriceCents: 1800, Stock: 1, Currency: "usd"}

	testCases := []struct {
		name   string
		mutate func(*shopdata.Item)
		valid  bool
	}{
		{name: "valid", mutate: func(*shopdata.Item) {}, valid: true},
		{name: "missing title", mutate: func(i *shopdata.Item) { i.Title = "" }, valid: false},
		{name: "price must be positive", mutate: func(i *shopdata.Item) { i.PriceCents = 0 }, valid: false},
		{name: "negative price", mutate: func(i *shopdata.Item) { i.PriceCents = -100 }, valid: false},
		{name: "negative stock", mutate: func(i *shopdata.Item) { i.Stock = -1 }, valid: false},
		{name: "zero stock is allowed", mutate: func(i *shopdata.Item) { i.Stock = 0 }, valid: true},
		{name: "bad currency length", mutate: func(i *shopdata.Item) { i.Currency = "us" }, valid: false},
		{name: "uppercase currency", mutate: func(i *shopdata.Item) { i.Currency = "USD" }, valid: false},
		{name: "long title", mutate: func(i *shopdata.Item) { i.Title = strings.Repeat("a", maxTitleLength+1) }, valid: false},
		{name: "decimal measurements", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{
				{Label: "Waist", ValueInches: 24.5},
				{Label: "Inseam", ValueInches: 22.5},
			}
		}, valid: true},
		{name: "omitted measurements are allowed", mutate: func(i *shopdata.Item) {
			i.Measurements = nil
		}, valid: true},
		{name: "zero measurement is rejected", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "Waist", ValueInches: 0}}
		}, valid: false},
		{name: "negative measurement", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "Waist", ValueInches: -1}}
		}, valid: false},
		{name: "absurd measurement", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "Waist", ValueInches: maxMeasurementInches + 1}}
		}, valid: false},
		{name: "NaN measurement", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "Waist", ValueInches: math.NaN()}}
		}, valid: false},
		{name: "infinite measurement", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "Inseam", ValueInches: math.Inf(1)}}
		}, valid: false},
		{name: "blank measurement label", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "  ", ValueInches: 10}}
		}, valid: false},
		{name: "long measurement label", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{
				Label:       strings.Repeat("a", maxMeasurementLabelLength+1),
				ValueInches: 10,
			}}
		}, valid: false},
		{name: "too many measurements", mutate: func(i *shopdata.Item) {
			measurements := make([]*shopdata.Measurement, maxItemMeasurements+1)
			for n := range measurements {
				measurements[n] = &shopdata.Measurement{Label: "Waist", ValueInches: 10}
			}
			i.Measurements = measurements
		}, valid: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			item := valid
			testCase.mutate(&item)

			err := isValidItem(&item)
			if testCase.valid && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !testCase.valid && err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

func TestNormalizeItemTrimsAndDefaultsCurrency(t *testing.T) {
	item := &shopdata.Item{Title: "  Mug  ", Description: "  A mug  ", Currency: "  "}

	normalizeItem(item)

	if item.Title != "Mug" {
		t.Fatalf("title = %q, want Mug", item.Title)
	}
	if item.Description != "A mug" {
		t.Fatalf("description = %q, want A mug", item.Description)
	}
	if item.Currency != shopdata.DefaultCurrency {
		t.Fatalf("currency = %q, want %q", item.Currency, shopdata.DefaultCurrency)
	}
}

func TestNormalizeItemRoundsMeasurementsToOneDecimal(t *testing.T) {
	item := &shopdata.Item{
		Title:      "Jacket",
		PriceCents: 1000,
		Measurements: []*shopdata.Measurement{
			{Label: "  Pit to pit  ", ValueInches: 24.46},
			{Label: "Back length", ValueInches: 22.55},
			{Label: "Shoulder", ValueInches: 18.04},
		},
	}

	normalizeItem(item)

	if len(item.Measurements) != 3 {
		t.Fatalf("measurements = %+v, want 3 rows", item.Measurements)
	}
	if got := measurementByLabel(t, item.Measurements, "Pit to pit").ValueInches; got != 24.5 {
		t.Fatalf("pit-to-pit = %v, want 24.5", got)
	}
	if got := measurementByLabel(t, item.Measurements, "Back length").ValueInches; got != 22.6 {
		t.Fatalf("back length = %v, want 22.6", got)
	}
	if got := measurementByLabel(t, item.Measurements, "Shoulder").ValueInches; got != 18 {
		t.Fatalf("shoulder = %v, want 18", got)
	}
}

func TestNormalizeMeasurementsDropsBlanksAndDuplicates(t *testing.T) {
	item := &shopdata.Item{
		Measurements: []*shopdata.Measurement{
			{Label: "Waist", ValueInches: 31},
			{Label: "   ", ValueInches: 10},
			{Label: "waist", ValueInches: 99},
			nil,
			{Label: "Inseam", ValueInches: 30},
		},
	}

	normalizeItem(item)

	if len(item.Measurements) != 2 {
		t.Fatalf("measurements = %+v, want 2 rows", item.Measurements)
	}
	if got := measurementByLabel(t, item.Measurements, "Waist").ValueInches; got != 31 {
		t.Fatalf("waist = %v, want the first value 31", got)
	}
}
