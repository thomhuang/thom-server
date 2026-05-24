package coffee

import (
	"database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"

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

func TestModelEnsureSchemaAddsCoffeeMetadataColumns(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})
	db.SetMaxOpenConns(1)

	schema := `
		CREATE TABLE CoffeeEntries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			BrewDate TEXT NOT NULL,
			CoffeeName TEXT NOT NULL,
			DaysSinceRoast TEXT NOT NULL DEFAULT '',
			RoasterID TEXT NOT NULL,
			Roaster TEXT NOT NULL,
			BrewMethod TEXT NOT NULL,
			Ratio TEXT NOT NULL,
			Grinder TEXT NOT NULL,
			GrindSetting TEXT NOT NULL,
			Dose TEXT NOT NULL DEFAULT '',
			YieldAmount TEXT NOT NULL DEFAULT '',
			WaterTemperature TEXT NOT NULL DEFAULT '',
			BrewTime TEXT NOT NULL DEFAULT '',
			BloomTime TEXT NOT NULL DEFAULT '',
			BloomWater TEXT NOT NULL DEFAULT '',
			PourNotes TEXT NOT NULL DEFAULT '',
			RoastLevel TEXT NOT NULL DEFAULT '',
			Notes TEXT NOT NULL,
			Rating INTEGER NOT NULL DEFAULT 0,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);`
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	model := &Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	entry, err := model.Insert(&Entry{
		Date:             "2026-05-21",
		CoffeeName:       "Tanzania Test Lot",
		Origin:           "Mbeya, Tanzania",
		CoffeeVarietal:   "Bourbon",
		ProcessingMethod: "Washed",
		RoasterID:        "sey-coffee",
		Roaster:          "Sey Coffee",
		BrewMethod:       "v60",
		Ratio:            "1:16",
		Grinder:          "fellow-ode",
		GrindSetting:     "4.2",
		Notes:            "stone fruit",
		Rating:           5,
	})
	if err != nil {
		t.Fatal(err)
	}

	if entry.Origin != "Mbeya, Tanzania" {
		t.Fatalf("expected origin to persist after migration, got %q", entry.Origin)
	}
	if entry.CoffeeVarietal != "Bourbon" {
		t.Fatalf("expected varietal to persist after migration, got %q", entry.CoffeeVarietal)
	}
	if entry.ProcessingMethod != "Washed" {
		t.Fatalf("expected processing method to persist after migration, got %q", entry.ProcessingMethod)
	}
}

func TestModelEnsureSchemaHandlesSeedRoasterNameConflicts(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})
	db.SetMaxOpenConns(1)

	schema := `
		CREATE TABLE CoffeeEntries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			BrewDate TEXT NOT NULL,
			CoffeeName TEXT NOT NULL,
			DaysSinceRoast TEXT NOT NULL DEFAULT '',
			RoasterID TEXT NOT NULL,
			Roaster TEXT NOT NULL,
			BrewMethod TEXT NOT NULL,
			Ratio TEXT NOT NULL,
			Grinder TEXT NOT NULL,
			GrindSetting TEXT NOT NULL,
			Dose TEXT NOT NULL DEFAULT '',
			YieldAmount TEXT NOT NULL DEFAULT '',
			WaterTemperature TEXT NOT NULL DEFAULT '',
			BrewTime TEXT NOT NULL DEFAULT '',
			BloomTime TEXT NOT NULL DEFAULT '',
			BloomWater TEXT NOT NULL DEFAULT '',
			PourNotes TEXT NOT NULL DEFAULT '',
			RoastLevel TEXT NOT NULL DEFAULT '',
			Notes TEXT NOT NULL,
			Rating INTEGER NOT NULL DEFAULT 0,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);
		INSERT INTO CoffeeEntries (
			BrewDate, CoffeeName, DaysSinceRoast, RoasterID, Roaster,
			BrewMethod, Ratio, Grinder, GrindSetting, Notes
		)
		VALUES (
			'2026-05-21', 'Legacy Sey Lot', '8', 'sey', 'Sey Coffee',
			'v60', '1:16', 'fellow-ode', '4.2', 'stone fruit'
		);`
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	model := &Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	roaster, err := model.GetRoasterByName("Sey Coffee")
	if err != nil {
		t.Fatal(err)
	}
	if roaster.ID != "sey-coffee" {
		t.Fatalf("expected seeded roaster ID sey-coffee, got %q", roaster.ID)
	}
}

func TestModelGetRoasters(t *testing.T) {
	model := newTestModel(t)

	roasters, err := model.GetRoasters()
	if err != nil {
		t.Fatal(err)
	}

	if len(roasters) != 4 {
		t.Fatalf("expected 4 seeded roasters, got %d", len(roasters))
	}
	if roasters[0].ID != "sey-coffee" {
		t.Fatalf("expected first roaster sey-coffee, got %q", roasters[0].ID)
	}
}

func TestModelUpsertRoaster(t *testing.T) {
	model := newTestModel(t)

	roaster, err := model.UpsertRoaster(&Roaster{ID: "dak-coffee-roasters", Roaster: "DAK Coffee Roasters"})
	if err != nil {
		t.Fatal(err)
	}

	if roaster.ID != "dak-coffee-roasters" {
		t.Fatalf("expected roaster ID dak-coffee-roasters, got %q", roaster.ID)
	}
	if roaster.CreatedAt == "" {
		t.Fatal("expected created timestamp")
	}
}

func TestModelUpsertRoasterReturnsExistingNameConflict(t *testing.T) {
	model := newTestModel(t)

	roaster, err := model.UpsertRoaster(&Roaster{ID: "sey", Roaster: "Sey Coffee"})
	if err != nil {
		t.Fatal(err)
	}

	if roaster.ID != "sey-coffee" {
		t.Fatalf("expected existing roaster ID sey-coffee, got %q", roaster.ID)
	}
}

func TestModelUpsertRoasterReturnsExistingIDConflict(t *testing.T) {
	model := newTestModel(t)

	roaster, err := model.UpsertRoaster(&Roaster{ID: "sey-coffee", Roaster: "Renamed Roaster"})
	if err != nil {
		t.Fatal(err)
	}

	if roaster.Roaster != "Sey Coffee" {
		t.Fatalf("expected existing roaster name Sey Coffee, got %q", roaster.Roaster)
	}

	roaster, err = model.GetRoasterByID("sey-coffee")
	if err != nil {
		t.Fatal(err)
	}
	if roaster.Roaster != "Sey Coffee" {
		t.Fatalf("expected stored roaster name to remain Sey Coffee, got %q", roaster.Roaster)
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
		DaysSinceRoast:   "8",
		RoasterID:        "heart-coffee",
		Roaster:          "Heart Coffee",
		BrewMethod:       "kalita-wave",
		Ratio:            "1:15",
		Grinder:          "comandante-c40",
		GrindSetting:     "24",
		Dose:             "18g",
		YieldAmount:      "270g",
		WaterTemperature: "202F",
		BrewTime:         "3:05",
		BloomTime:        "40s",
		BloomWater:       "45g",
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

	roaster, err := model.GetRoasterByID("heart-coffee")
	if err != nil {
		t.Fatal(err)
	}
	if roaster.Roaster != "Heart Coffee" {
		t.Fatalf("expected Heart Coffee roaster to be persisted, got %q", roaster.Roaster)
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
		DaysSinceRoast:   "8",
		RoasterID:        "heart-coffee",
		Roaster:          "Heart Coffee",
		BrewMethod:       "kalita-wave",
		Ratio:            "1:15",
		Grinder:          "comandante-c40",
		GrindSetting:     "24",
		Dose:             "18g",
		YieldAmount:      "270g",
		WaterTemperature: "202F",
		BrewTime:         "3:05",
		BloomTime:        "40s",
		BloomWater:       "45g",
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
		RoasterID:    "heart-coffee",
		Roaster:      "Heart Coffee",
		BrewMethod:   "kalita-wave",
		Ratio:        "1:15",
		Grinder:      "comandante-c40",
		GrindSetting: "24",
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

func newTestModel(t *testing.T) *Model {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}

	model := &Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	fixture := `
		INSERT INTO CoffeeEntries (
			id, BrewDate, CoffeeName, Origin, CoffeeVarietal, ProcessingMethod,
			DaysSinceRoast, RoasterID, Roaster, BrewMethod, Ratio, Grinder,
			GrindSetting, Dose, YieldAmount, WaterTemperature, BrewTime,
			BloomTime, BloomWater, PourNotes, RoastLevel, Notes, Rating, CreatedAt
		)
		VALUES (
			1, '2026-05-20', 'Ethiopia Test Lot', 'Yirgacheffe, Ethiopia',
			'Heirloom', 'Washed', '10', 'sey-coffee', 'Sey Coffee',
			'v60', '1:16', 'fellow-ode', '4.2', '20g', '320g', '203F',
			'3:20', '45s', '50g', 'Two-pour finish', 'light',
			'floral, citrus, honey', 5, '2026-05-20 12:00:00'
		);`
	if _, err = db.Exec(fixture); err != nil {
		t.Fatal(err)
	}

	return model
}
