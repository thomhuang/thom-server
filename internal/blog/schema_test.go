package blog

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestModelEnsureSchemaAddsCategoryID(t *testing.T) {
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

	// A pre-category BlogPosts table must migrate in place, and its existing
	// rows must read back the new column as empty.
	legacy := `
		CREATE TABLE BlogPosts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			Title TEXT NOT NULL,
			Body TEXT NOT NULL DEFAULT '',
			Published INTEGER NOT NULL DEFAULT 0,
			CreatedAt TEXT NOT NULL DEFAULT (datetime('now')),
			UpdatedAt TEXT NOT NULL DEFAULT (datetime('now'))
		);
		INSERT INTO BlogPosts (Title, Body) VALUES ('Legacy post', 'body');`
	if _, err = db.Exec(legacy); err != nil {
		t.Fatal(err)
	}

	model := &Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema on a legacy BlogPosts failed: %v", err)
	}
	if err = model.EnsureSchema(); err != nil {
		t.Fatalf("second EnsureSchema failed: %v", err)
	}

	created, err := model.Insert(&Post{Title: "New post", Category: "Coffee"})
	if err != nil {
		t.Fatal(err)
	}
	if created.CategoryID != "coffee" {
		t.Fatalf("expected category id coffee, got %q", created.CategoryID)
	}
}
