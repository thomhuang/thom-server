package server

import (
	"database/sql"
	"io"
	"log"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"thom-server/internal/coffee"
)

const testAdminPasswordHash = "$2a$04$NcZBWqKwXIayYJceXw24N.PyNMu7vrnvSgWwROy1o9AMCIa3Rdl5i"

func newTestApp(t *testing.T) *App {
	t.Helper()

	db := newTestDB(t)

	return New(
		log.New(io.Discard, "", 0),
		log.New(io.Discard, "", 0),
		&coffee.Model{DB: db},
		Config{
			AdminUsername:     "admin",
			AdminPasswordHash: testAdminPasswordHash,
			JWTSecret:         "test-secret",
			ClientOrigins:     []string{"http://localhost:3000", "https://app.example.com"},
		},
	)
}

func newTestDB(t *testing.T) *sql.DB {
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

	coffeeModel := &coffee.Model{DB: db}
	if err = coffeeModel.EnsureSchema(); err != nil {
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

	return db
}
