package coffee

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coffeedata "thom-server/internal/coffee"
)

func TestGetCoffeeRoasters(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/coffee/roasters", nil)
	rr := httptest.NewRecorder()

	app.GetRoasters(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var roasters []coffeedata.Roaster
	if err := json.Unmarshal(rr.Body.Bytes(), &roasters); err != nil {
		t.Fatal(err)
	}
	if len(roasters) != 1 {
		t.Fatalf("expected 1 roaster, got %d", len(roasters))
	}
	if roasters[0].ID != "shoebox" {
		t.Fatalf("expected first roaster shoebox, got %q", roasters[0].ID)
	}
}

func TestCreateCoffeeRoaster(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/coffee/roasters", strings.NewReader(`{
		"roaster": "DAK Coffee Roasters"
	}`))
	rr := httptest.NewRecorder()

	app.CreateRoaster(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}

	var roaster coffeedata.Roaster
	if err := json.Unmarshal(rr.Body.Bytes(), &roaster); err != nil {
		t.Fatal(err)
	}
	if roaster.ID != "dak-coffee-roasters" {
		t.Fatalf("expected generated roaster ID dak-coffee-roasters, got %q", roaster.ID)
	}
}

func TestCreateCoffeeRoasterRejectsInvalidPayload(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/coffee/roasters", strings.NewReader(`{"roaster": ""}`))
	rr := httptest.NewRecorder()

	app.CreateRoaster(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}
