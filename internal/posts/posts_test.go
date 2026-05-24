package posts

import (
	"database/sql"
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"thom-server/internal/data"
)

func TestGetCategories(t *testing.T) {
	model := newTestModel(t)

	categories, err := model.GetCategories()
	if err != nil {
		t.Fatal(err)
	}

	if len(categories) != 1 {
		t.Fatalf("expected 1 category, got %d", len(categories))
	}
	if categories[0].Category != "Projects" {
		t.Fatalf("expected category Projects, got %q", categories[0].Category)
	}
}

func TestGetPostsByCategory(t *testing.T) {
	model := newTestModel(t)

	posts, err := model.GetPostsByCategory(1)
	if err != nil {
		t.Fatal(err)
	}

	if len(posts) != 1 {
		t.Fatalf("expected 1 post, got %d", len(posts))
	}
	if posts[0].Title != "Test Post" {
		t.Fatalf("expected Test Post, got %q", posts[0].Title)
	}
	if posts[0].Link != "" {
		t.Fatalf("expected null Link to scan as empty string, got %q", posts[0].Link)
	}
}

func TestGetPostWithContentByID(t *testing.T) {
	model := newTestModel(t)

	post, err := model.GetPostWithContentByID(1)
	if err != nil {
		t.Fatal(err)
	}

	if post.Title != "Test Post" {
		t.Fatalf("expected Test Post, got %q", post.Title)
	}
	if len(post.Content) != 2 {
		t.Fatalf("expected 2 content chunks, got %d", len(post.Content))
	}
	if post.Content[0].Text != "First chunk" {
		t.Fatalf("expected first content chunk, got %q", post.Content[0].Text)
	}
	if post.Content[1].ImagePath != "" {
		t.Fatalf("expected null ImagePath to scan as empty string, got %q", post.Content[1].ImagePath)
	}
}

func TestGetPostWithContentByPathName(t *testing.T) {
	model := newTestModel(t)

	post, err := model.GetPostWithContentByPathName("test-post")
	if err != nil {
		t.Fatal(err)
	}

	if post.ID != 1 {
		t.Fatalf("expected post ID 1, got %d", post.ID)
	}
}

func TestGetPostWithContentNoRecord(t *testing.T) {
	model := newTestModel(t)

	_, err := model.GetPostWithContentByID(999)
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}
}

func newTestModel(t *testing.T) *Model {
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

	schema := `
		CREATE TABLE PostCategories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			Category VARCHAR(50) NOT NULL
		);
		CREATE TABLE Posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			CategoryId int NOT NULL,
			Title VARCHAR(255) NOT NULL,
			Summary VARCHAR(255) NOT NULL,
			PathName VARCHAR(50) NOT NULL,
			Link varchar(255),
			FOREIGN KEY (CategoryId) REFERENCES PostCategories(id)
		);
		CREATE TABLE PostContent (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			PostId int,
			Text text,
			ImagePath text,
			FOREIGN KEY (PostId) REFERENCES Posts(id)
		);`
	if _, err = db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	fixture := `
		INSERT INTO PostCategories (id, Category) VALUES (1, 'Projects');
		INSERT INTO Posts (id, CategoryId, Title, Summary, PathName, Link)
		VALUES (1, 1, 'Test Post', 'Summary', 'test-post', NULL);
		INSERT INTO PostContent (id, PostId, Text, ImagePath) VALUES
			(1, 1, 'First chunk', '/first.png'),
			(2, 1, 'Second chunk', NULL);`
	if _, err = db.Exec(fixture); err != nil {
		t.Fatal(err)
	}

	return &Model{DB: db}
}
