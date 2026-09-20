package shop

import (
	"net/http"

	"thom-server/internal/server/response"
	shopdata "thom-server/internal/shop"
)

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
