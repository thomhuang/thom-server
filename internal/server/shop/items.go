package shop

import (
	"net/http"

	authhttp "thom-server/internal/server/auth"
	"thom-server/internal/server/response"
	shopdata "thom-server/internal/shop"
)

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

	if err = h.responder.WriteJSON(w, http.StatusOK, items, cacheHeaders(r)); err != nil {
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

	if err = h.responder.WriteJSON(w, http.StatusOK, item, cacheHeaders(r)); err != nil {
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

	if !h.normalizeAndValidateItem(w, item) {
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
	if !h.normalizeAndValidateItem(w, item) {
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

// normalizeAndValidateItem trims a listing, fills derived brand defaults, and
// rejects one that is missing required fields. It writes the error response
// itself and reports whether the item may be saved.
func (h *Handler) normalizeAndValidateItem(w http.ResponseWriter, item *shopdata.Item) bool {
	normalizeItem(item)
	if err := isValidItem(item); err != nil {
		h.infoLog.Printf("invalid shop item: %v", err)
		h.responder.BadRequest(w)
		return false
	}

	return true
}

// cacheHeaders lets a public caller share a response, while an authenticated
// admin response (which may include drafts) must never be cached.
func cacheHeaders(r *http.Request) http.Header {
	if authhttp.IsAuthenticated(r) {
		return response.NoStore()
	}

	return response.PublicCache()
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
