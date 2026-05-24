package posts

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	postdata "thom-server/internal/posts"
	"thom-server/internal/server/response"

	_ "github.com/mattn/go-sqlite3"
)

func TestGetPostCategories(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/categories", nil)
	rr := httptest.NewRecorder()

	app.GetCategories(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var categories []postdata.Category
	if err := json.Unmarshal(rr.Body.Bytes(), &categories); err != nil {
		t.Fatal(err)
	}
	if len(categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(categories))
	}
	if categories[0].Category != "Projects" {
		t.Fatalf("expected first category Projects, got %q", categories[0].Category)
	}
}

func TestGetPostsByCategory(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/post?category=1", nil)
	rr := httptest.NewRecorder()

	app.GetByCategory(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var gotPosts []postdata.Post
	if err := json.Unmarshal(rr.Body.Bytes(), &gotPosts); err != nil {
		t.Fatal(err)
	}
	if len(gotPosts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(gotPosts))
	}
	if gotPosts[1].Link != "https://example.com" {
		t.Fatalf("expected linked post URL, got %q", gotPosts[1].Link)
	}
}

func TestGetPostsByCategoryRejectsInvalidCategory(t *testing.T) {
	app := newTestHandler(t)

	tests := []string{
		"/post",
		"/post?category=0",
		"/post?category=abc",
	}

	for _, target := range tests {
		t.Run(target, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rr := httptest.NewRecorder()

			app.GetByCategory(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
			}
		})
	}
}

func TestGetPostContentByID(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/post/content/id?post=1", nil)
	rr := httptest.NewRecorder()

	app.GetContentByID(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var post postdata.Post
	if err := json.Unmarshal(rr.Body.Bytes(), &post); err != nil {
		t.Fatal(err)
	}
	if post.ID != 1 {
		t.Fatalf("expected post ID 1, got %d", post.ID)
	}
	if len(post.Content) != 2 {
		t.Fatalf("expected 2 content chunks, got %d", len(post.Content))
	}
}

func TestGetPostContentByIDRejectsInvalidPostID(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/post/content/id?post=-1", nil)
	rr := httptest.NewRecorder()

	app.GetContentByID(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestGetPostContentByIDHandlesMissingPost(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/post/content/id?post=999", nil)
	rr := httptest.NewRecorder()

	app.GetContentByID(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestGetPostContentByPathName(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/post/content/path?pathName=test-post", nil)
	rr := httptest.NewRecorder()

	app.GetContentByPathName(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var post postdata.Post
	if err := json.Unmarshal(rr.Body.Bytes(), &post); err != nil {
		t.Fatal(err)
	}
	if post.PathName != "test-post" {
		t.Fatalf("expected pathName test-post, got %q", post.PathName)
	}
}

func TestGetPostContentByPathNameRejectsMissingPathName(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/post/content/path", nil)
	rr := httptest.NewRecorder()

	app.GetContentByPathName(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestGetPostContentByPathNameHandlesMissingPost(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/post/content/path?pathName=missing", nil)
	rr := httptest.NewRecorder()

	app.GetContentByPathName(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestReadPositiveIntQuery(t *testing.T) {
	app := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/?id=42", nil)
	rr := httptest.NewRecorder()

	got, ok := app.readPositiveIntQuery(rr, req, "id")
	if !ok {
		t.Fatal("expected query value to be valid")
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected recorder to remain untouched, got %d", rr.Code)
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
		INSERT INTO PostCategories (id, Category) VALUES
			(1, 'Projects'),
			(2, 'Notes');
		INSERT INTO Posts (id, CategoryId, Title, Summary, PathName, Link)
		VALUES
			(1, 1, 'Test Post', 'Summary', 'test-post', NULL),
			(2, 1, 'Linked Post', 'Link summary', 'linked-post', 'https://example.com');
		INSERT INTO PostContent (id, PostId, Text, ImagePath) VALUES
			(1, 1, 'First chunk', '/first.png'),
			(2, 1, 'Second chunk', NULL),
			(3, 2, NULL, '/linked.png');`
	if _, err = db.Exec(fixture); err != nil {
		t.Fatal(err)
	}

	return New(
		&postdata.Model{DB: db},
		response.Responder{ErrorLog: log.New(io.Discard, "", 0)},
	)
}
