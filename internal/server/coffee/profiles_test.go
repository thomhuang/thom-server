package coffee

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coffeedata "thom-server/internal/coffee"
)

func seedProfile(t *testing.T, app *Handler, profile *coffeedata.Profile) {
	t.Helper()
	if _, err := app.coffee.InsertProfile(profile); err != nil {
		t.Fatal(err)
	}
}

func TestGetCoffeeProfiles(t *testing.T) {
	app := newTestHandler(t)
	seedProfile(t, app, &coffeedata.Profile{
		ID:         "v60-four-six",
		Name:       "V60 4:6",
		BrewMethod: "v60",
		Notes:      "Bloom 45s, then four equal pours",
	})

	req := httptest.NewRequest(http.MethodGet, "/coffee/profiles", nil)
	rr := httptest.NewRecorder()

	app.GetProfiles(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var profiles []coffeedata.Profile
	if err := json.Unmarshal(rr.Body.Bytes(), &profiles); err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].ID != "v60-four-six" {
		t.Fatalf("expected id v60-four-six, got %q", profiles[0].ID)
	}
}

func TestCreateCoffeeProfile(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/coffee/profiles", strings.NewReader(`{
		"name": "V60 4:6",
		"brewMethod": "v60",
		"notes": "Bloom 45s, then four equal pours"
	}`))
	rr := httptest.NewRecorder()

	app.CreateProfile(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}

	var profile coffeedata.Profile
	if err := json.Unmarshal(rr.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.ID != "v60-4-6" {
		t.Fatalf("expected id v60-4-6, got %q", profile.ID)
	}
	if profile.BrewMethod != "v60" {
		t.Fatalf("expected brew method v60, got %q", profile.BrewMethod)
	}
}

func TestCreateCoffeeProfileRejectsDuplicateName(t *testing.T) {
	app := newTestHandler(t)
	seedProfile(t, app, &coffeedata.Profile{
		ID:         "v60-four-six",
		Name:       "V60 4:6",
		BrewMethod: "v60",
		Notes:      "Bloom 45s, then four equal pours",
	})

	req := httptest.NewRequest(http.MethodPost, "/coffee/profiles", strings.NewReader(`{
		"name": "V60 4:6",
		"brewMethod": "v60",
		"notes": "different"
	}`))
	rr := httptest.NewRecorder()

	app.CreateProfile(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rr.Code)
	}
}

func TestCreateCoffeeProfileRejectsInvalidPayload(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/coffee/profiles", strings.NewReader(`{
		"name": "",
		"brewMethod": "v60",
		"notes": "x"
	}`))
	rr := httptest.NewRecorder()

	app.CreateProfile(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestUpdateCoffeeProfile(t *testing.T) {
	app := newTestHandler(t)
	seedProfile(t, app, &coffeedata.Profile{
		ID:         "v60-four-six",
		Name:       "V60 4:6",
		BrewMethod: "v60",
		Notes:      "Bloom 45s, then four equal pours",
	})

	req := httptest.NewRequest(http.MethodPatch, "/coffee/profiles/v60-four-six", strings.NewReader(`{
		"name": "V60 4:6 revised",
		"brewMethod": "v60",
		"notes": "Revised"
	}`))
	req.SetPathValue("id", "v60-four-six")
	rr := httptest.NewRecorder()

	app.UpdateProfile(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var profile coffeedata.Profile
	if err := json.Unmarshal(rr.Body.Bytes(), &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Name != "V60 4:6 revised" {
		t.Fatalf("expected updated name, got %q", profile.Name)
	}
}

func TestUpdateCoffeeProfileHandlesMissingProfile(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPatch, "/coffee/profiles/missing", strings.NewReader(`{
		"name": "x",
		"brewMethod": "v60",
		"notes": "y"
	}`))
	req.SetPathValue("id", "missing")
	rr := httptest.NewRecorder()

	app.UpdateProfile(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestDeleteCoffeeProfile(t *testing.T) {
	app := newTestHandler(t)
	seedProfile(t, app, &coffeedata.Profile{
		ID:         "v60-four-six",
		Name:       "V60 4:6",
		BrewMethod: "v60",
		Notes:      "Bloom 45s, then four equal pours",
	})

	req := httptest.NewRequest(http.MethodDelete, "/coffee/profiles/v60-four-six", nil)
	req.SetPathValue("id", "v60-four-six")
	rr := httptest.NewRecorder()

	app.DeleteProfile(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}

func TestCreateCoffeeEntryRejectsProfileMethodMismatch(t *testing.T) {
	app := newTestHandler(t)
	seedProfile(t, app, &coffeedata.Profile{
		ID:         "v60-four-six",
		Name:       "V60 4:6",
		BrewMethod: "v60",
		Notes:      "Bloom 45s, then four equal pours",
	})

	req := httptest.NewRequest(http.MethodPost, "/coffee", strings.NewReader(`{
		"date": "2026-05-21",
		"coffeeName": "Colombia Test Lot",
		"roaster": "Shoebox",
		"brewMethod": "kalita-wave",
		"ratio": "1:15",
		"grinder": "comandante-c40",
		"grindSetting": 24,
		"notes": "red fruit and caramel",
		"rating": 4,
		"profileId": "v60-four-six"
	}`))
	rr := httptest.NewRecorder()

	app.CreateEntry(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}
