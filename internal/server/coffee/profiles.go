package coffee

import (
	"fmt"
	"net/http"
	"strings"

	coffeedata "thom-server/internal/coffee"
	"thom-server/internal/server/response"
)

func (h *Handler) GetProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.coffee.GetProfiles()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, profiles, response.PublicCacheLong()); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) CreateProfile(w http.ResponseWriter, r *http.Request) {
	var profile coffeedata.Profile
	if err := h.responder.DecodeJSON(w, r.Body, &profile); err != nil {
		h.infoLog.Printf("failed to decode profile JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	normalizeCoffeeProfile(&profile)
	if err := isValidCoffeeProfile(&profile); err != nil {
		h.infoLog.Printf("invalid profile: %v", err)
		h.responder.BadRequest(w)
		return
	}

	if existing, err := h.coffee.GetProfileByName(profile.Name); err == nil {
		h.infoLog.Printf("duplicate profile name %q (existing id %s)", profile.Name, existing.ID)
		h.responder.ClientError(w, http.StatusConflict)
		return
	}

	createdProfile, err := h.coffee.InsertProfile(&profile)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	h.infoLog.Printf("CREATE_PROFILE id=%s name=%s", createdProfile.ID, createdProfile.Name)

	if err = h.responder.WriteJSON(w, http.StatusCreated, createdProfile, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.responder.NotFound(w)
		return
	}

	var profile coffeedata.Profile
	if err := h.responder.DecodeJSON(w, r.Body, &profile); err != nil {
		h.infoLog.Printf("failed to decode profile JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	normalizeCoffeeProfile(&profile)
	if err := isValidCoffeeProfile(&profile); err != nil {
		h.infoLog.Printf("invalid profile: %v", err)
		h.responder.BadRequest(w)
		return
	}

	if existing, err := h.coffee.GetProfileByName(profile.Name); err == nil && existing.ID != id {
		h.infoLog.Printf("duplicate profile name %q (existing id %s)", profile.Name, existing.ID)
		h.responder.ClientError(w, http.StatusConflict)
		return
	}

	updatedProfile, err := h.coffee.UpdateProfile(id, &profile)
	if h.responder.HandleDataError(w, err) {
		return
	}

	h.infoLog.Printf("UPDATE_PROFILE id=%s name=%s", id, updatedProfile.Name)

	if err = h.responder.WriteJSON(w, http.StatusOK, updatedProfile, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.responder.NotFound(w)
		return
	}

	if err := h.coffee.DeleteProfile(id); h.responder.HandleDataError(w, err) {
		return
	}

	h.infoLog.Printf("DELETE_PROFILE id=%s", id)

	w.WriteHeader(http.StatusNoContent)
}

func normalizeCoffeeProfile(profile *coffeedata.Profile) {
	profile.ID = strings.TrimSpace(profile.ID)
	profile.Name = strings.TrimSpace(profile.Name)
	profile.BrewMethod = strings.TrimSpace(profile.BrewMethod)
	profile.Notes = strings.TrimSpace(profile.Notes)

	if profile.ID == "" && profile.Name != "" {
		profile.ID = coffeedata.SlugifyName(profile.Name)
	}
}

func isValidCoffeeProfile(profile *coffeedata.Profile) error {
	if profile.ID == "" {
		return fmt.Errorf("profile id is required")
	}
	if profile.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	if profile.BrewMethod == "" {
		return fmt.Errorf("profile brew method is required")
	}
	if profile.Notes == "" {
		return fmt.Errorf("profile notes are required")
	}
	return nil
}
