package blog

import (
	"testing"
	"time"
)

func TestUploadLifecycle(t *testing.T) {
	model := newTestModel(t)

	if err := model.AddUpload("blog/fresh.jpg"); err != nil {
		t.Fatal(err)
	}
	// Simulate an upload that has already passed the TTL.
	if _, err := model.DB.Exec(
		`INSERT INTO BlogUploads (ObjectKey, CreatedAt) VALUES (?, ?)`,
		"blog/stale.jpg",
		time.Now().Add(-2*uploadTTL).Unix(),
	); err != nil {
		t.Fatal(err)
	}

	expired, err := model.ExpiredUploads(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0] != "blog/stale.jpg" {
		t.Fatalf("expected only the stale upload to be expired, got %v", expired)
	}

	if err := model.DeleteUploads([]string{"blog/fresh.jpg"}); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := model.DB.QueryRow(`SELECT COUNT(*) FROM BlogUploads`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected the stale upload to remain, got %d rows", count)
	}
}
