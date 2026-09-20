package coffee

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestModel(t *testing.T) *Model {
	t.Helper()

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
	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}

	model := &Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	if _, err = db.Exec(`INSERT OR IGNORE INTO CoffeeRoasters (id, Roaster, SortOrder) VALUES ('shoebox', 'Shoebox', 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT OR IGNORE INTO CoffeeGrinders (id, Grinder, SortOrder) VALUES ('fellow-ode', 'Fellow Ode', 20)`); err != nil {
		t.Fatal(err)
	}

	fixture := `
		INSERT INTO CoffeeEntries (
			id, BrewDate, CoffeeName, Origin, CoffeeVarietal, ProcessingMethod,
			DaysSinceRoast, RoasterID, Roaster, BrewMethod, Ratio, GrinderID, Grinder,
			GrindSetting, Dose, YieldAmount, WaterTemperature, BrewTime,
			BloomTime, BloomWater, PourNotes, RoastLevel, Notes, Rating, CreatedAt
		)
		VALUES (
			1, '2026-05-20', 'Ethiopia Test Lot', 'Yirgacheffe, Ethiopia',
			'Heirloom', 'Washed', 10, 'shoebox', 'Shoebox',
			'v60', '1:16', 'fellow-ode', 'fellow-ode', 4.2, 20, 320, 203,
			'3:20', '45s', 50, 'Two-pour finish', 'light',
			'floral, citrus, honey', 5, '2026-05-20 12:00:00'
		);`
	if _, err = db.Exec(fixture); err != nil {
		t.Fatal(err)
	}

	return model
}
