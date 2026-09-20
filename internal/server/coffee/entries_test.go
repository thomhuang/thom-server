package coffee

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coffeedata "thom-server/internal/coffee"
	"thom-server/internal/data"
)

func TestGetCoffeeEntries(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/coffee", nil)
	rr := httptest.NewRecorder()

	app.GetEntries(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var entries []coffeedata.Entry
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
	if entry.Roaster != "Shoebox" {
		t.Fatalf("expected Shoebox, got %q", entry.Roaster)
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

func TestCreateCoffeeEntry(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/coffee", strings.NewReader(`{
		"date": "2026-05-21",
		"coffeeName": "Colombia Test Lot",
		"origin": " Huila, Colombia ",
		"coffeeVarietal": " Caturra ",
		"processingMethod": " Honey ",
		"roaster": "Shoebox",
		"brewMethod": "kalita-wave",
		"ratio": "1:15",
		"grinder": "comandante-c40",
		"grindSetting": 24,
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
	if entry.RoasterID != "shoebox" {
		t.Fatalf("expected generated roaster ID shoebox, got %q", entry.RoasterID)
	}
	if entry.GrinderID != "comandante-c40" {
		t.Fatalf("expected generated grinder ID comandante-c40, got %q", entry.GrinderID)
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
		"roasterId": "shoebox",
		"roaster": "Shoebox",
		"brewMethod": "kalita-wave",
		"ratio": "1:15",
		"grinder": "comandante-c40",
		"grindSetting": 24,
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
				"roaster": "Shoebox",
				"ratio": "1:15",
				"grinder": "comandante-c40",
				"grindSetting": 24,
				"notes": "red fruit and caramel",
				"rating": 4
			}`,
		},
		{
			name: "invalid date",
			body: `{
				"date": "May 21",
				"coffeeName": "Colombia Test Lot",
				"roaster": "Shoebox",
				"brewMethod": "kalita-wave",
				"ratio": "1:15",
				"grinder": "comandante-c40",
				"grindSetting": 24,
				"notes": "red fruit and caramel",
				"rating": 4
			}`,
		},
		{
			name: "invalid rating",
			body: `{
				"date": "2026-05-21",
				"coffeeName": "Colombia Test Lot",
				"roaster": "Shoebox",
				"brewMethod": "kalita-wave",
				"ratio": "1:15",
				"grinder": "comandante-c40",
				"grindSetting": 24,
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
	if entry.Roaster != "Shoebox" {
		t.Fatalf("expected unchanged roaster Shoebox, got %q", entry.Roaster)
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
		"roasterId": "shoebox"
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
