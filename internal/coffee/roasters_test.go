package coffee

import "testing"

func TestModelGetRoasters(t *testing.T) {
	model := newTestModel(t)

	roasters, err := model.GetRoasters()
	if err != nil {
		t.Fatal(err)
	}

	if len(roasters) != 1 {
		t.Fatalf("expected 1 seeded roaster, got %d", len(roasters))
	}
	if roasters[0].ID != "shoebox" {
		t.Fatalf("expected first roaster shoebox, got %q", roasters[0].ID)
	}
}

func TestModelUpsertRoaster(t *testing.T) {
	model := newTestModel(t)

	roaster, err := model.UpsertRoaster(&Roaster{ID: "dak-coffee-roasters", Roaster: "DAK Coffee Roasters"})
	if err != nil {
		t.Fatal(err)
	}

	if roaster.ID != "dak-coffee-roasters" {
		t.Fatalf("expected roaster ID dak-coffee-roasters, got %q", roaster.ID)
	}
	if roaster.CreatedAt == "" {
		t.Fatal("expected created timestamp")
	}
}

func TestModelUpsertRoasterReturnsExistingNameConflict(t *testing.T) {
	model := newTestModel(t)

	roaster, err := model.UpsertRoaster(&Roaster{ID: "sh", Roaster: "Shoebox"})
	if err != nil {
		t.Fatal(err)
	}

	if roaster.ID != "shoebox" {
		t.Fatalf("expected existing roaster ID shoebox, got %q", roaster.ID)
	}
}

func TestModelUpsertRoasterReturnsExistingIDConflict(t *testing.T) {
	model := newTestModel(t)

	roaster, err := model.UpsertRoaster(&Roaster{ID: "shoebox", Roaster: "Renamed Roaster"})
	if err != nil {
		t.Fatal(err)
	}

	if roaster.Roaster != "Shoebox" {
		t.Fatalf("expected existing roaster name Shoebox, got %q", roaster.Roaster)
	}

	roaster, err = model.GetRoasterByID("shoebox")
	if err != nil {
		t.Fatal(err)
	}
	if roaster.Roaster != "Shoebox" {
		t.Fatalf("expected stored roaster name to remain Shoebox, got %q", roaster.Roaster)
	}
}

func TestModelUpsertRoasterCachesByName(t *testing.T) {
	model := newTestModel(t)

	first, err := model.UpsertRoaster(&Roaster{Roaster: "Shoebox"})
	if err != nil {
		t.Fatal(err)
	}

	// A different case must hit the same cache entry rather than re-querying D1.
	second, err := model.UpsertRoaster(&Roaster{Roaster: "SHOEBOX"})
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Fatal("expected the second upsert to be served from the lookup cache")
	}
}
