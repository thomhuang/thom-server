package coffee

import (
	"errors"
	"testing"

	"thom-server/internal/data"
)

func TestModelGetAll(t *testing.T) {
	model := newTestModel(t)

	entries, err := model.GetAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 coffee entry, got %d", len(entries))
	}
	if entries[0].ID != "1" {
		t.Fatalf("expected entry ID 1, got %q", entries[0].ID)
	}
	if entries[0].TastingNotes != "floral, citrus, honey" {
		t.Fatalf("expected tasting notes from Notes, got %q", entries[0].TastingNotes)
	}
	if entries[0].Origin != "Yirgacheffe, Ethiopia" {
		t.Fatalf("expected origin, got %q", entries[0].Origin)
	}
	if entries[0].CoffeeVarietal != "Heirloom" {
		t.Fatalf("expected coffee varietal, got %q", entries[0].CoffeeVarietal)
	}
	if entries[0].ProcessingMethod != "Washed" {
		t.Fatalf("expected processing method, got %q", entries[0].ProcessingMethod)
	}
}

func TestModelGetByID(t *testing.T) {
	model := newTestModel(t)

	entry, err := model.GetByID(1)
	if err != nil {
		t.Fatal(err)
	}

	if entry.CoffeeName != "Ethiopia Test Lot" {
		t.Fatalf("expected Ethiopia Test Lot, got %q", entry.CoffeeName)
	}
	if entry.TastingNotes != entry.Notes {
		t.Fatalf("expected tasting notes to mirror notes")
	}
	if entry.Origin != "Yirgacheffe, Ethiopia" {
		t.Fatalf("expected origin, got %q", entry.Origin)
	}
	if entry.CoffeeVarietal != "Heirloom" {
		t.Fatalf("expected coffee varietal, got %q", entry.CoffeeVarietal)
	}
	if entry.ProcessingMethod != "Washed" {
		t.Fatalf("expected processing method, got %q", entry.ProcessingMethod)
	}
}

func TestModelGetByIDNoRecord(t *testing.T) {
	model := newTestModel(t)

	_, err := model.GetByID(999)
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestModelInsert(t *testing.T) {
	model := newTestModel(t)

	entry, err := model.Insert(&Entry{
		Date:             "2026-05-21",
		CoffeeName:       "Colombia Test Lot",
		Origin:           "Huila, Colombia",
		CoffeeVarietal:   "Caturra",
		ProcessingMethod: "Honey",
		DaysSinceRoast:   8,
		RoasterID:        "shoebox",
		Roaster:          "Shoebox",
		BrewMethod:       "kalita-wave",
		Ratio:            "1:15",
		Grinder:          "comandante-c40",
		GrindSetting:     24,
		Dose:             18,
		YieldAmount:      270,
		WaterTemperature: 202,
		BrewTime:         "3:05",
		BloomTime:        "40s",
		BloomWater:       45,
		PourNotes:        "Steady pulse pours",
		RoastLevel:       "light-medium",
		Notes:            "red fruit and caramel",
		Rating:           4,
	})
	if err != nil {
		t.Fatal(err)
	}

	if entry.ID == "" {
		t.Fatal("expected inserted entry ID")
	}
	if entry.CreatedAt == "" {
		t.Fatal("expected created timestamp")
	}
	if entry.TastingNotes != "red fruit and caramel" {
		t.Fatalf("expected tasting notes to mirror notes, got %q", entry.TastingNotes)
	}
	if entry.Origin != "Huila, Colombia" {
		t.Fatalf("expected origin to persist, got %q", entry.Origin)
	}
	if entry.CoffeeVarietal != "Caturra" {
		t.Fatalf("expected coffee varietal to persist, got %q", entry.CoffeeVarietal)
	}
	if entry.ProcessingMethod != "Honey" {
		t.Fatalf("expected processing method to persist, got %q", entry.ProcessingMethod)
	}

	roaster, err := model.GetRoasterByID("shoebox")
	if err != nil {
		t.Fatal(err)
	}
	if roaster.Roaster != "Shoebox" {
		t.Fatalf("expected Shoebox roaster to be persisted, got %q", roaster.Roaster)
	}
}

func TestModelUpdate(t *testing.T) {
	model := newTestModel(t)

	entry, err := model.Update(1, &Entry{
		Date:             "2026-05-21",
		CoffeeName:       "Colombia Test Lot",
		Origin:           "Huila, Colombia",
		CoffeeVarietal:   "Caturra",
		ProcessingMethod: "Honey",
		DaysSinceRoast:   8,
		RoasterID:        "shoebox",
		Roaster:          "Shoebox",
		BrewMethod:       "kalita-wave",
		Ratio:            "1:15",
		Grinder:          "comandante-c40",
		GrindSetting:     24,
		Dose:             18,
		YieldAmount:      270,
		WaterTemperature: 202,
		BrewTime:         "3:05",
		BloomTime:        "40s",
		BloomWater:       45,
		PourNotes:        "Steady pulse pours",
		RoastLevel:       "light-medium",
		Notes:            "red fruit and caramel",
		Rating:           4,
	})
	if err != nil {
		t.Fatal(err)
	}

	if entry.ID != "1" {
		t.Fatalf("expected entry ID 1, got %q", entry.ID)
	}
	if entry.CoffeeName != "Colombia Test Lot" {
		t.Fatalf("expected updated coffee name, got %q", entry.CoffeeName)
	}
	if entry.Origin != "Huila, Colombia" {
		t.Fatalf("expected updated origin, got %q", entry.Origin)
	}
	if entry.CoffeeVarietal != "Caturra" {
		t.Fatalf("expected updated coffee varietal, got %q", entry.CoffeeVarietal)
	}
	if entry.ProcessingMethod != "Honey" {
		t.Fatalf("expected updated processing method, got %q", entry.ProcessingMethod)
	}
}

func TestModelUpdateNoRecord(t *testing.T) {
	model := newTestModel(t)

	_, err := model.Update(999, &Entry{
		Date:         "2026-05-21",
		CoffeeName:   "Colombia Test Lot",
		RoasterID:    "shoebox",
		Roaster:      "Shoebox",
		BrewMethod:   "kalita-wave",
		Ratio:        "1:15",
		Grinder:      "comandante-c40",
		GrindSetting: 24,
		Notes:        "red fruit and caramel",
		Rating:       4,
	})
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestModelDelete(t *testing.T) {
	model := newTestModel(t)

	if err := model.Delete(1); err != nil {
		t.Fatal(err)
	}

	_, err := model.GetByID(1)
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected deleted entry to be missing, got %v", err)
	}
}

func TestModelDeleteNoRecord(t *testing.T) {
	model := newTestModel(t)

	err := model.Delete(999)
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}
