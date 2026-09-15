package server

import (
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"thom-server/internal/d1"
)

const DefaultDBPath = "./internal/thom.db"

func DBPathFromEnv() string {
	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if dbPath == "" {
		return DefaultDBPath
	}

	return dbPath
}

func PrepareDBFile(dbPath, seedPath string) error {
	if dbPath == "" || dbPath == seedPath {
		return nil
	}

	if samePath(dbPath, seedPath) {
		return nil
	}

	if _, err := os.Stat(dbPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	source, err := os.Open(seedPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer source.Close()

	if err = os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return err
	}

	tempPath := dbPath + ".tmp"
	destination, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		_ = os.Remove(tempPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return closeErr
	}

	return os.Rename(tempPath, dbPath)
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	return errA == nil && errB == nil && absA == absB
}

// D1ConfigFromEnv reports the Cloudflare D1 settings when every required
// variable is present. Deployments that set these use D1 instead of SQLite.
func D1ConfigFromEnv() (d1.Config, bool) {
	accountID := strings.TrimSpace(os.Getenv("D1_ACCOUNT_ID"))
	databaseID := strings.TrimSpace(os.Getenv("D1_DATABASE_ID"))
	apiToken := strings.TrimSpace(os.Getenv("CF_API_TOKEN"))
	if accountID == "" || databaseID == "" || apiToken == "" {
		return d1.Config{}, false
	}

	return d1.Config{
		AccountID:  accountID,
		DatabaseID: databaseID,
		APIToken:   apiToken,
		Endpoint:   strings.TrimSpace(os.Getenv("D1_ENDPOINT")),
	}, true
}

// UsingD1 reports whether the server should talk to Cloudflare D1.
func UsingD1() bool {
	_, ok := D1ConfigFromEnv()
	return ok
}

// OpenAppDB opens Cloudflare D1 when it is configured, otherwise the local
// SQLite file used for development and tests.
func OpenAppDB(dbPath string) (*sql.DB, error) {
	if cfg, ok := D1ConfigFromEnv(); ok {
		return openD1(cfg)
	}

	return OpenDB(dbPath)
}

func openD1(cfg d1.Config) (*sql.DB, error) {
	db := sql.OpenDB(d1.NewConnector(cfg))
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func OpenDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}

	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
