// Package authstate persists the auth decisions that must outlive a process:
// login throttling and revoked session tokens. It runs on the same database as
// the rest of the app (Cloudflare D1 at runtime, in-memory SQLite in tests), so a
// lockout is not forgotten and a revoked token stays revoked across restarts
// and across instances.
package authstate

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

const (
	// LoginFailureLimit is how many failures inside LoginFailureWindow trigger a
	// lockout, and LoginLockoutDuration is how long that lockout lasts.
	LoginFailureLimit    = 5
	LoginFailureWindow   = 15 * time.Minute
	LoginLockoutDuration = 15 * time.Minute

	// timeFormat is fixed-width and UTC so its text sorts chronologically, which
	// the expiry prune relies on.
	timeFormat = "2006-01-02T15:04:05.000Z"
)

type Model struct {
	DB *sql.DB
}

func (m *Model) EnsureSchema() error {
	_, err := m.DB.Exec(`
		CREATE TABLE IF NOT EXISTS AuthLoginFailures (
			Key TEXT PRIMARY KEY,
			Failures INTEGER NOT NULL DEFAULT 0,
			FirstFailure TEXT NOT NULL,
			LockedUntil TEXT NOT NULL DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS AuthRevokedTokens (
			TokenHash TEXT PRIMARY KEY,
			ExpiresAt TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS AuthRevokedTokensExpiryIndex
		ON AuthRevokedTokens(ExpiresAt);`)

	return err
}

// LoginLockedUntil reports whether a throttle key is locked. An expired lock or
// a failure window that has closed is cleared so the next attempt starts fresh.
func (m *Model) LoginLockedUntil(key string) (time.Time, bool, error) {
	now := time.Now().UTC()

	var firstFailure, lockedUntil string
	err := m.DB.QueryRow(
		`SELECT FirstFailure, LockedUntil FROM AuthLoginFailures WHERE Key = ?`,
		key,
	).Scan(&firstFailure, &lockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}

	if until, ok := parseTime(lockedUntil); ok && now.Before(until) {
		return until, true, nil
	}

	first, _ := parseTime(firstFailure)
	if lockedUntil != "" || (!first.IsZero() && now.Sub(first) > LoginFailureWindow) {
		if _, err = m.DB.Exec(`DELETE FROM AuthLoginFailures WHERE Key = ?`, key); err != nil {
			return time.Time{}, false, err
		}
	}

	return time.Time{}, false, nil
}

// RecordLoginFailure counts one failed attempt, locking the key once the limit
// is reached within the window.
func (m *Model) RecordLoginFailure(key string) error {
	now := time.Now().UTC()

	var failures int
	var firstFailure string
	err := m.DB.QueryRow(
		`SELECT Failures, FirstFailure FROM AuthLoginFailures WHERE Key = ?`,
		key,
	).Scan(&failures, &firstFailure)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = m.DB.Exec(
			`INSERT INTO AuthLoginFailures (Key, Failures, FirstFailure, LockedUntil) VALUES (?, 1, ?, '')`,
			key,
			formatTime(now),
		)
		return err
	}
	if err != nil {
		return err
	}

	first, _ := parseTime(firstFailure)
	if first.IsZero() || now.Sub(first) > LoginFailureWindow {
		failures = 0
		firstFailure = formatTime(now)
	}

	failures++
	lockedUntil := ""
	if failures >= LoginFailureLimit {
		lockedUntil = formatTime(now.Add(LoginLockoutDuration))
	}

	_, err = m.DB.Exec(
		`UPDATE AuthLoginFailures SET Failures = ?, FirstFailure = ?, LockedUntil = ? WHERE Key = ?`,
		failures,
		firstFailure,
		lockedUntil,
		key,
	)

	return err
}

// ClearLoginFailures forgets a throttle key after a successful login.
func (m *Model) ClearLoginFailures(key string) error {
	_, err := m.DB.Exec(`DELETE FROM AuthLoginFailures WHERE Key = ?`, key)
	return err
}

// RevokeToken records a token as revoked until it would have expired anyway.
// Tokens are hashed so the stored value cannot be replayed.
func (m *Model) RevokeToken(token string, expiresAt time.Time) error {
	now := time.Now().UTC()

	if _, err := m.DB.Exec(
		`DELETE FROM AuthRevokedTokens WHERE ExpiresAt <= ?`, formatTime(now),
	); err != nil {
		return err
	}

	if !now.Before(expiresAt) {
		return nil
	}

	_, err := m.DB.Exec(
		`INSERT OR REPLACE INTO AuthRevokedTokens (TokenHash, ExpiresAt) VALUES (?, ?)`,
		hashToken(token),
		formatTime(expiresAt),
	)

	return err
}

// IsTokenRevoked reports whether a token was revoked and has not yet expired.
func (m *Model) IsTokenRevoked(token string) (bool, error) {
	now := time.Now().UTC()
	hash := hashToken(token)

	var expiresAt string
	err := m.DB.QueryRow(
		`SELECT ExpiresAt FROM AuthRevokedTokens WHERE TokenHash = ?`,
		hash,
	).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	expiry, ok := parseTime(expiresAt)
	if !ok || !now.Before(expiry) {
		if _, err = m.DB.Exec(`DELETE FROM AuthRevokedTokens WHERE TokenHash = ?`, hash); err != nil {
			return false, err
		}
		return false, nil
	}

	return true, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func formatTime(value time.Time) string {
	return value.UTC().Format(timeFormat)
}

func parseTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}

	parsed, err := time.Parse(timeFormat, value)
	if err != nil {
		return time.Time{}, false
	}

	return parsed, true
}
