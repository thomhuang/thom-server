package coffee

import (
	"fmt"
	"net/http"
	"strings"

	coffeedata "thom-server/internal/coffee"
	"thom-server/internal/server/response"
)

func (h *Handler) GetRoasters(w http.ResponseWriter, r *http.Request) {
	roasters, err := h.coffee.GetRoasters()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, roasters, response.PublicCache()); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) CreateRoaster(w http.ResponseWriter, r *http.Request) {
	var roaster coffeedata.Roaster
	if err := h.responder.DecodeJSON(w, r.Body, &roaster); err != nil {
		h.infoLog.Printf("failed to decode roaster JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	normalizeCoffeeRoaster(&roaster)
	if err := isValidCoffeeRoaster(&roaster); err != nil {
		h.infoLog.Printf("invalid roaster: %v", err)
		h.responder.BadRequest(w)
		return
	}

	createdRoaster, err := h.coffee.UpsertRoaster(&roaster)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	h.infoLog.Printf("CREATE_ROASTER id=%s name=%s", createdRoaster.ID, createdRoaster.Roaster)

	if err = h.responder.WriteJSON(w, http.StatusCreated, createdRoaster, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func normalizeCoffeeRoaster(roaster *coffeedata.Roaster) {
	roaster.ID = strings.TrimSpace(roaster.ID)
	roaster.Roaster = strings.TrimSpace(roaster.Roaster)

	if roaster.ID == "" && roaster.Roaster != "" {
		roaster.ID = coffeedata.SlugifyName(roaster.Roaster)
	}
}

func isValidCoffeeRoaster(roaster *coffeedata.Roaster) error {
	if roaster.ID == "" {
		return fmt.Errorf("roaster id is required")
	}
	if roaster.Roaster == "" {
		return fmt.Errorf("roaster name is required")
	}
	return nil
}
