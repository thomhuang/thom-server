package server

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

type countingEnsurer struct {
	calls int
}

func (c *countingEnsurer) EnsureSchema() error {
	c.calls++
	return nil
}

func newSchemaTestDB(t *testing.T) *sql.DB {
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

	return db
}

func TestEnsureSchemaSkipsWhenVersionCurrent(t *testing.T) {
	db := newSchemaTestDB(t)
	model := &countingEnsurer{}

	if err := EnsureSchema(db, model); err != nil {
		t.Fatalf("first EnsureSchema: %v", err)
	}
	if model.calls != 1 {
		t.Fatalf("first run should run the model once, got %d calls", model.calls)
	}

	if err := EnsureSchema(db, model); err != nil {
		t.Fatalf("second EnsureSchema: %v", err)
	}
	if model.calls != 1 {
		t.Fatalf("second run should skip the model, got %d calls", model.calls)
	}
}

func TestEnsureSchemaRerunsWhenVersionStale(t *testing.T) {
	db := newSchemaTestDB(t)
	model := &countingEnsurer{}

	if err := EnsureSchema(db, model); err != nil {
		t.Fatalf("first EnsureSchema: %v", err)
	}

	if _, err := db.Exec(`UPDATE SchemaMeta SET Value = 'older' WHERE Key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}

	if err := EnsureSchema(db, model); err != nil {
		t.Fatalf("EnsureSchema after version change: %v", err)
	}
	if model.calls != 2 {
		t.Fatalf("stale version should re-run the model, got %d calls", model.calls)
	}
}
