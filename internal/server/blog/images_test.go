package blog

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPresignImageUpload(t *testing.T) {
	handler, images := newTestHandler(t)

	rr := serve(handler.PresignImageUpload, http.MethodPost, "/blog/images/presign", `{"contentType":"image/PNG"}`)

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
	if !strings.HasPrefix(presigned.ObjectKey, "blog/") {
		t.Fatalf("objectKey = %q, want the blog/ prefix", presigned.ObjectKey)
	}
	if !strings.HasSuffix(presigned.ObjectKey, ".png") {
		t.Fatalf("objectKey = %q, want a .png extension", presigned.ObjectKey)
	}
	if presigned.URL != "https://images.example.com/"+presigned.ObjectKey {
		t.Fatalf("url = %q, want the composed public URL", presigned.URL)
	}
	if images.lastType != "image/png" {
		t.Fatalf("store received content type %q, want image/png", images.lastType)
	}
	if images.lastKey != presigned.ObjectKey {
		t.Fatalf("store signed %q but handler reported %q", images.lastKey, presigned.ObjectKey)
	}

	var count int
	if err := handler.blog.DB.QueryRow(`SELECT COUNT(*) FROM BlogUploads`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 pending upload, got %d", count)
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
			rr := serve(handler.PresignImageUpload, http.MethodPost, "/blog/images/presign", body)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
			}
		})
	}
}

func TestReferencedObjectKeys(t *testing.T) {
	key, err := newObjectKey("webp")
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name string
		body string
		want []string
	}{
		{name: "no images", body: "plain markdown", want: nil},
		{name: "one image", body: "![alt](https://images.example.com/" + key + ")", want: []string{key}},
		{name: "deduplicates", body: "![](" + key + ") and ![](" + key + ")", want: []string{key}},
		{name: "ignores foreign keys", body: "![x](https://img.thomhuang.com/shop/1/abc.webp)", want: nil},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := referencedObjectKeys(testCase.body)
			if len(got) != len(testCase.want) {
				t.Fatalf("referencedObjectKeys(%q) = %v, want %v", testCase.body, got, testCase.want)
			}
			for index := range testCase.want {
				if got[index] != testCase.want[index] {
					t.Fatalf("referencedObjectKeys(%q) = %v, want %v", testCase.body, got, testCase.want)
				}
			}
		})
	}
}

func TestCreatePostCommitsUploads(t *testing.T) {
	handler, _ := newTestHandler(t)

	key, err := newObjectKey("jpg")
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.blog.AddUpload(key); err != nil {
		t.Fatal(err)
	}

	body := `{"title":"With image","body":"Look: ![](https://images.example.com/` + key + `)","category":"Coffee"}`
	rr := serve(handler.CreatePost, http.MethodPost, "/blog", body)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d (%s)", http.StatusCreated, rr.Code, rr.Body.String())
	}

	var count int
	if err := handler.blog.DB.QueryRow(`SELECT COUNT(*) FROM BlogUploads WHERE ObjectKey = ?`, key).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("expected the referenced upload to be committed, but it is still pending")
	}
}
