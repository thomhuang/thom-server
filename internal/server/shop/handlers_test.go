package shop

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

	"thom-server/internal/server/response"
	shopdata "thom-server/internal/shop"
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

// serve calls a handler directly with path values applied.
func serve(handler http.HandlerFunc, method, target, body string, pathValues ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for index := 0; index+1 < len(pathValues); index += 2 {
		req.SetPathValue(pathValues[index], pathValues[index+1])
	}

	rr := httptest.NewRecorder()
	handler(rr, req)

	return rr
}

func decodeItems(t *testing.T, rr *httptest.ResponseRecorder) []*shopdata.ItemSummary {
	t.Helper()

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var items []*shopdata.ItemSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}

	return items
}

// measurementByLabel returns the row for a label, failing the test if it is
// absent.
func measurementByLabel(t *testing.T, measurements []*shopdata.Measurement, label string) *shopdata.Measurement {
	t.Helper()

	for _, measurement := range measurements {
		if strings.EqualFold(measurement.Label, label) {
			return measurement
		}
	}

	t.Fatalf("measurement %q not found in %+v", label, measurements)
	return nil
}

func newTestHandler(t *testing.T) (*Handler, *fakeImageStore) {
	t.Helper()

	db := newTestDB(t)

	images := &fakeImageStore{presigned: "https://r2.example.com/signed"}

	return New(
		&shopdata.Model{DB: db},
		images,
		"https://images.example.com/",
		response.Responder{ErrorLog: log.New(io.Discard, "", 0)},
		log.New(io.Discard, "", 0),
	), images
}

func newTestDB(t *testing.T) *sql.DB {
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

	model := &shopdata.Model{DB: db}
	if err = model.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	fixture := `
		INSERT INTO ShopItems (id, Title, Description, PriceCents, Currency, Stock, IsPublished)
		VALUES
			(1, 'Test mug', 'A mug', 1800, 'usd', 5, 1),
			(2, 'Test beans', 'A bag of beans', 2200, 'usd', 10, 1),
			(3, 'Draft item', 'Not published yet', 900, 'usd', 1, 0);

		INSERT INTO ShopItemImages (id, ItemID, ObjectKey, AltText, SortOrder)
		VALUES
			(1, 1, 'shop/1/first.jpg', 'First', 2000),
			(2, 1, 'shop/1/second.jpg', 'Second', 1000);`

	if _, err = db.Exec(fixture); err != nil {
		t.Fatal(err)
	}

	return db
}
