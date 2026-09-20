package shop

import "testing"

func TestModelGetBrands(t *testing.T) {
	model := newTestModel(t)

	brands, err := model.GetBrands()
	if err != nil {
		t.Fatal(err)
	}

	if len(brands) != 1 {
		t.Fatalf("expected 1 seeded brand, got %d", len(brands))
	}
	if brands[0].ID != "acme" {
		t.Fatalf("expected brand acme, got %q", brands[0].ID)
	}
}
