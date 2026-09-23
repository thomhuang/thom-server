package blog

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	presignExpiry   = 10 * time.Minute
	objectKeyPrefix = "blog"
)

// allowedImageTypes maps an accepted upload type to the stored file extension.
var allowedImageTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/avif": "avif",
	"image/gif":  "gif",
}

type presignRequest struct {
	ContentType string `json:"contentType"`
}

type presignResponse struct {
	ObjectKey string `json:"objectKey"`
	UploadURL string `json:"uploadUrl"`
	// ContentType is echoed back because it is part of the signature; the
	// browser must send exactly this header for the upload to be accepted.
	ContentType string `json:"contentType"`
	ExpiresAt   string `json:"expiresAt"`
	// URL is the public address of the uploaded object, ready to paste into
	// the post body as markdown.
	URL string `json:"url"`
}

// PresignImageUpload returns a short-lived URL the browser PUTs a pasted image
// to, and records the key as pending so the sweeper can reclaim it if the post
// is abandoned.
func (h *Handler) PresignImageUpload(w http.ResponseWriter, r *http.Request) {
	var request presignRequest
	if err := h.responder.DecodeJSON(w, r.Body, &request); err != nil {
		h.infoLog.Printf("failed to decode presign JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	contentType := strings.ToLower(strings.TrimSpace(request.ContentType))
	extension, allowed := allowedImageTypes[contentType]
	if !allowed {
		h.infoLog.Printf("unsupported image content type %q", contentType)
		h.responder.BadRequest(w)
		return
	}

	objectKey, err := newObjectKey(extension)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	uploadURL, err := h.images.PresignPut(objectKey, contentType, presignExpiry)
	if err != nil {
		h.infoLog.Printf("failed to presign upload: %v", err)
		h.responder.ClientError(w, http.StatusServiceUnavailable)
		return
	}

	if err := h.blog.AddUpload(objectKey); err != nil {
		h.responder.ServerError(w, err)
		return
	}

	h.infoLog.Printf("PRESIGN_IMAGE key=%s", objectKey)

	err = h.responder.WriteJSON(w, http.StatusOK, presignResponse{
		ObjectKey:   objectKey,
		UploadURL:   uploadURL,
		ContentType: contentType,
		ExpiresAt:   time.Now().Add(presignExpiry).UTC().Format(time.RFC3339),
		URL:         h.publicImageURL(objectKey),
	}, nil)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) publicImageURL(objectKey string) string {
	if objectKey == "" || h.publicURL == "" {
		return ""
	}

	return h.publicURL + "/" + objectKey
}

// newObjectKey builds blog/{random}.{ext}. The random name prevents collisions
// and keeps guesses from overwriting existing uploads.
func newObjectKey(extension string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%s.%s", objectKeyPrefix, hex.EncodeToString(random), extension), nil
}

// objectKeyPattern matches the object keys this server mints, so a saved body
// can be reconciled against the pending uploads table without the client
// reporting its keys separately.
var objectKeyPattern = regexp.MustCompile(`blog/[0-9a-f]{32}\.(?:jpg|png|webp|avif|gif)`)

// referencedObjectKeys returns the unique object keys a post body embeds.
func referencedObjectKeys(body string) []string {
	matches := objectKeyPattern.FindAllString(body, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	keys := make([]string, 0, len(matches))
	for _, key := range matches {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}

	return keys
}
