package server

import (
	"database/sql"
	"errors"
)

// schemaVersion is bumped whenever any model's EnsureSchema changes, so a fresh
// container can skip the idempotent DDL when the database already has the
// current schema. Bump it in the same change as the schema: a stale version
// silently skips new migrations on existing databases.
const schemaVersion = "2026-09-25"

// schemaEnsurer runs a model's idempotent schema setup.
type schemaEnsurer interface {
	EnsureSchema() error
}

// EnsureSchema runs each model's idempotent schema setup unless the database
// already records this build's schemaVersion, which skips ~16 D1 round trips on
// every container cold start.
func EnsureSchema(db *sql.DB, models ...schemaEnsurer) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS SchemaMeta (Key TEXT PRIMARY KEY, Value TEXT NOT NULL)`); err != nil {
		return err
	}

	var recorded string
	err := db.QueryRow(`SELECT Value FROM SchemaMeta WHERE Key = 'schema_version'`).Scan(&recorded)
	if err == nil {
		if recorded == schemaVersion {
			return nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	for _, model := range models {
		if err := model.EnsureSchema(); err != nil {
			return err
		}
	}

	_, err = db.Exec(`INSERT OR REPLACE INTO SchemaMeta (Key, Value) VALUES ('schema_version', ?)`, schemaVersion)
	return err
}
