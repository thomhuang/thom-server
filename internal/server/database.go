package server

import (
	"database/sql"
	"errors"
	"os"
	"strings"

	"thom-server/internal/d1"
)

// d1ConfigFromEnv reports the Cloudflare D1 settings when every required
// variable is present.
func d1ConfigFromEnv() (d1.Config, bool) {
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

// OpenAppDB opens the configured Cloudflare D1 database. There is no local
// fallback: test and production both run on D1, and local development points at
// the test database through the same variables.
func OpenAppDB() (*sql.DB, error) {
	cfg, ok := d1ConfigFromEnv()
	if !ok {
		return nil, errors.New("D1 is not configured: set D1_ACCOUNT_ID, D1_DATABASE_ID, and CF_API_TOKEN")
	}

	return openD1(cfg)
}

func openD1(cfg d1.Config) (*sql.DB, error) {
	db := sql.OpenDB(d1.NewConnector(cfg))
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
