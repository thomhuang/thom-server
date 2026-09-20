package coffee

import "testing"

func TestModelGetGrinders(t *testing.T) {
	model := newTestModel(t)

	grinders, err := model.GetGrinders()
	if err != nil {
		t.Fatal(err)
	}

	if len(grinders) != 1 {
		t.Fatalf("expected 1 seeded grinder, got %d", len(grinders))
	}
	if grinders[0].ID != "fellow-ode" {
		t.Fatalf("expected first grinder fellow-ode, got %q", grinders[0].ID)
	}
}

func TestModelUpsertGrinder(t *testing.T) {
	model := newTestModel(t)

	grinder, err := model.UpsertGrinder(&Grinder{ID: "df64", Grinder: "DF64V mk. II with SSP MP Burrs"})
	if err != nil {
		t.Fatal(err)
	}

	if grinder.ID != "df64" {
		t.Fatalf("expected grinder ID df64, got %q", grinder.ID)
	}
	if grinder.CreatedAt == "" {
		t.Fatal("expected created timestamp")
	}
}

func TestModelUpsertGrinderDerivesIDFromName(t *testing.T) {
	model := newTestModel(t)

	grinder, err := model.UpsertGrinder(&Grinder{Grinder: "1zpresso K-Ultra"})
	if err != nil {
		t.Fatal(err)
	}

	if grinder.ID != "1zpresso-k-ultra" {
		t.Fatalf("expected derived grinder ID 1zpresso-k-ultra, got %q", grinder.ID)
	}
}

func TestModelUpsertGrinderReturnsExistingNameConflict(t *testing.T) {
	model := newTestModel(t)

	grinder, err := model.UpsertGrinder(&Grinder{ID: "other", Grinder: "Fellow Ode"})
	if err != nil {
		t.Fatal(err)
	}

	if grinder.ID != "fellow-ode" {
		t.Fatalf("expected existing grinder ID fellow-ode, got %q", grinder.ID)
	}
}
