package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogin(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{
		"username": "admin",
		"password": "password"
	}`))
	rr := httptest.NewRecorder()

	app.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 auth cookie, got %d", len(cookies))
	}
	if cookies[0].Name != authCookieName {
		t.Fatalf("expected auth cookie name %q, got %q", authCookieName, cookies[0].Name)
	}
	if cookies[0].MaxAge != int(authTokenDuration.Seconds()) {
		t.Fatalf("expected auth cookie max age %d, got %d", int(authTokenDuration.Seconds()), cookies[0].MaxAge)
	}

	var response authResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Authenticated {
		t.Fatal("expected authenticated response")
	}
	if response.Username != "admin" {
		t.Fatalf("expected username admin, got %q", response.Username)
	}
}

func TestLoginRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name   string
		app    *Handler
		method string
		body   string
		status int
	}{
		{
			name:   "wrong method",
			app:    newTestHandler(t),
			method: http.MethodGet,
			body:   `{}`,
			status: http.StatusMethodNotAllowed,
		},
		{
			name: "auth not configured",
			app: &Handler{
				config: Config{},
			},
			method: http.MethodPost,
			body:   `{}`,
			status: http.StatusServiceUnavailable,
		},
		{
			name:   "bad json",
			app:    newTestHandler(t),
			method: http.MethodPost,
			body:   `{`,
			status: http.StatusBadRequest,
		},
		{
			name:   "wrong credentials",
			app:    newTestHandler(t),
			method: http.MethodPost,
			body:   `{"username":"admin","password":"wrong"}`,
			status: http.StatusUnauthorized,
		},
		{
			name:   "wrong username",
			app:    newTestHandler(t),
			method: http.MethodPost,
			body:   `{"username":"someone-else","password":"password"}`,
			status: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/auth/login", strings.NewReader(tt.body))
			rr := httptest.NewRecorder()

			tt.app.Login(rr, req)

			if rr.Code != tt.status {
				t.Fatalf("expected status %d, got %d", tt.status, rr.Code)
			}
		})
	}
}

func TestLoginRateLimitsInvalidCredentials(t *testing.T) {
	app := newTestHandler(t)
	body := `{"username":"admin","password":"wrong"}`

	for i := 0; i < loginFailureLimit; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
		req.RemoteAddr = "203.0.113.10:1234"
		rr := httptest.NewRecorder()

		app.Login(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected status %d, got %d", i+1, http.StatusUnauthorized, rr.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body))
	req.RemoteAddr = "203.0.113.10:1234"
	rr := httptest.NewRecorder()

	app.Login(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func TestLogout(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rr := httptest.NewRecorder()

	app.Logout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 auth cookie, got %d", len(cookies))
	}
	if cookies[0].Name != authCookieName {
		t.Fatalf("expected auth cookie name %q, got %q", authCookieName, cookies[0].Name)
	}
	if cookies[0].MaxAge != -1 {
		t.Fatalf("expected logout to clear cookie with max age -1, got %d", cookies[0].MaxAge)
	}
}

func TestLogoutRejectsWrongMethod(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	rr := httptest.NewRecorder()

	app.Logout(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
	if got := rr.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("expected Allow header POST, got %q", got)
	}
}

func TestGetCurrentUser(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	ctx := context.WithValue(req.Context(), authClaimsContextKey, &authClaims{
		Username: "admin",
	})
	rr := httptest.NewRecorder()

	app.GetCurrentUser(rr, req.WithContext(ctx))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var response authResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Authenticated {
		t.Fatal("expected authenticated response")
	}
	if response.Username != "admin" {
		t.Fatalf("expected username admin, got %q", response.Username)
	}
}

func TestGetCurrentUserRejectsMissingClaims(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rr := httptest.NewRecorder()

	app.GetCurrentUser(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestGetCurrentUserRejectsWrongMethod(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/auth/me", nil)
	rr := httptest.NewRecorder()

	app.GetCurrentUser(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
	if got := rr.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("expected Allow header GET, got %q", got)
	}
}
