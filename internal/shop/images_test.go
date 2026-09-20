package shop

import (
	"errors"
	"testing"

	"thom-server/internal/data"
)

func TestModelAddImageRejectsDuplicateObjectKey(t *testing.T) {
	model := newTestModel(t)

	_, err := model.AddImage(2, &Image{ObjectKey: "shop/1/first.jpg"})
	if err == nil {
		t.Fatal("expected a unique constraint error for a reused object key")
	}
}

func TestModelDeleteImageScopesToItem(t *testing.T) {
	model := newTestModel(t)

	// Image 1 belongs to item 1, so asking for it through item 2 must miss.
	_, err := model.DeleteImage(2, 1)
	if !errors.Is(err, data.ErrNoRecord) {
		t.Fatalf("expected ErrNoRecord, got %v", err)
	}

	image, err := model.DeleteImage(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if image.ObjectKey != "shop/1/first.jpg" {
		t.Fatalf("expected the deleted image key back, got %q", image.ObjectKey)
	}

	count, err := model.CountImages(1)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 image left on item 1, got %d", count)
	}
}

func TestModelCountImages(t *testing.T) {
	model := newTestModel(t)

	testCases := []struct {
		itemID int
		want   int
	}{
		{itemID: 1, want: 2},
		{itemID: 2, want: 0},
	}

	for _, testCase := range testCases {
		count, err := model.CountImages(testCase.itemID)
		if err != nil {
			t.Fatal(err)
		}
		if count != testCase.want {
			t.Fatalf("CountImages(%d) = %d, want %d", testCase.itemID, count, testCase.want)
		}
	}
}
