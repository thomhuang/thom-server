package shop

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
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

func TestValidObjectKey(t *testing.T) {
	testCases := []struct {
		name      string
		objectKey string
		itemID    int
		want      bool
	}{
		{name: "minted key", objectKey: "shop/4/abcdef.png", itemID: 4, want: true},
		{name: "wrong item prefix", objectKey: "shop/5/abcdef.png", itemID: 4, want: false},
		{name: "traversal", objectKey: "shop/4/../../etc/passwd", itemID: 4, want: false},
		{name: "nested path", objectKey: "shop/4/nested/abcdef.png", itemID: 4, want: false},
		{name: "disallowed extension", objectKey: "shop/4/abcdef.svg", itemID: 4, want: false},
		{name: "no extension", objectKey: "shop/4/abcdef", itemID: 4, want: false},
		{name: "empty", objectKey: "", itemID: 4, want: false},
		{name: "bare prefix", objectKey: "shop/4/", itemID: 4, want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := validObjectKey(testCase.objectKey, testCase.itemID); got != testCase.want {
				t.Fatalf("validObjectKey(%q, %d) = %v, want %v", testCase.objectKey, testCase.itemID, got, testCase.want)
			}
		})
	}
}

func TestIsValidItem(t *testing.T) {
	valid := shopdata.Item{Title: "Mug", PriceCents: 1800, Stock: 1, Currency: "usd"}

	testCases := []struct {
		name   string
		mutate func(*shopdata.Item)
		valid  bool
	}{
		{name: "valid", mutate: func(*shopdata.Item) {}, valid: true},
		{name: "missing title", mutate: func(i *shopdata.Item) { i.Title = "" }, valid: false},
		{name: "price must be positive", mutate: func(i *shopdata.Item) { i.PriceCents = 0 }, valid: false},
		{name: "negative price", mutate: func(i *shopdata.Item) { i.PriceCents = -100 }, valid: false},
		{name: "negative stock", mutate: func(i *shopdata.Item) { i.Stock = -1 }, valid: false},
		{name: "zero stock is allowed", mutate: func(i *shopdata.Item) { i.Stock = 0 }, valid: true},
		{name: "bad currency length", mutate: func(i *shopdata.Item) { i.Currency = "us" }, valid: false},
		{name: "uppercase currency", mutate: func(i *shopdata.Item) { i.Currency = "USD" }, valid: false},
		{name: "long title", mutate: func(i *shopdata.Item) { i.Title = strings.Repeat("a", maxTitleLength+1) }, valid: false},
		{name: "decimal measurements", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{
				{Label: "Waist", ValueInches: 24.5},
				{Label: "Inseam", ValueInches: 22.5},
			}
		}, valid: true},
		{name: "omitted measurements are allowed", mutate: func(i *shopdata.Item) {
			i.Measurements = nil
		}, valid: true},
		{name: "zero measurement is rejected", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "Waist", ValueInches: 0}}
		}, valid: false},
		{name: "negative measurement", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "Waist", ValueInches: -1}}
		}, valid: false},
		{name: "absurd measurement", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "Waist", ValueInches: maxMeasurementInches + 1}}
		}, valid: false},
		{name: "NaN measurement", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "Waist", ValueInches: math.NaN()}}
		}, valid: false},
		{name: "infinite measurement", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "Inseam", ValueInches: math.Inf(1)}}
		}, valid: false},
		{name: "blank measurement label", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{Label: "  ", ValueInches: 10}}
		}, valid: false},
		{name: "long measurement label", mutate: func(i *shopdata.Item) {
			i.Measurements = []*shopdata.Measurement{{
				Label:       strings.Repeat("a", maxMeasurementLabelLength+1),
				ValueInches: 10,
			}}
		}, valid: false},
		{name: "too many measurements", mutate: func(i *shopdata.Item) {
			measurements := make([]*shopdata.Measurement, maxItemMeasurements+1)
			for n := range measurements {
				measurements[n] = &shopdata.Measurement{Label: "Waist", ValueInches: 10}
			}
			i.Measurements = measurements
		}, valid: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			item := valid
			testCase.mutate(&item)

			err := isValidItem(&item)
			if testCase.valid && err != nil {
				t.Fatalf("expected valid, got %v", err)
			}
			if !testCase.valid && err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

func TestNormalizeItemTrimsAndDefaultsCurrency(t *testing.T) {
	item := &shopdata.Item{Title: "  Mug  ", Description: "  A mug  ", Currency: "  "}

	normalizeItem(item)

	if item.Title != "Mug" {
		t.Fatalf("title = %q, want Mug", item.Title)
	}
	if item.Description != "A mug" {
		t.Fatalf("description = %q, want A mug", item.Description)
	}
	if item.Currency != shopdata.DefaultCurrency {
		t.Fatalf("currency = %q, want %q", item.Currency, shopdata.DefaultCurrency)
	}
}

func TestGetItemsHidesDraftsFromAnonymousCallers(t *testing.T) {
	handler, _ := newTestHandler(t)

	items := decodeItems(t, serve(handler.GetItems, http.MethodGet, "/shop/items", ""))

	if len(items) != 2 {
		t.Fatalf("expected 2 published items, got %d", len(items))
	}
	for _, item := range items {
		if !item.IsPublished {
			t.Fatalf("item %s leaked a draft to an anonymous caller", item.ID)
		}
	}
}

func TestGetItemsDecoratesPrimaryImageURL(t *testing.T) {
	handler, _ := newTestHandler(t)

	items := decodeItems(t, serve(handler.GetItems, http.MethodGet, "/shop/items", ""))

	var found bool
	for _, item := range items {
		if item.ID == "1" {
			found = true
			want := "https://images.example.com/shop/1/second.jpg"
			if item.PrimaryImageURL != want {
				t.Fatalf("primaryImageUrl = %q, want %q", item.PrimaryImageURL, want)
			}
		}
	}
	if !found {
		t.Fatal("expected item 1 in the list")
	}
}

func TestGetItemByIDHidesDraftsFromAnonymousCallers(t *testing.T) {
	handler, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/shop/items/3", nil)
	req.SetPathValue("id", "3")
	rr := httptest.NewRecorder()

	handler.GetItemByID(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d for a draft, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestGetItemByIDDecoratesImages(t *testing.T) {
	handler, _ := newTestHandler(t)

	rr := serve(handler.GetItemByID, http.MethodGet, "/shop/items/1", "", "id", "1")

	var item shopdata.Item
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}

	if len(item.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(item.Images))
	}
	for _, image := range item.Images {
		want := "https://images.example.com/" + image.ObjectKey
		if image.URL != want {
			t.Fatalf("image url = %q, want %q", image.URL, want)
		}
	}
}

func TestGetItemByIDMissing(t *testing.T) {
	handler, _ := newTestHandler(t)

	rr := serve(handler.GetItemByID, http.MethodGet, "/shop/items/999", "", "id", "999")

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestCreateItem(t *testing.T) {
	handler, _ := newTestHandler(t)

	body := `{"title":"Pour-over kit","description":"A kit","priceCents":4500,"stock":2,"currency":"usd","isPublished":true}`
	rr := serve(handler.CreateItem, http.MethodPost, "/shop/items", body)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusCreated, rr.Code, rr.Body.String())
	}

	var created shopdata.Item
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Title != "Pour-over kit" {
		t.Fatalf("title = %q, want Pour-over kit", created.Title)
	}
	if created.PriceCents != 4500 {
		t.Fatalf("priceCents = %d, want 4500", created.PriceCents)
	}
	if !created.IsPublished {
		t.Fatal("expected the item to be published")
	}
}

func TestNormalizeItemRoundsMeasurementsToOneDecimal(t *testing.T) {
	item := &shopdata.Item{
		Title:      "Jacket",
		PriceCents: 1000,
		Measurements: []*shopdata.Measurement{
			{Label: "  Pit to pit  ", ValueInches: 24.46},
			{Label: "Back length", ValueInches: 22.55},
			{Label: "Shoulder", ValueInches: 18.04},
		},
	}

	normalizeItem(item)

	if len(item.Measurements) != 3 {
		t.Fatalf("measurements = %+v, want 3 rows", item.Measurements)
	}
	if got := measurementByLabel(t, item.Measurements, "Pit to pit").ValueInches; got != 24.5 {
		t.Fatalf("pit-to-pit = %v, want 24.5", got)
	}
	if got := measurementByLabel(t, item.Measurements, "Back length").ValueInches; got != 22.6 {
		t.Fatalf("back length = %v, want 22.6", got)
	}
	if got := measurementByLabel(t, item.Measurements, "Shoulder").ValueInches; got != 18 {
		t.Fatalf("shoulder = %v, want 18", got)
	}
}

func TestNormalizeMeasurementsDropsBlanksAndDuplicates(t *testing.T) {
	item := &shopdata.Item{
		Measurements: []*shopdata.Measurement{
			{Label: "Waist", ValueInches: 31},
			{Label: "   ", ValueInches: 10},
			{Label: "waist", ValueInches: 99},
			nil,
			{Label: "Inseam", ValueInches: 30},
		},
	}

	normalizeItem(item)

	if len(item.Measurements) != 2 {
		t.Fatalf("measurements = %+v, want 2 rows", item.Measurements)
	}
	if got := measurementByLabel(t, item.Measurements, "Waist").ValueInches; got != 31 {
		t.Fatalf("waist = %v, want the first value 31", got)
	}
}

func TestCreateItemAcceptsMeasurements(t *testing.T) {
	handler, _ := newTestHandler(t)

	body := `{"title":"Denim jacket","category":"tops","size":"Large","priceCents":8500,"stock":1,"currency":"usd",` +
		`"measurements":[{"label":"Pit to pit","valueInches":24.5},{"label":"Shoulder","valueInches":18.25}]}`
	rr := serve(handler.CreateItem, http.MethodPost, "/shop/items", body)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusCreated, rr.Code, rr.Body.String())
	}

	var created shopdata.Item
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Category != "tops" {
		t.Fatalf("category = %q, want tops", created.Category)
	}
	if created.Size != "Large" {
		t.Fatalf("size = %q, want Large", created.Size)
	}
	if got := measurementByLabel(t, created.Measurements, "Pit to pit").ValueInches; got != 24.5 {
		t.Fatalf("pit to pit = %v, want 24.5", got)
	}
	if got := measurementByLabel(t, created.Measurements, "Shoulder").ValueInches; got != 18.3 {
		t.Fatalf("shoulder = %v, want 18.3 (18.25 rounds to one decimal)", got)
	}
}

func TestCreateItemWithoutMeasurementsLeavesThemEmpty(t *testing.T) {
	handler, _ := newTestHandler(t)

	// A listing that is not clothing omits the measurements entirely.
	body := `{"title":"Coffee mug","priceCents":1800,"stock":3,"currency":"usd"}`
	rr := serve(handler.CreateItem, http.MethodPost, "/shop/items", body)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusCreated, rr.Code, rr.Body.String())
	}

	var created shopdata.Item
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.Measurements) != 0 {
		t.Fatalf("measurements = %+v, want none", created.Measurements)
	}
}

func TestUpdateItemRejectsNonPositiveMeasurement(t *testing.T) {
	handler, _ := newTestHandler(t)

	rr := serve(handler.UpdateItem, http.MethodPatch, "/shop/items/1",
		`{"measurements":[{"label":"Waist","valueInches":0}]}`, "id", "1")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusBadRequest, rr.Code, rr.Body.String())
	}
}

func TestUpdateItemReplacesMeasurements(t *testing.T) {
	handler, _ := newTestHandler(t)

	rr := serve(handler.UpdateItem, http.MethodPatch, "/shop/items/1",
		`{"measurements":[{"label":"Waist","valueInches":21.5},{"label":"Inseam","valueInches":27.5}]}`, "id", "1")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, rr.Code, rr.Body.String())
	}

	var updated shopdata.Item
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if got := measurementByLabel(t, updated.Measurements, "Waist").ValueInches; got != 21.5 {
		t.Fatalf("waist = %v, want 21.5", got)
	}
	if got := measurementByLabel(t, updated.Measurements, "Inseam").ValueInches; got != 27.5 {
		t.Fatalf("inseam = %v, want 27.5", got)
	}
}

func TestUpdateItemOmittingMeasurementsPreservesThem(t *testing.T) {
	handler, _ := newTestHandler(t)

	seed := serve(handler.UpdateItem, http.MethodPatch, "/shop/items/1",
		`{"measurements":[{"label":"Waist","valueInches":31}]}`, "id", "1")
	if seed.Code != http.StatusOK {
		t.Fatalf("seed status = %d (%s)", seed.Code, seed.Body.String())
	}

	rr := serve(handler.UpdateItem, http.MethodPatch, "/shop/items/1", `{"title":"Renamed"}`, "id", "1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, rr.Code, rr.Body.String())
	}

	var updated shopdata.Item
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Renamed" {
		t.Fatalf("title = %q, want Renamed", updated.Title)
	}
	if got := measurementByLabel(t, updated.Measurements, "Waist").ValueInches; got != 31 {
		t.Fatalf("waist = %v, want the preserved 31", got)
	}
}

func TestUpdateItemEmptyMeasurementsClearsThem(t *testing.T) {
	handler, _ := newTestHandler(t)

	seed := serve(handler.UpdateItem, http.MethodPatch, "/shop/items/1",
		`{"measurements":[{"label":"Waist","valueInches":31}]}`, "id", "1")
	if seed.Code != http.StatusOK {
		t.Fatalf("seed status = %d (%s)", seed.Code, seed.Body.String())
	}

	rr := serve(handler.UpdateItem, http.MethodPatch, "/shop/items/1", `{"measurements":[]}`, "id", "1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, rr.Code, rr.Body.String())
	}

	var updated shopdata.Item
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Measurements) != 0 {
		t.Fatalf("measurements = %+v, want cleared", updated.Measurements)
	}
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

func TestCreateItemRejectsInvalidPayload(t *testing.T) {
	handler, _ := newTestHandler(t)

	testCases := []struct {
		name string
		body string
	}{
		{name: "missing title", body: `{"priceCents":100}`},
		{name: "zero price", body: `{"title":"Free","priceCents":0}`},
		{name: "negative stock", body: `{"title":"Mug","priceCents":100,"stock":-1}`},
		{name: "bad currency", body: `{"title":"Mug","priceCents":100,"currency":"dollars"}`},
		{name: "malformed json", body: `{"title":`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			rr := serve(handler.CreateItem, http.MethodPost, "/shop/items", testCase.body)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
			}
		})
	}
}

func TestUpdateItemAppliesPatch(t *testing.T) {
	handler, _ := newTestHandler(t)

	rr := serve(handler.UpdateItem, http.MethodPatch, "/shop/items/2", `{"title":"Renamed","stock":7,"size":"Medium"}`, "id", "2")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, rr.Code, rr.Body.String())
	}

	var updated shopdata.Item
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Renamed" {
		t.Fatalf("title = %q, want Renamed", updated.Title)
	}
	if updated.Stock != 7 {
		t.Fatalf("stock = %d, want 7", updated.Stock)
	}
	if updated.Size != "Medium" {
		t.Fatalf("size = %q, want Medium", updated.Size)
	}
	if updated.Description != "A bag of beans" {
		t.Fatalf("expected untouched fields to survive, got description %q", updated.Description)
	}
}

func TestUpdateItemRejectsInvalidPatch(t *testing.T) {
	handler, _ := newTestHandler(t)

	rr := serve(handler.UpdateItem, http.MethodPatch, "/shop/items/2", `{"priceCents":0}`, "id", "2")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestDeleteItemRemovesUploadedObjects(t *testing.T) {
	handler, images := newTestHandler(t)

	rr := serve(handler.DeleteItem, http.MethodDelete, "/shop/items/1", "", "id", "1")

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
	if len(images.deleted) != 2 {
		t.Fatalf("expected 2 objects deleted, got %d (%v)", len(images.deleted), images.deleted)
	}
}

func TestDeleteItemStillSucceedsWhenObjectDeleteFails(t *testing.T) {
	handler, images := newTestHandler(t)
	images.deleteErr = errors.New("r2 unavailable")

	rr := serve(handler.DeleteItem, http.MethodDelete, "/shop/items/1", "", "id", "1")

	// The rows are already gone, so the request should still report success
	// rather than leaving the client thinking the item survived.
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}

func TestPresignImageUpload(t *testing.T) {
	handler, images := newTestHandler(t)

	rr := serve(handler.PresignImageUpload, http.MethodPost, "/shop/items/2/images/presign", `{"contentType":"image/PNG"}`, "id", "2")

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, rr.Code, rr.Body.String())
	}

	var presigned presignResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &presigned); err != nil {
		t.Fatal(err)
	}

	if presigned.ContentType != "image/png" {
		t.Fatalf("contentType = %q, want the normalized image/png", presigned.ContentType)
	}
	if !strings.HasPrefix(presigned.ObjectKey, "shop/2/") {
		t.Fatalf("objectKey = %q, want the shop/2/ prefix", presigned.ObjectKey)
	}
	if !strings.HasSuffix(presigned.ObjectKey, ".png") {
		t.Fatalf("objectKey = %q, want a .png extension", presigned.ObjectKey)
	}
	if images.lastType != "image/png" {
		t.Fatalf("store received content type %q, want image/png", images.lastType)
	}
	if images.lastKey != presigned.ObjectKey {
		t.Fatalf("store signed %q but handler reported %q", images.lastKey, presigned.ObjectKey)
	}
}

func TestPresignImageUploadRejectsUnsupportedType(t *testing.T) {
	handler, _ := newTestHandler(t)

	testCases := []string{
		`{"contentType":"image/svg+xml"}`,
		`{"contentType":"text/html"}`,
		`{"contentType":""}`,
	}

	for _, body := range testCases {
		t.Run(body, func(t *testing.T) {
			rr := serve(handler.PresignImageUpload, http.MethodPost, "/shop/items/2/images/presign", body, "id", "2")
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
			}
		})
	}
}

func TestPresignImageUploadRejectsWhenImageLimitReached(t *testing.T) {
	handler, _ := newTestHandler(t)

	// Item 2 starts with no images, so this brings it exactly to the limit.
	for index := 0; index < shopdata.MaxItemImages; index++ {
		if _, err := handler.shop.DB.Exec(
			`INSERT INTO ShopItemImages (ItemID, ObjectKey) VALUES (2, ?)`,
			fmt.Sprintf("shop/2/limit-%d.jpg", index),
		); err != nil {
			t.Fatal(err)
		}
	}

	rr := serve(handler.PresignImageUpload, http.MethodPost, "/shop/items/2/images/presign", `{"contentType":"image/png"}`, "id", "2")

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d once the limit is reached, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestPresignImageUploadReportsUnavailableWhenStoreFails(t *testing.T) {
	handler, images := newTestHandler(t)
	images.presignErr = errors.New("credentials are not configured")

	rr := serve(handler.PresignImageUpload, http.MethodPost, "/shop/items/2/images/presign", `{"contentType":"image/png"}`, "id", "2")

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestPresignImageUploadMissingItem(t *testing.T) {
	handler, _ := newTestHandler(t)

	rr := serve(handler.PresignImageUpload, http.MethodPost, "/shop/items/999/images/presign", `{"contentType":"image/png"}`, "id", "999")

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestCreateImage(t *testing.T) {
	handler, _ := newTestHandler(t)

	body := `{"objectKey":"shop/2/abc123.webp","altText":"  A mug  ","sortOrder":50}`
	rr := serve(handler.CreateImage, http.MethodPost, "/shop/items/2/images", body, "id", "2")

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusCreated, rr.Code, rr.Body.String())
	}

	var image shopdata.Image
	if err := json.Unmarshal(rr.Body.Bytes(), &image); err != nil {
		t.Fatal(err)
	}
	if image.AltText != "A mug" {
		t.Fatalf("altText = %q, want the trimmed value", image.AltText)
	}
	if image.SortOrder != 50 {
		t.Fatalf("sortOrder = %d, want 50", image.SortOrder)
	}
	if image.URL != "https://images.example.com/shop/2/abc123.webp" {
		t.Fatalf("url = %q", image.URL)
	}
}

func TestCreateImageRejectsForeignObjectKey(t *testing.T) {
	handler, _ := newTestHandler(t)

	testCases := []string{
		`{"objectKey":"shop/1/stolen.jpg"}`,
		`{"objectKey":"shop/2/../1/stolen.jpg"}`,
		`{"objectKey":"shop/2/nested/abc.jpg"}`,
		`{"objectKey":"shop/2/abc.svg"}`,
		`{"objectKey":""}`,
	}

	for _, body := range testCases {
		t.Run(body, func(t *testing.T) {
			rr := serve(handler.CreateImage, http.MethodPost, "/shop/items/2/images", body, "id", "2")
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
			}
		})
	}
}

func TestCreateImageMissingItem(t *testing.T) {
	handler, _ := newTestHandler(t)

	rr := serve(handler.CreateImage, http.MethodPost, "/shop/items/999/images", `{"objectKey":"shop/999/abc.jpg"}`, "id", "999")

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
}

func TestDeleteImageRemovesObject(t *testing.T) {
	handler, images := newTestHandler(t)

	rr := serve(handler.DeleteImage, http.MethodDelete, "/shop/items/1/images/1", "", "id", "1", "imageId", "1")

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
	if len(images.deleted) != 1 || images.deleted[0] != "shop/1/first.jpg" {
		t.Fatalf("deleted = %v, want [shop/1/first.jpg]", images.deleted)
	}
}

func TestDeleteImageScopesToItem(t *testing.T) {
	handler, images := newTestHandler(t)

	rr := serve(handler.DeleteImage, http.MethodDelete, "/shop/items/2/images/1", "", "id", "2", "imageId", "1")

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rr.Code)
	}
	if len(images.deleted) != 0 {
		t.Fatalf("expected no objects deleted, got %v", images.deleted)
	}
}

func TestReadPositiveIntPathRejectsNonPositive(t *testing.T) {
	handler, _ := newTestHandler(t)

	for _, value := range []string{"0", "-1", "abc", ""} {
		t.Run(value, func(t *testing.T) {
			rr := serve(handler.GetItemByID, http.MethodGet, "/shop/items/x", "", "id", value)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d for %q, got %d", http.StatusBadRequest, value, rr.Code)
			}
		})
	}
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
