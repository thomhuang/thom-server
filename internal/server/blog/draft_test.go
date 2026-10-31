package blog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	blogdata "thom-server/internal/blog"
)

func TestDraftEndpointsRoundTrip(t *testing.T) {
	handler, _ := newTestHandler(t)

	rr := serve(handler.GetDraft, http.MethodGet, "/blog/draft", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d for an empty draft, got %d", http.StatusOK, rr.Code)
	}
	var empty blogdata.Draft
	if err := json.Unmarshal(rr.Body.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Title != "" {
		t.Fatalf("expected an empty draft, got %+v", empty)
	}

	put := httptest.NewRequest(http.MethodPut, "/blog/draft", strings.NewReader(`{"title":"Drafting","body":"Some body","category":"Coffee","published":false}`))
	rr = httptest.NewRecorder()
	handler.PutDraft(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d for put, got %d (%s)", http.StatusOK, rr.Code, rr.Body.String())
	}

	rr = serve(handler.GetDraft, http.MethodGet, "/blog/draft", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d for get, got %d", http.StatusOK, rr.Code)
	}
	var saved blogdata.Draft
	if err := json.Unmarshal(rr.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Title != "Drafting" || saved.Body != "Some body" || saved.Category != "Coffee" || saved.Published {
		t.Fatalf("unexpected saved draft: %+v", saved)
	}

	del := httptest.NewRequest(http.MethodDelete, "/blog/draft", nil)
	rr = httptest.NewRecorder()
	handler.DeleteDraft(rr, del)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d for delete, got %d", http.StatusNoContent, rr.Code)
	}

	rr = serve(handler.GetDraft, http.MethodGet, "/blog/draft", "")
	var cleared blogdata.Draft
	if err := json.Unmarshal(rr.Body.Bytes(), &cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.Title != "" {
		t.Fatalf("expected a cleared draft, got %+v", cleared)
	}
}

func TestPutDraftRejectsOversizedTitle(t *testing.T) {
	handler, _ := newTestHandler(t)

	body := `{"title":"` + strings.Repeat("a", 201) + `"}`
	rr := serve(handler.PutDraft, http.MethodPut, "/blog/draft", body)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}
