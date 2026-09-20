package coffee

import (
	"fmt"
	"net/http"
	"strings"

	coffeedata "thom-server/internal/coffee"
	"thom-server/internal/server/response"
)

func (h *Handler) GetGrinders(w http.ResponseWriter, r *http.Request) {
	grinders, err := h.coffee.GetGrinders()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, grinders, response.PublicCache()); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) CreateGrinder(w http.ResponseWriter, r *http.Request) {
	var grinder coffeedata.Grinder
	if err := h.responder.DecodeJSON(w, r.Body, &grinder); err != nil {
		h.infoLog.Printf("failed to decode grinder JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	normalizeCoffeeGrinder(&grinder)
	if err := isValidCoffeeGrinder(&grinder); err != nil {
		h.infoLog.Printf("invalid grinder: %v", err)
		h.responder.BadRequest(w)
		return
	}

	createdGrinder, err := h.coffee.UpsertGrinder(&grinder)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	h.infoLog.Printf("CREATE_GRINDER id=%s name=%s", createdGrinder.ID, createdGrinder.Grinder)

	if err = h.responder.WriteJSON(w, http.StatusCreated, createdGrinder, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func normalizeCoffeeGrinder(grinder *coffeedata.Grinder) {
	grinder.ID = strings.TrimSpace(grinder.ID)
	grinder.Grinder = strings.TrimSpace(grinder.Grinder)

	if grinder.ID == "" && grinder.Grinder != "" {
		grinder.ID = coffeedata.SlugifyName(grinder.Grinder)
	}
}

func isValidCoffeeGrinder(grinder *coffeedata.Grinder) error {
	if grinder.ID == "" {
		return fmt.Errorf("grinder id is required")
	}
	if grinder.Grinder == "" {
		return fmt.Errorf("grinder name is required")
	}
	return nil
}
