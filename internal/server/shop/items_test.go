package shop

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	shopdata "thom-server/internal/shop"
)

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

	rr := serve(handler.GetItemByID, http.MethodGet, "/shop/items/3", "", "id", "3")

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
