package shop

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	shopdata "thom-server/internal/shop"
)

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
