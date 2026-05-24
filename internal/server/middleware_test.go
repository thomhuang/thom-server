package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommonMiddlewareAllowsConfiguredOrigins(t *testing.T) {
	app := &App{
		config: Config{
			ClientOrigins: []string{"https://app.example.com"},
		},
	}
	nextCalled := false
	handler := app.commonMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("expected allowed origin header, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials header true, got %q", got)
	}
	if got := rr.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("expected Vary Origin, got %q", got)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("expected JSON content type, got %q", got)
	}
}

func TestCommonMiddlewareRejectsUnconfiguredOrigins(t *testing.T) {
	app := &App{
		config: Config{
			ClientOrigins: []string{"https://app.example.com"},
		},
	}
	handler := app.commonMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS origin header, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("expected no credentials header, got %q", got)
	}
}

func TestCommonMiddlewareHandlesPreflight(t *testing.T) {
	app := newTestApp(t)
	nextCalled := false
	handler := app.commonMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}))

	req := httptest.NewRequest(http.MethodOptions, "/categories", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if nextCalled {
		t.Fatal("expected preflight request to stop before next handler")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PATCH, DELETE, OPTIONS" {
		t.Fatalf("expected allowed methods header, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type" {
		t.Fatalf("expected allowed headers header, got %q", got)
	}
}

func TestIsAllowedOrigin(t *testing.T) {
	app := &App{
		config: Config{
			ClientOrigins: []string{"https://app.example.com"},
		},
	}

	if !app.isAllowedOrigin("https://app.example.com") {
		t.Fatal("expected configured origin to be allowed")
	}
	if app.isAllowedOrigin("https://other.example.com") {
		t.Fatal("expected unconfigured origin to be rejected")
	}
	if app.isAllowedOrigin("") {
		t.Fatal("expected empty origin to be rejected")
	}
}
