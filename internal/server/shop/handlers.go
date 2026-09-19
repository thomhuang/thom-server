package shop

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"thom-server/internal/mail"
	authhttp "thom-server/internal/server/auth"
	"thom-server/internal/server/response"
	shopdata "thom-server/internal/shop"
)

const (
	maxTitleLength       = 200
	maxDescriptionLength = 5000
	presignExpiry        = 10 * time.Minute
	defaultSortOrder     = shopdata.DefaultImageSortOrder
	objectKeyPrefix      = "shop"

	// maxMeasurementInches bounds a garment measurement. It is far above any real
	// garment, so it only catches obvious typos and unit mix-ups.
	maxMeasurementInches      = 100.0
	maxMeasurementLabelLength = 60
	maxItemMeasurements       = 20
	maxCategoryLength         = 40
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

// ImageStore is an interface so handler tests can run without R2 credentials.
type ImageStore interface {
	PresignPut(objectKey, contentType string, expires time.Duration) (string, error)
	Delete(objectKey string) error
}

type Handler struct {
	shop              *shopdata.Model
	images            ImageStore
	publicURL         string
	stripe            StripeClient
	stripeSettings    StripeSettings
	mailer            mail.Sender
	siteURL           string
	notificationEmail string
	responder         response.Responder
	infoLog           *log.Logger
}

func New(model *shopdata.Model, images ImageStore, publicURL string, responder response.Responder, infoLog *log.Logger) *Handler {
	if infoLog == nil {
		infoLog = log.New(io.Discard, "", 0)
	}

	return &Handler{
		shop:      model,
		images:    images,
		publicURL: strings.TrimSuffix(strings.TrimSpace(publicURL), "/"),
		responder: responder,
		infoLog:   infoLog,
	}
}

type itemPatch struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	BrandID     *string `json:"brandId"`
	Brand       *string `json:"brand"`
	Category    *string `json:"category"`
	PriceCents  *int    `json:"priceCents"`
	Currency    *string `json:"currency"`
	Stock       *int    `json:"stock"`
	IsPublished *bool   `json:"isPublished"`
	// A pointer so an omitted field leaves the stored measurements untouched; an
	// empty array clears them.
	Measurements *[]*shopdata.Measurement `json:"measurements"`
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

// GetItems lists listings. Anonymous callers see published items only; a valid
// admin session also sees drafts.
func (h *Handler) GetItems(w http.ResponseWriter, r *http.Request) {
	items, err := h.shop.GetItems(authhttp.IsAuthenticated(r))
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	for _, item := range items {
		item.PrimaryImageURL = h.publicImageURL(item.PrimaryImageKey)
	}

	headers := response.PublicCache()
	if authhttp.IsAuthenticated(r) {
		headers = response.NoStore()
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, items, headers); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) GetItemByID(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	item, err := h.shop.GetItemByID(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	if !item.IsPublished && !authhttp.IsAuthenticated(r) {
		h.responder.NotFound(w)
		return
	}

	h.decorateImages(item)

	headers := response.PublicCache()
	if authhttp.IsAuthenticated(r) {
		headers = response.NoStore()
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, item, headers); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) GetBrands(w http.ResponseWriter, r *http.Request) {
	brands, err := h.shop.GetBrands()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, brands, response.PublicCache()); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) CreateBrand(w http.ResponseWriter, r *http.Request) {
	var brand shopdata.Brand
	if err := h.responder.DecodeJSON(w, r.Body, &brand); err != nil {
		h.infoLog.Printf("failed to decode brand JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	normalizeBrand(&brand)
	if err := isValidBrand(&brand); err != nil {
		h.infoLog.Printf("invalid brand: %v", err)
		h.responder.BadRequest(w)
		return
	}

	createdBrand, err := h.shop.UpsertBrand(&brand)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	h.infoLog.Printf("CREATE_BRAND id=%s name=%s", createdBrand.ID, createdBrand.Brand)

	if err = h.responder.WriteJSON(w, http.StatusCreated, createdBrand, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) CreateItem(w http.ResponseWriter, r *http.Request) {
	var request itemPatch
	if err := h.responder.DecodeJSON(w, r.Body, &request); err != nil {
		h.infoLog.Printf("failed to decode shop item JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	item := &shopdata.Item{}
	applyItemPatch(item, &request)

	normalizeItem(item)
	if err := isValidItem(item); err != nil {
		h.infoLog.Printf("invalid shop item: %v", err)
		h.responder.BadRequest(w)
		return
	}

	createdItem, err := h.shop.InsertItem(item)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	h.infoLog.Printf("CREATE_ITEM id=%s title=%s", createdItem.ID, createdItem.Title)

	h.decorateImages(createdItem)

	if err = h.responder.WriteJSON(w, http.StatusCreated, createdItem, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) UpdateItem(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	var patch itemPatch
	if err := h.responder.DecodeJSON(w, r.Body, &patch); err != nil {
		h.infoLog.Printf("failed to decode shop item patch JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	item, err := h.shop.GetItemByID(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	applyItemPatch(item, &patch)
	normalizeItem(item)
	if err := isValidItem(item); err != nil {
		h.infoLog.Printf("invalid shop item: %v", err)
		h.responder.BadRequest(w)
		return
	}

	updatedItem, err := h.shop.UpdateItem(id, item)
	if h.responder.HandleDataError(w, err) {
		return
	}

	h.infoLog.Printf("UPDATE_ITEM id=%d title=%s", id, updatedItem.Title)

	h.decorateImages(updatedItem)

	if err = h.responder.WriteJSON(w, http.StatusOK, updatedItem, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) DeleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	images, err := h.shop.DeleteItem(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	h.infoLog.Printf("DELETE_ITEM id=%d", id)

	// The rows are already gone, so a failed object delete would only strand
	// cheap orphaned files. Log it and still report success.
	h.deleteObjects(images)

	w.WriteHeader(http.StatusNoContent)
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

	if _, err := h.shop.GetItemByID(id); h.responder.HandleDataError(w, err) {
		return
	}

	imageCount, err := h.shop.CountImages(id)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}
	if imageCount >= shopdata.MaxItemImages {
		h.infoLog.Printf("item %d already has the maximum number of images", id)
		h.responder.BadRequest(w)
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

	if _, err := h.shop.GetItemByID(id); h.responder.HandleDataError(w, err) {
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

func (h *Handler) decorateImages(item *shopdata.Item) {
	for _, image := range item.Images {
		image.URL = h.publicImageURL(image.ObjectKey)
	}
}

func (h *Handler) publicImageURL(objectKey string) string {
	if objectKey == "" || h.publicURL == "" {
		return ""
	}

	return h.publicURL + "/" + objectKey
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

func normalizeItem(item *shopdata.Item) {
	item.Title = strings.TrimSpace(item.Title)
	item.Description = strings.TrimSpace(item.Description)
	item.BrandID = strings.TrimSpace(item.BrandID)
	item.Brand = strings.TrimSpace(item.Brand)
	item.Category = strings.TrimSpace(item.Category)
	item.Currency = strings.ToLower(strings.TrimSpace(item.Currency))

	item.Measurements = normalizeMeasurements(item.Measurements)

	if item.Brand == "" {
		item.BrandID = ""
	} else if item.BrandID == "" {
		item.BrandID = shopdata.SlugifyName(item.Brand)
	}

	if item.Currency == "" {
		item.Currency = shopdata.DefaultCurrency
	}
}

// normalizeMeasurements trims labels, rounds values to a tenth, drops blank
// labels, and keeps only the first row for a repeated label so what is stored
// matches what the admin form shows.
func normalizeMeasurements(measurements []*shopdata.Measurement) []*shopdata.Measurement {
	normalized := make([]*shopdata.Measurement, 0, len(measurements))
	seen := make(map[string]bool, len(measurements))

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		label := strings.TrimSpace(measurement.Label)
		if label == "" {
			continue
		}

		key := strings.ToLower(label)
		if seen[key] {
			continue
		}
		seen[key] = true

		normalized = append(normalized, &shopdata.Measurement{
			Label:       label,
			ValueInches: roundToTenth(measurement.ValueInches),
		})
	}

	return normalized
}

func applyItemPatch(item *shopdata.Item, patch *itemPatch) {
	if patch.Title != nil {
		item.Title = *patch.Title
	}
	if patch.Description != nil {
		item.Description = *patch.Description
	}
	if patch.Brand != nil {
		item.Brand = *patch.Brand
		if patch.BrandID == nil {
			item.BrandID = ""
		}
	}
	if patch.BrandID != nil {
		item.BrandID = *patch.BrandID
	}
	if patch.Category != nil {
		item.Category = *patch.Category
	}
	if patch.PriceCents != nil {
		item.PriceCents = *patch.PriceCents
	}
	if patch.Currency != nil {
		item.Currency = *patch.Currency
	}
	if patch.Stock != nil {
		item.Stock = *patch.Stock
	}
	if patch.IsPublished != nil {
		item.IsPublished = *patch.IsPublished
	}
	if patch.Measurements != nil {
		item.Measurements = *patch.Measurements
	}
}

// roundToTenth rounds a measurement to one decimal place. The UI shows a single
// decimal (24.5), so storing more precision would only invite values that
// display differently from how they were entered.
func roundToTenth(value float64) float64 {
	return math.Round(value*10) / 10
}

func isValidItem(item *shopdata.Item) error {
	if item.Title == "" {
		return errors.New("title is required")
	}
	if len([]rune(item.Title)) > maxTitleLength {
		return fmt.Errorf("title must be at most %d characters", maxTitleLength)
	}
	if len([]rune(item.Description)) > maxDescriptionLength {
		return fmt.Errorf("description must be at most %d characters", maxDescriptionLength)
	}
	if item.PriceCents <= 0 {
		return fmt.Errorf("priceCents must be greater than zero, got %d", item.PriceCents)
	}
	if item.Stock < 0 {
		return fmt.Errorf("stock must be non-negative, got %d", item.Stock)
	}
	if !validCurrency(item.Currency) {
		return fmt.Errorf("invalid currency %q", item.Currency)
	}
	if len([]rune(item.Category)) > maxCategoryLength {
		return fmt.Errorf("category must be at most %d characters", maxCategoryLength)
	}
	if err := validMeasurements(item.Measurements); err != nil {
		return err
	}

	return nil
}

// validMeasurements bounds the number of rows and validates each label and
// value. Absence is expressed by omitting a row, so there is no zero sentinel.
func validMeasurements(measurements []*shopdata.Measurement) error {
	if len(measurements) > maxItemMeasurements {
		return fmt.Errorf("at most %d measurements are allowed, got %d", maxItemMeasurements, len(measurements))
	}

	for _, measurement := range measurements {
		if measurement == nil {
			return errors.New("measurement must not be null")
		}

		label := strings.TrimSpace(measurement.Label)
		if label == "" {
			return errors.New("measurement label is required")
		}
		if len([]rune(label)) > maxMeasurementLabelLength {
			return fmt.Errorf("measurement label must be at most %d characters", maxMeasurementLabelLength)
		}
		if err := validMeasurementValue(label, measurement.ValueInches); err != nil {
			return err
		}
	}

	return nil
}

// validMeasurementValue accepts a plausible garment measurement in inches. NaN
// and infinity would otherwise reach the database and break JSON encoding.
func validMeasurementValue(label string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%s must be a finite number", label)
	}
	if value <= 0 {
		return fmt.Errorf("%s must be greater than zero, got %v", label, value)
	}
	if value > maxMeasurementInches {
		return fmt.Errorf("%s must be at most %v inches, got %v", label, maxMeasurementInches, value)
	}

	return nil
}

func validCurrency(currency string) bool {
	if len(currency) != 3 {
		return false
	}

	for _, character := range currency {
		if character < 'a' || character > 'z' {
			return false
		}
	}

	return true
}

func normalizeBrand(brand *shopdata.Brand) {
	brand.ID = strings.TrimSpace(brand.ID)
	brand.Brand = strings.TrimSpace(brand.Brand)

	if brand.ID == "" && brand.Brand != "" {
		brand.ID = shopdata.SlugifyName(brand.Brand)
	}
}

func isValidBrand(brand *shopdata.Brand) error {
	if brand.ID == "" {
		return errors.New("brand id is required")
	}
	if brand.Brand == "" {
		return errors.New("brand name is required")
	}

	return nil
}
