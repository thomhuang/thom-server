package coffee

import (
	"errors"
	"strconv"
	"testing"

	"thom-server/internal/data"
)

func TestModelProfileCRUD(t *testing.T) {
	model := newTestModel(t)

	created, err := model.InsertProfile(&Profile{
		ID:         "v60-four-six",
		Name:       "V60 4:6",
		BrewMethod: "v60",
		Notes:      "Bloom 45s, then four equal pours",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "v60-four-six" {
		t.Fatalf("expected id v60-four-six, got %q", created.ID)
	}

	profiles, err := model.GetProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	got, err := model.GetProfileByID("v60-four-six")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "V60 4:6" || got.BrewMethod != "v60" {
		t.Fatalf("unexpected profile: %+v", got)
	}

	updated, err := model.UpdateProfile("v60-four-six", &Profile{
		Name:       "V60 4:6 revised",
		BrewMethod: "v60",
		Notes:      "Revised notes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "V60 4:6 revised" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}

	if err = model.DeleteProfile("v60-four-six"); err != nil {
		t.Fatal(err)
	}
	if _, err = model.GetProfileByID("v60-four-six"); !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord after delete, got %v", err)
	}
}

func TestModelUpdateProfileNoRecord(t *testing.T) {
	model := newTestModel(t)

	_, err := model.UpdateProfile("missing", &Profile{
		Name:       "x",
		BrewMethod: "v60",
		Notes:      "y",
	})
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func TestModelEntryPersistsProfileAndSessionNotes(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.InsertProfile(&Profile{
		ID:         "v60-four-six",
		Name:       "V60 4:6",
		BrewMethod: "v60",
		Notes:      "Bloom 45s, then four equal pours",
	}); err != nil {
		t.Fatal(err)
	}

	entry, err := model.Insert(&Entry{
		Date:         "2026-05-21",
		CoffeeName:   "Colombia Test Lot",
		RoasterID:    "shoebox",
		Roaster:      "Shoebox",
		BrewMethod:   "v60",
		Ratio:        "1:15",
		Grinder:      "comandante-c40",
		GrindSetting: 24,
		Notes:        "red fruit and caramel",
		Rating:       4,
		ProfileID:    "v60-four-six",
		SessionNotes: "stalled at 2:00",
	})
	if err != nil {
		t.Fatal(err)
	}

	if entry.ProfileID != "v60-four-six" {
		t.Fatalf("expected profile id, got %q", entry.ProfileID)
	}
	if entry.SessionNotes != "stalled at 2:00" {
		t.Fatalf("expected session notes, got %q", entry.SessionNotes)
	}
}

func TestModelDeleteProfileClearsEntryReference(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.InsertProfile(&Profile{
		ID:         "v60-four-six",
		Name:       "V60 4:6",
		BrewMethod: "v60",
		Notes:      "Bloom 45s, then four equal pours",
	}); err != nil {
		t.Fatal(err)
	}

	entry, err := model.Insert(&Entry{
		Date:         "2026-05-21",
		CoffeeName:   "Colombia Test Lot",
		RoasterID:    "shoebox",
		Roaster:      "Shoebox",
		BrewMethod:   "v60",
		Ratio:        "1:15",
		Grinder:      "comandante-c40",
		GrindSetting: 24,
		Notes:        "red fruit and caramel",
		Rating:       4,
		ProfileID:    "v60-four-six",
	})
	if err != nil {
		t.Fatal(err)
	}

	id, err := strconv.Atoi(entry.ID)
	if err != nil {
		t.Fatal(err)
	}

	if err = model.DeleteProfile("v60-four-six"); err != nil {
		t.Fatal(err)
	}

	entry, err = model.GetByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if entry.ProfileID != "" {
		t.Fatalf("expected cleared profile id after delete, got %q", entry.ProfileID)
	}
}
