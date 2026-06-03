package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutes(t *testing.T) {
	app := newTestApp(t)
	handler := app.commonMiddleware(app.routes())

	req := httptest.NewRequest(http.MethodGet, "/coffee", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected coffee route status %d, got %d", http.StatusOK, rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/coffee/roasters", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected coffee roasters route status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestCoffeeMutatingRoutesRequireAuth(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/auth/logout"},
		{method: http.MethodPost, path: "/coffee", body: `{}`},
		{method: http.MethodPost, path: "/coffee/roasters", body: `{}`},
		{method: http.MethodPatch, path: "/coffee/1", body: `{}`},
		{method: http.MethodDelete, path: "/coffee/1"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
			}
		})
	}
}

func TestCoffeeMutatingRoutesForAdmin(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()
	cookie := loginCookie(t, handler)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{
			name:   "create roaster",
			method: http.MethodPost,
			path:   "/coffee/roasters",
			body:   `{"roaster":"DAK Coffee Roasters"}`,
			status: http.StatusCreated,
		},
		{
			name:   "create entry",
			method: http.MethodPost,
			path:   "/coffee",
			body: `{
				"date": "2026-05-21",
				"coffeeName": "Colombia Test Lot",
				"roasterId": "shoebox",
				"roaster": "Shoebox",
				"brewMethod": "kalita-wave",
				"ratio": "1:15",
				"grinder": "comandante-c40",
				"grindSetting": 24,
				"notes": "red fruit and caramel",
				"rating": 4
			}`,
			status: http.StatusCreated,
		},
		{
			name:   "update entry",
			method: http.MethodPatch,
			path:   "/coffee/1",
			body:   `{"coffeeName":"Kenya Test Lot","rating":3}`,
			status: http.StatusOK,
		},
		{
			name:   "delete entry",
			method: http.MethodDelete,
			path:   "/coffee/1",
			status: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Origin", "http://localhost:3000")
			req.AddCookie(cookie)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.status {
				t.Fatalf("expected status %d, got %d", tt.status, rr.Code)
			}
		})
	}
}

func TestLogoutRouteRequiresAllowedOrigin(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()
	cookie := loginCookie(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected missing origin status %d, got %d", http.StatusForbidden, rr.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.AddCookie(cookie)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected allowed origin status %d, got %d", http.StatusOK, rr.Code)
	}
	cookies := rr.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != -1 {
		t.Fatal("expected logout to clear the auth cookie")
	}
}

func loginCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{
		"username": "admin",
		"password": "password"
	}`))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected login status %d, got %d", http.StatusOK, rr.Code)
	}

	cookies := rr.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 auth cookie, got %d", len(cookies))
	}

	return cookies[0]
}
