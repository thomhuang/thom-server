package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDB(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "thom.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var foreignKeys int
	if err = db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("expected foreign keys to be enabled, got %d", foreignKeys)
	}
}

func TestPrepareDBFileCopiesSeedWhenDestinationIsMissing(t *testing.T) {
	dir := t.TempDir()
	seedPath := filepath.Join(dir, "seed.db")
	dbPath := filepath.Join(dir, "data", "thom.db")

	if err := os.WriteFile(seedPath, []byte("seed database"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := PrepareDBFile(dbPath, seedPath); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "seed database" {
		t.Fatalf("expected copied seed database, got %q", got)
	}
}

func TestPrepareDBFileDoesNotOverwriteExistingDestination(t *testing.T) {
	dir := t.TempDir()
	seedPath := filepath.Join(dir, "seed.db")
	dbPath := filepath.Join(dir, "thom.db")

	if err := os.WriteFile(seedPath, []byte("seed database"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, []byte("existing database"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := PrepareDBFile(dbPath, seedPath); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "existing database" {
		t.Fatalf("expected existing database to remain unchanged, got %q", got)
	}
}
