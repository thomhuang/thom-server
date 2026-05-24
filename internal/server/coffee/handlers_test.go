package coffee

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coffeedata "thom-server/internal/coffee"
	"thom-server/internal/data"
	"thom-server/internal/server/response"

	_ "github.com/mattn/go-sqlite3"
)

func TestGetCoffeeEntries(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/coffee", nil)
	rr := httptest.NewRecorder()

	app.GetEntries(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var entries []coffeedata.EntrySummary
	if err := json.Unmarshal(rr.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 coffee entry, got %d", len(entries))
	}
	if entries[0].CoffeeName != "Ethiopia Test Lot" {
		t.Fatalf("expected Ethiopia Test Lot, got %q", entries[0].CoffeeName)
	}
	if entries[0].TastingNotes != "floral, citrus, honey" {
		t.Fatalf("expected tasting notes, got %q", entries[0].TastingNotes)
	}
	if entries[0].Origin != "Yirgacheffe, Ethiopia" {
		t.Fatalf("expected origin, got %q", entries[0].Origin)
	}
	if entries[0].CoffeeVarietal != "Heirloom" {
		t.Fatalf("expected coffee varietal, got %q", entries[0].CoffeeVarietal)
	}
	if entries[0].ProcessingMethod != "Washed" {
		t.Fatalf("expected processing method, got %q", entries[0].ProcessingMethod)
	}
}

func TestGetCoffeeEntryByID(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/coffee/1", nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()

	app.GetEntryByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var entry coffeedata.Entry
	if err := json.Unmarshal(rr.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Roaster != "Sey Coffee" {
		t.Fatalf("expected Sey Coffee, got %q", entry.Roaster)
	}
	if entry.TastingNotes != entry.Notes {
		t.Fatal("expected tasting notes to mirror notes")
	}
	if entry.Origin != "Yirgacheffe, Ethiopia" {
		t.Fatalf("expected origin, got %q", entry.Origin)
	}
	if entry.CoffeeVarietal != "Heirloom" {
		t.Fatalf("expected coffee varietal, got %q", entry.CoffeeVarietal)
	}
	if entry.ProcessingMethod != "Washed" {
		t.Fatalf("expected processing method, got %q", entry.ProcessingMethod)
	}
}

func TestGetCoffeeEntryByIDRejectsInvalidID(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/coffee/abc", nil)
	req.SetPathValue("id", "abc")
	rr := httptest.NewRecorder()

	app.GetEntryByID(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestGetCoffeeEntryByIDHandlesMissingEntry(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/coffee/999", nil)
	req.SetPathValue("id", "999")
	rr := httptest.NewRecorder()

	app.GetEntryByID(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

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
	if len(roasters) != 4 {
		t.Fatalf("expected 4 roasters, got %d", len(roasters))
	}
	if roasters[0].ID != "sey-coffee" {
		t.Fatalf("expected first roaster sey-coffee, got %q", roasters[0].ID)
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

func TestCreateCoffeeEntry(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/coffee", strings.NewReader(`{
		"date": "2026-05-21",
		"coffeeName": "Colombia Test Lot",
		"origin": " Huila, Colombia ",
		"coffeeVarietal": " Caturra ",
		"processingMethod": " Honey ",
		"roaster": "Heart Coffee",
		"brewMethod": "kalita-wave",
		"ratio": "1:15",
		"grinder": "comandante-c40",
		"grindSetting": "24",
		"notes": "red fruit and caramel",
		"rating": 4
	}`))
	rr := httptest.NewRecorder()

	app.CreateEntry(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}

	var entry coffeedata.Entry
	if err := json.Unmarshal(rr.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.ID == "" {
		t.Fatal("expected created entry ID")
	}
	if entry.RoasterID != "heart-coffee" {
		t.Fatalf("expected generated roaster ID heart-coffee, got %q", entry.RoasterID)
	}
	if entry.TastingNotes != "red fruit and caramel" {
		t.Fatalf("expected tasting notes from notes, got %q", entry.TastingNotes)
	}
	if entry.Origin != "Huila, Colombia" {
		t.Fatalf("expected trimmed origin, got %q", entry.Origin)
	}
	if entry.CoffeeVarietal != "Caturra" {
		t.Fatalf("expected trimmed coffee varietal, got %q", entry.CoffeeVarietal)
	}
	if entry.ProcessingMethod != "Honey" {
		t.Fatalf("expected trimmed processing method, got %q", entry.ProcessingMethod)
	}
}

func TestCreateCoffeeEntryAcceptsTastingNotesAlias(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/coffee", strings.NewReader(`{
		"date": "2026-05-21",
		"coffeeName": "Colombia Test Lot",
		"origin": "Huila, Colombia",
		"coffeeVarietal": "Caturra",
		"processingMethod": "Honey",
		"roasterId": "heart-coffee",
		"roaster": "Heart Coffee",
		"brewMethod": "kalita-wave",
		"ratio": "1:15",
		"grinder": "comandante-c40",
		"grindSetting": "24",
		"tastingNotes": "red fruit and caramel",
		"rating": 4
	}`))
	rr := httptest.NewRecorder()

	app.CreateEntry(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}

	var entry coffeedata.Entry
	if err := json.Unmarshal(rr.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Notes != "red fruit and caramel" {
		t.Fatalf("expected notes from tastingNotes alias, got %q", entry.Notes)
	}
}

func TestCreateCoffeeEntryRejectsInvalidPayload(t *testing.T) {
	app := newTestHandler(t)

	tests := []struct {
		name string
		body string
	}{
		{
			name: "invalid JSON",
			body: `{`,
		},
		{
			name: "missing required field",
			body: `{
				"date": "2026-05-21",
				"coffeeName": "Colombia Test Lot",
				"roaster": "Heart Coffee",
				"brewMethod": "kalita-wave",
				"ratio": "1:15",
				"grinder": "comandante-c40",
				"notes": "red fruit and caramel",
				"rating": 4
			}`,
		},
		{
			name: "invalid date",
			body: `{
				"date": "May 21",
				"coffeeName": "Colombia Test Lot",
				"roaster": "Heart Coffee",
				"brewMethod": "kalita-wave",
				"ratio": "1:15",
				"grinder": "comandante-c40",
				"grindSetting": "24",
				"notes": "red fruit and caramel",
				"rating": 4
			}`,
		},
		{
			name: "invalid rating",
			body: `{
				"date": "2026-05-21",
				"coffeeName": "Colombia Test Lot",
				"roaster": "Heart Coffee",
				"brewMethod": "kalita-wave",
				"ratio": "1:15",
				"grinder": "comandante-c40",
				"grindSetting": "24",
				"notes": "red fruit and caramel",
				"rating": 6
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/coffee", strings.NewReader(tt.body))
			rr := httptest.NewRecorder()

			app.CreateEntry(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
			}
		})
	}
}

func TestUpdateCoffeeEntry(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPatch, "/coffee/1", strings.NewReader(`{
		"coffeeName": "Kenya Test Lot",
		"origin": "Nyeri, Kenya",
		"coffeeVarietal": "SL28",
		"processingMethod": "Washed",
		"rating": 3
	}`))
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()

	app.UpdateEntry(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var entry coffeedata.Entry
	if err := json.Unmarshal(rr.Body.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.ID != "1" {
		t.Fatalf("expected entry ID 1, got %q", entry.ID)
	}
	if entry.CoffeeName != "Kenya Test Lot" {
		t.Fatalf("expected updated coffee name, got %q", entry.CoffeeName)
	}
	if entry.Rating != 3 {
		t.Fatalf("expected updated rating 3, got %d", entry.Rating)
	}
	if entry.Roaster != "Sey Coffee" {
		t.Fatalf("expected unchanged roaster Sey Coffee, got %q", entry.Roaster)
	}
	if entry.Origin != "Nyeri, Kenya" {
		t.Fatalf("expected updated origin, got %q", entry.Origin)
	}
	if entry.CoffeeVarietal != "SL28" {
		t.Fatalf("expected updated coffee varietal, got %q", entry.CoffeeVarietal)
	}
	if entry.ProcessingMethod != "Washed" {
		t.Fatalf("expected updated processing method, got %q", entry.ProcessingMethod)
	}
}

func TestUpdateCoffeeEntryHandlesMissingEntry(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPatch, "/coffee/999", strings.NewReader(`{
		"rating": 4
	}`))
	req.SetPathValue("id", "999")
	rr := httptest.NewRecorder()

	app.UpdateEntry(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestUpdateCoffeeEntryRejectsInvalidPatch(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPatch, "/coffee/1", strings.NewReader(`{
		"rating": 6
	}`))
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()

	app.UpdateEntry(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestUpdateCoffeeEntryRejectsPartialRoasterPatch(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPatch, "/coffee/1", strings.NewReader(`{
		"roasterId": "heart-coffee"
	}`))
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()

	app.UpdateEntry(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestDeleteCoffeeEntry(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodDelete, "/coffee/1", nil)
	req.SetPathValue("id", "1")
	rr := httptest.NewRecorder()

	app.DeleteEntry(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}

	_, err := app.coffee.GetByID(1)
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected deleted coffee entry to be missing, got %v", err)
	}
}

func TestDeleteCoffeeEntryHandlesMissingEntry(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodDelete, "/coffee/999", nil)
	req.SetPathValue("id", "999")
	rr := httptest.NewRecorder()

	app.DeleteEntry(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func newTestHandler(t *testing.T) *Handler {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})

	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}

	model := &coffeedata.Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	fixture := `
		INSERT INTO CoffeeEntries (
			id, BrewDate, CoffeeName, Origin, CoffeeVarietal, ProcessingMethod,
			DaysSinceRoast, RoasterID, Roaster, BrewMethod, Ratio, Grinder,
			GrindSetting, Dose, YieldAmount, WaterTemperature, BrewTime,
			BloomTime, BloomWater, PourNotes, RoastLevel, Notes, Rating, CreatedAt
		)
		VALUES (
			1, '2026-05-20', 'Ethiopia Test Lot', 'Yirgacheffe, Ethiopia',
			'Heirloom', 'Washed', '10', 'sey-coffee', 'Sey Coffee',
			'v60', '1:16', 'fellow-ode', '4.2', '20g', '320g', '203F',
			'3:20', '45s', '50g', 'Two-pour finish', 'light',
			'floral, citrus, honey', 5, '2026-05-20 12:00:00'
		);`
	if _, err = db.Exec(fixture); err != nil {
		t.Fatal(err)
	}

	return New(
		model,
		response.Responder{ErrorLog: log.New(io.Discard, "", 0)},
	)
}
