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

	h.state.RevokeToken(token, time.Unix(claims.ExpiresAt, 0))

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

func TestAuthCookieAttributes(t *testing.T) {
	cases := []struct {
		name          string
		secureCookies bool
		wantSecure    bool
		wantName      string
	}{
		{name: "local", secureCookies: false, wantSecure: false, wantName: defaultAuthCookieName},
		{name: "secure", secureCookies: true, wantSecure: true, wantName: "__Host-" + defaultAuthCookieName},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := newTestAuthHandler(Config{SecureCookies: tc.secureCookies})
			recorder := httptest.NewRecorder()
			handler.setAuthCookie(recorder, "token-value", 3600)

			cookies := recorder.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("got %d cookies, want 1", len(cookies))
			}
			cookie := cookies[0]

			if cookie.Name != tc.wantName {
				t.Errorf("name = %q, want %q", cookie.Name, tc.wantName)
			}
			if !cookie.HttpOnly {
				t.Error("cookie must be HttpOnly")
			}
			if cookie.Secure != tc.wantSecure {
				t.Errorf("secure = %v, want %v", cookie.Secure, tc.wantSecure)
			}
			if cookie.SameSite != http.SameSiteLaxMode {
				t.Errorf("same-site = %v, want Lax", cookie.SameSite)
			}
			if cookie.Path != "/" {
				t.Errorf("path = %q, want /", cookie.Path)
			}
		})
	}
}

func TestLoginThrottleKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.RemoteAddr = "10.0.0.1:5555"

	if got, want := loginThrottleKey(r, "Admin"), "10.0.0.1|admin"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	r.Header.Set("CF-Connecting-IP", "203.0.113.7")
	if got, want := loginThrottleKey(r, "Admin"), "203.0.113.7|admin"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
