package blog

import "testing"

func TestDraftDefaultsToEmpty(t *testing.T) {
	model := newTestModel(t)

	draft, err := model.GetDraft()
	if err != nil {
		t.Fatal(err)
	}
	if draft.Title != "" || draft.Body != "" || draft.Category != "" || draft.Published {
		t.Fatalf("expected an empty draft, got %+v", draft)
	}
}

func TestDraftSaveAndRead(t *testing.T) {
	model := newTestModel(t)

	err := model.SaveDraft(&Draft{Title: "Working", Body: "Notes", Category: "Coffee", Published: true})
	if err != nil {
		t.Fatal(err)
	}

	draft, err := model.GetDraft()
	if err != nil {
		t.Fatal(err)
	}
	if draft.Title != "Working" || draft.Body != "Notes" || draft.Category != "Coffee" || !draft.Published {
		t.Fatalf("unexpected draft: %+v", draft)
	}
}

func TestDraftSaveOverwrites(t *testing.T) {
	model := newTestModel(t)

	if err := model.SaveDraft(&Draft{Title: "First"}); err != nil {
		t.Fatal(err)
	}
	if err := model.SaveDraft(&Draft{Title: "Second", Body: "Body"}); err != nil {
		t.Fatal(err)
	}

	draft, err := model.GetDraft()
	if err != nil {
		t.Fatal(err)
	}
	if draft.Title != "Second" || draft.Body != "Body" || draft.Category != "" || draft.Published {
		t.Fatalf("expected the second draft to replace the first, got %+v", draft)
	}
}

func TestDraftDelete(t *testing.T) {
	model := newTestModel(t)

	if err := model.SaveDraft(&Draft{Title: "Temp"}); err != nil {
		t.Fatal(err)
	}
	if err := model.DeleteDraft(); err != nil {
		t.Fatal(err)
	}

	draft, err := model.GetDraft()
	if err != nil {
		t.Fatal(err)
	}
	if draft.Title != "" {
		t.Fatalf("expected the draft to be cleared, got %+v", draft)
	}
}
