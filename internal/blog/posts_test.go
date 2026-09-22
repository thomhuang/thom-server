package blog

import (
	"database/sql"
	"strconv"
	"testing"

	"thom-server/internal/data"

	_ "modernc.org/sqlite"
)

func newTestModel(t *testing.T) *Model {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
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

	model := &Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	return model
}

func TestPostLifecycle(t *testing.T) {
	model := newTestModel(t)

	created, err := model.Insert(&Post{Title: "First post", Body: "Hello, world.", Category: "Coffee"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("expected the inserted post to have an id")
	}
	if created.Published {
		t.Fatal("expected a new post to default to unpublished")
	}
	if created.CategoryID != "coffee" || created.Category != "Coffee" {
		t.Fatalf("unexpected resolved category: %+v", created)
	}
	if created.CreatedAt == "" || created.UpdatedAt == "" {
		t.Fatal("expected timestamps on the inserted post")
	}

	fetched, err := model.GetByID(mustID(t, created.ID))
	if err != nil {
		t.Fatal(err)
	}
	if fetched.Title != "First post" || fetched.Body != "Hello, world." {
		t.Fatalf("unexpected stored post: %+v", fetched)
	}

	updated, err := model.Update(mustID(t, created.ID), &Post{Title: "First post", Body: "Edited.", Category: "Gear", Published: true})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Published || updated.Body != "Edited." || updated.Category != "Gear" {
		t.Fatalf("unexpected updated post: %+v", updated)
	}

	if err = model.Delete(mustID(t, created.ID)); err != nil {
		t.Fatal(err)
	}

	if _, err = model.GetByID(mustID(t, created.ID)); err != data.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord after delete, got %v", err)
	}

	if err = model.Delete(mustID(t, created.ID)); err != data.ErrNoRecord {
		t.Fatalf("expected ErrNoRecord on second delete, got %v", err)
	}
}

func TestGetAllPublishedFilter(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.Insert(&Post{Title: "Draft", Category: "Coffee"}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Insert(&Post{Title: "Live", Category: "Coffee", Published: true}); err != nil {
		t.Fatal(err)
	}

	published, err := model.GetAll(true, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 1 || published[0].Title != "Live" {
		t.Fatalf("expected only the published post, got %+v", published)
	}

	all, err := model.GetAll(false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected both posts, got %d", len(all))
	}

	// Newest first, so the draft (inserted first) comes after the live post.
	if all[0].Title != "Live" || all[1].Title != "Draft" {
		t.Fatalf("expected newest-first order, got %v and %v", all[0].Title, all[1].Title)
	}
}

func TestGetAllCategoryFilter(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.Insert(&Post{Title: "Pour-over setup", Category: "Gear", Published: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Insert(&Post{Title: "Ethiopia notes", Category: "Coffee", Published: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Insert(&Post{Title: "Hidden draft", Category: "Gear"}); err != nil {
		t.Fatal(err)
	}

	gear, err := model.GetAll(true, "gear")
	if err != nil {
		t.Fatal(err)
	}
	if len(gear) != 1 || gear[0].Title != "Pour-over setup" {
		t.Fatalf("expected only the published gear post, got %+v", gear)
	}

	allGear, err := model.GetAll(false, "gear")
	if err != nil {
		t.Fatal(err)
	}
	if len(allGear) != 2 {
		t.Fatalf("expected both gear posts for the admin, got %d", len(allGear))
	}

	none, err := model.GetAll(true, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no posts for a missing category, got %d", len(none))
	}
}

func TestCategories(t *testing.T) {
	model := newTestModel(t)

	if _, err := model.Insert(&Post{Title: "One", Category: "Coffee"}); err != nil {
		t.Fatal(err)
	}
	// A second post in the same category must not create a duplicate, and a
	// different case must resolve to the stored row.
	if _, err := model.Insert(&Post{Title: "Two", Category: "coffee"}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Insert(&Post{Title: "Three", Category: "Gear"}); err != nil {
		t.Fatal(err)
	}

	categories, err := model.GetCategories()
	if err != nil {
		t.Fatal(err)
	}
	if len(categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(categories))
	}
	if categories[0].Category != "Coffee" || categories[0].ID != "coffee" {
		t.Fatalf("expected alphabetical categories with canonical case, got %+v", categories)
	}
	if categories[1].Category != "Gear" {
		t.Fatalf("expected Gear second, got %+v", categories)
	}
}

func mustID(t *testing.T, id string) int {
	t.Helper()

	parsed, err := strconv.Atoi(id)
	if err != nil {
		t.Fatalf("expected numeric id, got %q", id)
	}

	return parsed
}
