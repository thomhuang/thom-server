package authstate

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestModel(t *testing.T, path string) *Model {
	t.Helper()

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	db.SetMaxOpenConns(1)

	model := &Model{DB: db}
	if err := model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	return model
}

func TestLoginFailuresLockAndClear(t *testing.T) {
	model := newTestModel(t, ":memory:")
	const key = "1.2.3.4|admin"

	for attempt := 0; attempt < LoginFailureLimit; attempt++ {
		if err := model.RecordLoginFailure(key); err != nil {
			t.Fatal(err)
		}
	}

	until, locked, err := model.LoginLockedUntil(key)
	if err != nil {
		t.Fatal(err)
	}
	if !locked {
		t.Fatal("expected the key to lock once the failure limit is reached")
	}
	if !until.After(time.Now()) {
		t.Fatal("expected the lockout to be in the future")
	}

	if err := model.ClearLoginFailures(key); err != nil {
		t.Fatal(err)
	}

	if _, locked, err = model.LoginLockedUntil(key); err != nil {
		t.Fatal(err)
	} else if locked {
		t.Fatal("expected a cleared key to be unlocked")
	}
}

func TestRevokedTokenStaysRevokedUntilExpiry(t *testing.T) {
	model := newTestModel(t, ":memory:")

	if err := model.RevokeToken("token-value", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	revoked, err := model.IsTokenRevoked("token-value")
	if err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Fatal("expected the token to be revoked")
	}

	revoked, err = model.IsTokenRevoked("other-token")
	if err != nil {
		t.Fatal(err)
	}
	if revoked {
		t.Fatal("expected an unrelated token to pass")
	}

	if err := model.RevokeToken("expired", time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if revoked, err = model.IsTokenRevoked("expired"); err != nil {
		t.Fatal(err)
	} else if revoked {
		t.Fatal("expected an already-expired revocation to be ignored")
	}
}

func TestStateSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.db")

	first := newTestModel(t, path)
	if err := first.RevokeToken("persisted", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := first.DB.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := newTestModel(t, path)
	revoked, err := reopened.IsTokenRevoked("persisted")
	if err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Fatal("expected the revocation to survive a restart")
	}
}
