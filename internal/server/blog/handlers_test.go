package blog

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	blogdata "thom-server/internal/blog"
	"thom-server/internal/server/response"
)

type fakeImageStore struct {
	presigned  string
	presignErr error
	deleteErr  error
	lastKey    string
	lastType   string
	deleted    []string
}

func (f *fakeImageStore) PresignPut(objectKey, contentType string, expires time.Duration) (string, error) {
	f.lastKey = objectKey
	f.lastType = contentType

	if f.presignErr != nil {
		return "", f.presignErr
	}

	return f.presigned, nil
}

func (f *fakeImageStore) Delete(objectKey string) error {
	f.deleted = append(f.deleted, objectKey)

	return f.deleteErr
}

func serve(handler http.HandlerFunc, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rr := httptest.NewRecorder()
	handler(rr, req)

	return rr
}

func newTestHandler(t *testing.T) (*Handler, *fakeImageStore) {
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

	model := &blogdata.Model{DB: db}
	if err := model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	images := &fakeImageStore{presigned: "https://r2.example.com/signed"}

	return New(
		model,
		images,
		"https://images.example.com/",
		response.Responder{ErrorLog: log.New(io.Discard, "", 0)},
		log.New(io.Discard, "", 0),
	), images
}

func decodePost(t *testing.T, rr *httptest.ResponseRecorder) blogdata.Post {
	t.Helper()

	var post blogdata.Post
	if err := json.Unmarshal(rr.Body.Bytes(), &post); err != nil {
		t.Fatal(err)
	}

	return post
}
