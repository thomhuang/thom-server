package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	blogdata "thom-server/internal/blog"
)

func TestBlogMutatingRoutesRequireAuth(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/blog", body: `{}`},
		{method: http.MethodPatch, path: "/blog/1", body: `{}`},
		{method: http.MethodDelete, path: "/blog/1"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
			}
		})
	}
}

func TestBlogBrowsingRoutesArePublic(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()

	for _, path := range []string{"/blog", "/blog/categories", "/blog/1"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Nothing is published in the fixture, so post 1 is a 404 and the
			// list is empty. Both prove the route is reachable anonymously.
			if rr.Code != http.StatusOK && rr.Code != http.StatusNotFound {
				t.Fatalf("expected 200 or 404, got %d", rr.Code)
			}
		})
	}
}

func TestBlogDraftsAreVisibleOnlyToAdmin(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()

	draft := createBlogPost(t, handler, loginCookie(t, handler), `{"title":"Draft post","body":"Not ready.","category":"Coffee"}`)
	if draft.Published {
		t.Fatal("expected the created post to be a draft")
	}

	anonymous := getBlogPosts(t, handler, nil, "")
	if len(anonymous) != 0 {
		t.Fatalf("expected drafts to be hidden from anonymous callers, got %d posts", len(anonymous))
	}

	asAdmin := getBlogPosts(t, handler, loginCookie(t, handler), "")
	if len(asAdmin) != 1 {
		t.Fatalf("expected the admin to see 1 draft, got %d", len(asAdmin))
	}

	// The detail route follows the same rule.
	req := httptest.NewRequest(http.MethodGet, "/blog/"+draft.ID, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected a draft detail request to 404 anonymously, got %d", rr.Code)
	}
}

func TestBlogAdminCanManageAPostEndToEnd(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()
	cookie := loginCookie(t, handler)

	post := createBlogPost(t, handler, cookie, `{"title":"Hello","body":"First *post*.","category":"Coffee","published":true}`)

	if post.Title != "Hello" || post.Body != "First *post*." || post.Category != "Coffee" {
		t.Fatalf("unexpected created post: %+v", post)
	}

	// Publishing makes it visible to anonymous callers.
	if len(getBlogPosts(t, handler, nil, "")) != 1 {
		t.Fatal("expected the published post to be publicly visible")
	}

	// Updating, including a category change.
	patch := httptest.NewRequest(http.MethodPatch, "/blog/"+post.ID, strings.NewReader(`{"body":"Edited.","category":"Gear"}`))
	patch.Header.Set("Origin", "http://localhost:3000")
	patch.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, patch)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d for update, got %d", http.StatusOK, rr.Code)
	}

	var updated blogdata.Post
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Body != "Edited." || updated.Category != "Gear" {
		t.Fatalf("unexpected update: %+v", updated)
	}

	// The post now lives under the gear category.
	if len(getBlogPosts(t, handler, nil, "coffee")) != 0 {
		t.Fatal("expected the post to leave the coffee category")
	}
	if len(getBlogPosts(t, handler, nil, "gear")) != 1 {
		t.Fatal("expected the post under the gear category")
	}

	// Deleting.
	del := httptest.NewRequest(http.MethodDelete, "/blog/"+post.ID, nil)
	del.Header.Set("Origin", "http://localhost:3000")
	del.AddCookie(cookie)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, del)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d for delete, got %d", http.StatusNoContent, rr.Code)
	}

	if len(getBlogPosts(t, handler, nil, "")) != 0 {
		t.Fatal("expected the post list to be empty after deletion")
	}
}

func TestBlogCreateRejectsMissingTitle(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()
	cookie := loginCookie(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/blog", strings.NewReader(`{"body":"No title.","category":"Coffee"}`))
	req.Header.Set("Origin", "http://localhost:3000")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestBlogCreateRejectsMissingCategory(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()
	cookie := loginCookie(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/blog", strings.NewReader(`{"title":"No category."}`))
	req.Header.Set("Origin", "http://localhost:3000")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestBlogCategoriesEndpoint(t *testing.T) {
	app := newTestApp(t)
	handler := app.Handler()
	cookie := loginCookie(t, handler)

	createBlogPost(t, handler, cookie, `{"title":"One","category":"Coffee","published":true}`)
	createBlogPost(t, handler, cookie, `{"title":"Two","category":"Gear","published":true}`)

	req := httptest.NewRequest(http.MethodGet, "/blog/categories", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var categories []*blogdata.Category
	if err := json.Unmarshal(rr.Body.Bytes(), &categories); err != nil {
		t.Fatal(err)
	}
	if len(categories) != 2 || categories[0].Category != "Coffee" || categories[1].Category != "Gear" {
		t.Fatalf("unexpected categories: %+v", categories)
	}
}

func createBlogPost(t *testing.T, handler http.Handler, cookie *http.Cookie, body string) blogdata.Post {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/blog", strings.NewReader(body))
	req.Header.Set("Origin", "http://localhost:3000")
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d for create, got %d (%s)", http.StatusCreated, rr.Code, rr.Body.String())
	}

	var post blogdata.Post
	if err := json.Unmarshal(rr.Body.Bytes(), &post); err != nil {
		t.Fatal(err)
	}

	return post
}

func getBlogPosts(t *testing.T, handler http.Handler, cookie *http.Cookie, category string) []*blogdata.Post {
	t.Helper()

	path := "/blog"
	if category != "" {
		path += "?category=" + category
	}

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var posts []*blogdata.Post
	if err := json.Unmarshal(rr.Body.Bytes(), &posts); err != nil {
		t.Fatal(err)
	}

	return posts
}
