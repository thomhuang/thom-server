package server

import (
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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
