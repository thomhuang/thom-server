package auth

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"thom-server/internal/server/response"
)

func newTestAuthHandler(config Config) *Handler {
	return New(config, response.Responder{}, nil, log.New(io.Discard, "", 0))
}

func TestVerifyAuthTokenRejectsRevokedToken(t *testing.T) {
	h := newTestAuthHandler(Config{JWTSecret: "test-secret"})

	token, err := h.createAuthToken("admin")
	if err != nil {
		t.Fatal(err)
	}

	claims, err := h.verifyAuthToken(token)
	if err != nil {
		t.Fatalf("expected token to verify before revocation, got %v", err)
	}

	h.tokenDenylist.revoke(token, time.Unix(claims.ExpiresAt, 0))

	if _, err = h.verifyAuthToken(token); err == nil {
		t.Fatal("expected revoked token to fail verification")
	}
}

func TestTokenDenylistPrunesExpiredEntries(t *testing.T) {
	d := newTokenDenylist()
	base := time.Unix(1000, 0)
	d.now = func() time.Time { return base }

	d.revoke("token-a", base.Add(time.Minute))
	if !d.isRevoked("token-a") {
		t.Fatal("expected token-a to be revoked")
	}

	d.now = func() time.Time { return base.Add(2 * time.Minute) }
	if d.isRevoked("token-a") {
		t.Fatal("expected expired revocation to be pruned")
	}

	d.revoke("token-b", base)
	if d.isRevoked("token-b") {
		t.Fatal("expected already-expired token not to be denied")
	}
}

func TestAuthCookieName(t *testing.T) {
	insecure := newTestAuthHandler(Config{})
	if got := insecure.authCookieName(); got != defaultAuthCookieName {
		t.Errorf("got %q, want %q", got, defaultAuthCookieName)
	}

	secure := newTestAuthHandler(Config{SecureCookies: true})
	if got := secure.authCookieName(); got != "__Host-thom_auth" {
		t.Errorf("got %q, want %q", got, "__Host-thom_auth")
	}
}

func TestLoginThrottleKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.RemoteAddr = "10.0.0.1:5555"

	if got, want := loginThrottleKey(r, "Admin"), "10.0.0.1|admin"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	r.Header.Set("Fly-Client-IP", "203.0.113.7")
	if got, want := loginThrottleKey(r, "Admin"), "203.0.113.7|admin"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
