package coffee

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coffeedata "thom-server/internal/coffee"
)

func TestGetCoffeeGrinders(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/coffee/grinders", nil)
	rr := httptest.NewRecorder()

	app.GetGrinders(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var grinders []coffeedata.Grinder
	if err := json.Unmarshal(rr.Body.Bytes(), &grinders); err != nil {
		t.Fatal(err)
	}
	if len(grinders) != 1 {
		t.Fatalf("expected 1 grinder, got %d", len(grinders))
	}
	if grinders[0].ID != "fellow-ode" {
		t.Fatalf("expected first grinder fellow-ode, got %q", grinders[0].ID)
	}
}

func TestCreateCoffeeGrinder(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/coffee/grinders", strings.NewReader(`{
		"grinder": "1zpresso K-Ultra"
	}`))
	rr := httptest.NewRecorder()

	app.CreateGrinder(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}

	var grinder coffeedata.Grinder
	if err := json.Unmarshal(rr.Body.Bytes(), &grinder); err != nil {
		t.Fatal(err)
	}
	if grinder.ID != "1zpresso-k-ultra" {
		t.Fatalf("expected generated grinder ID 1zpresso-k-ultra, got %q", grinder.ID)
	}
}

func TestCreateCoffeeGrinderRejectsInvalidPayload(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/coffee/grinders", strings.NewReader(`{"grinder": ""}`))
	rr := httptest.NewRecorder()

	app.CreateGrinder(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}
