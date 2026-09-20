package coffee

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestModelEnsureSchemaAddsCoffeeMetadataColumns(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
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
			DaysSinceRoast INTEGER NOT NULL DEFAULT 0,
			RoasterID TEXT NOT NULL,
			Roaster TEXT NOT NULL,
			BrewMethod TEXT NOT NULL,
			Ratio TEXT NOT NULL,
			Grinder TEXT NOT NULL,
			GrindSetting REAL NOT NULL DEFAULT 0.0,
			Dose INTEGER NOT NULL DEFAULT 0,
			YieldAmount INTEGER NOT NULL DEFAULT 0,
			WaterTemperature INTEGER NOT NULL DEFAULT 0,
			BrewTime TEXT NOT NULL DEFAULT '',
			BloomTime TEXT NOT NULL DEFAULT '',
			BloomWater INTEGER NOT NULL DEFAULT 0,
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
		RoasterID:        "shoebox",
		Roaster:          "Shoebox",
		BrewMethod:       "v60",
		Ratio:            "1:16",
		Grinder:          "fellow-ode",
		GrindSetting:     4.2,
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
	db, err := sql.Open("sqlite", ":memory:")
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
			DaysSinceRoast INTEGER NOT NULL DEFAULT 0,
			RoasterID TEXT NOT NULL,
			Roaster TEXT NOT NULL,
			BrewMethod TEXT NOT NULL,
			Ratio TEXT NOT NULL,
			Grinder TEXT NOT NULL,
			GrindSetting REAL NOT NULL DEFAULT 0.0,
			Dose INTEGER NOT NULL DEFAULT 0,
			YieldAmount INTEGER NOT NULL DEFAULT 0,
			WaterTemperature INTEGER NOT NULL DEFAULT 0,
			BrewTime TEXT NOT NULL DEFAULT '',
			BloomTime TEXT NOT NULL DEFAULT '',
			BloomWater INTEGER NOT NULL DEFAULT 0,
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
			'2026-05-21', 'Legacy Sey Lot', 8, 'shoebox', 'Shoebox',
			'v60', '1:16', 'fellow-ode', '4.2', 'stone fruit'
		);`
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	model := &Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	roaster, err := model.GetRoasterByName("Shoebox")
	if err != nil {
		t.Fatal(err)
	}
	if roaster.ID != "shoebox" {
		t.Fatalf("expected seeded roaster ID shoebox, got %q", roaster.ID)
	}
}

func TestModelEnsureSchemaBackfillsGrinders(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
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
			DaysSinceRoast INTEGER NOT NULL DEFAULT 0,
			RoasterID TEXT NOT NULL,
			Roaster TEXT NOT NULL,
			BrewMethod TEXT NOT NULL,
			Ratio TEXT NOT NULL,
			Grinder TEXT NOT NULL,
			GrindSetting REAL NOT NULL DEFAULT 0.0,
			Dose INTEGER NOT NULL DEFAULT 0,
			YieldAmount INTEGER NOT NULL DEFAULT 0,
			WaterTemperature INTEGER NOT NULL DEFAULT 0,
			BrewTime TEXT NOT NULL DEFAULT '',
			BloomTime TEXT NOT NULL DEFAULT '',
			BloomWater INTEGER NOT NULL DEFAULT 0,
			PourNotes TEXT NOT NULL DEFAULT '',
			RoastLevel TEXT NOT NULL DEFAULT '',
			Notes TEXT NOT NULL,
			Rating INTEGER NOT NULL DEFAULT 0,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);
		INSERT INTO CoffeeEntries (
			BrewDate, CoffeeName, RoasterID, Roaster, BrewMethod, Ratio, Grinder, Notes
		)
		VALUES (
			'2026-05-21', 'Legacy Kenya Lot', 'shoebox', 'Shoebox',
			'v60', '1:16', 'fellow-ode', 'blackcurrant'
		);`
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	model := &Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	grinder, err := model.GetGrinderByID("fellow-ode")
	if err != nil {
		t.Fatal(err)
	}
	if grinder.Grinder != "fellow-ode" {
		t.Fatalf("expected backfilled grinder name, got %q", grinder.Grinder)
	}

	entry, err := model.GetByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if entry.GrinderID != "fellow-ode" {
		t.Fatalf("expected backfilled GrinderID fellow-ode, got %q", entry.GrinderID)
	}
}
