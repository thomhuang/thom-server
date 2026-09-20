package shop

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	shopdata "thom-server/internal/shop"
)

const (
	presignExpiry    = 10 * time.Minute
	defaultSortOrder = shopdata.DefaultImageSortOrder
	objectKeyPrefix  = "shop"
)

// allowedImageTypes maps an accepted upload type to the stored file extension.
// The extension comes from this table rather than the client filename so a
// crafted name cannot escape the item's key prefix.
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
}

type imageRequest struct {
	ObjectKey string `json:"objectKey"`
	AltText   string `json:"altText"`
	SortOrder *int   `json:"sortOrder"`
}

// PresignImageUpload returns a short-lived URL the browser PUTs an image to, so
// image bytes never pass through the container.
func (h *Handler) PresignImageUpload(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

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

	if !h.checkImageCapacity(w, id) {
		return
	}

	objectKey, err := newObjectKey(id, extension)
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

	h.infoLog.Printf("PRESIGN_IMAGE item=%d key=%s", id, objectKey)

	err = h.responder.WriteJSON(w, http.StatusOK, presignResponse{
		ObjectKey:   objectKey,
		UploadURL:   uploadURL,
		ContentType: contentType,
		ExpiresAt:   time.Now().Add(presignExpiry).UTC().Format(time.RFC3339),
	}, nil)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

// checkImageCapacity rejects a presign for a missing item or one that already
// holds the maximum number of images.
func (h *Handler) checkImageCapacity(w http.ResponseWriter, itemID int) bool {
	exists, imageCount, err := h.shop.ItemImageCount(itemID)
	if err != nil {
		h.responder.ServerError(w, err)
		return false
	}
	if !exists {
		h.responder.NotFound(w)
		return false
	}
	if imageCount >= shopdata.MaxItemImages {
		h.infoLog.Printf("item %d already has the maximum number of images", itemID)
		h.responder.BadRequest(w)
		return false
	}

	return true
}

// CreateImage records an object that the browser has already uploaded.
func (h *Handler) CreateImage(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	var request imageRequest
	if err := h.responder.DecodeJSON(w, r.Body, &request); err != nil {
		h.infoLog.Printf("failed to decode image JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	exists, err := h.shop.ItemExists(id)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
	if !exists {
		h.responder.NotFound(w)
		return
	}

	objectKey := strings.TrimSpace(request.ObjectKey)
	if !validObjectKey(objectKey, id) {
		h.infoLog.Printf("rejected image key %q for item %d", objectKey, id)
		h.responder.BadRequest(w)
		return
	}

	image := &shopdata.Image{
		ObjectKey: objectKey,
		AltText:   strings.TrimSpace(request.AltText),
		SortOrder: defaultSortOrder,
	}
	if request.SortOrder != nil {
		image.SortOrder = *request.SortOrder
	}

	createdImage, err := h.shop.AddImage(id, image)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	h.infoLog.Printf("CREATE_IMAGE item=%d id=%s", id, createdImage.ID)

	createdImage.URL = h.publicImageURL(createdImage.ObjectKey)

	if err = h.responder.WriteJSON(w, http.StatusCreated, createdImage, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	imageID, ok := h.responder.ReadPositiveIntPath(w, r, "imageId")
	if !ok {
		return
	}

	image, err := h.shop.DeleteImage(id, imageID)
	if h.responder.HandleDataError(w, err) {
		return
	}

	h.infoLog.Printf("DELETE_IMAGE item=%d id=%d", id, imageID)

	h.deleteObjects([]*shopdata.Image{image})

	w.WriteHeader(http.StatusNoContent)
}

// deleteObjects removes uploaded files on a best-effort basis.
func (h *Handler) deleteObjects(images []*shopdata.Image) {
	for _, image := range images {
		if err := h.images.Delete(image.ObjectKey); err != nil {
			h.infoLog.Printf("failed to delete object %s: %v", image.ObjectKey, err)
		}
	}
}

// newObjectKey builds shop/{itemID}/{random}.{ext}. The random name prevents
// collisions and keeps guesses from overwriting existing uploads.
func newObjectKey(itemID int, extension string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%d/%s.%s", objectKeyPrefix, itemID, hex.EncodeToString(random), extension), nil
}

// validObjectKey accepts only keys this server could have minted for the item.
func validObjectKey(objectKey string, itemID int) bool {
	if objectKey == "" || strings.Contains(objectKey, "..") {
		return false
	}

	prefix := fmt.Sprintf("%s/%d/", objectKeyPrefix, itemID)
	if !strings.HasPrefix(objectKey, prefix) {
		return false
	}

	remainder := strings.TrimPrefix(objectKey, prefix)
	if remainder == "" || strings.Contains(remainder, "/") {
		return false
	}

	dot := strings.LastIndex(remainder, ".")
	if dot <= 0 {
		return false
	}

	extension := remainder[dot+1:]
	for _, allowed := range allowedImageTypes {
		if extension == allowed {
			return true
		}
	}

	return false
}
