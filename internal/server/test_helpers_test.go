package server

import (
	"database/sql"
	"io"
	"log"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"thom-server/internal/coffee"
	"thom-server/internal/posts"
)

const testAdminPasswordHash = "$2a$04$NcZBWqKwXIayYJceXw24N.PyNMu7vrnvSgWwROy1o9AMCIa3Rdl5i"

func newTestApp(t *testing.T) *App {
	t.Helper()

	db := newTestDB(t)

	return New(
		log.New(io.Discard, "", 0),
		log.New(io.Discard, "", 0),
		&posts.Model{DB: db},
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

	schema := `
		CREATE TABLE PostCategories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			Category VARCHAR(50) NOT NULL
		);
		CREATE TABLE Posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			CategoryId int NOT NULL,
			Title VARCHAR(255) NOT NULL,
			Summary VARCHAR(255) NOT NULL,
			PathName VARCHAR(50) NOT NULL,
			Link varchar(255),
			FOREIGN KEY (CategoryId) REFERENCES PostCategories(id)
		);
		CREATE TABLE PostContent (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			PostId int,
			Text text,
			ImagePath text,
			FOREIGN KEY (PostId) REFERENCES Posts(id)
		);`
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	coffeeModel := &coffee.Model{DB: db}
	if err = coffeeModel.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	fixture := `
		INSERT INTO PostCategories (id, Category) VALUES
			(1, 'Projects'),
			(2, 'Notes');
		INSERT INTO Posts (id, CategoryId, Title, Summary, PathName, Link)
		VALUES
			(1, 1, 'Test Post', 'Summary', 'test-post', NULL),
			(2, 1, 'Linked Post', 'Link summary', 'linked-post', 'https://example.com');
		INSERT INTO PostContent (id, PostId, Text, ImagePath) VALUES
			(1, 1, 'First chunk', '/first.png'),
			(2, 1, 'Second chunk', NULL),
			(3, 2, NULL, '/linked.png');
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
