package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAuth(t *testing.T) {
	app := newTestHandler(t)
	token, err := app.createAuthToken("admin")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	rr := httptest.NewRecorder()

	app.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := claimsFromRequest(r)
		if !ok {
			t.Fatal("expected auth claims in request context")
		}
		if claims.Username != "admin" {
			t.Fatalf("expected username admin, got %q", claims.Username)
		}
		w.WriteHeader(http.StatusAccepted)
	})(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rr.Code)
	}
}

func TestRequireAuthRejectsMissingOrInvalidCookie(t *testing.T) {
	app := newTestHandler(t)

	tests := []struct {
		name   string
		cookie *http.Cookie
	}{
		{
			name: "missing cookie",
		},
		{
			name:   "invalid cookie",
			cookie: &http.Cookie{Name: authCookieName, Value: "bad-token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rr := httptest.NewRecorder()

			app.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("next handler should not be called")
			})(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
			}
		})
	}
}

func TestRequireAuthRejectsMutatingRequestWithoutAllowedOrigin(t *testing.T) {
	app := newTestHandler(t)
	token, err := app.createAuthToken("admin")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		origin string
	}{
		{name: "missing origin"},
		{name: "unconfigured origin", origin: "https://evil.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/coffee", nil)
			req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			rr := httptest.NewRecorder()

			app.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
				t.Fatal("next handler should not be called")
			})(rr, req)

			if rr.Code != http.StatusForbidden {
				t.Fatalf("expected status %d, got %d", http.StatusForbidden, rr.Code)
			}
		})
	}
}
