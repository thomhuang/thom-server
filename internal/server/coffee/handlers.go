package coffee

import (
	"io"
	"log"
	"net/http"

	coffeedata "thom-server/internal/coffee"
	"thom-server/internal/server/response"
)

type Handler struct {
	coffee    *coffeedata.Model
	responder response.Responder
	infoLog   *log.Logger
}

func New(model *coffeedata.Model, responder response.Responder, infoLog *log.Logger) *Handler {
	if infoLog == nil {
		infoLog = log.New(io.Discard, "", 0)
	}
	return &Handler{
		coffee:    model,
		responder: responder,
		infoLog:   infoLog,
	}
}

func (h *Handler) GetEntries(w http.ResponseWriter, r *http.Request) {
	entries, err := h.coffee.GetAll()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, entries, response.PublicCacheLong()); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) GetEntryByID(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	entry, err := h.coffee.GetByID(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, entry, response.PublicCacheLong()); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	var entry coffeedata.Entry
	if err := h.responder.DecodeJSON(w, r.Body, &entry); err != nil {
		h.infoLog.Printf("failed to decode coffee entry JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	if !h.normalizeAndValidateEntry(w, &entry) {
		return
	}

	createdEntry, err := h.coffee.Insert(&entry)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	h.infoLog.Printf("CREATE_ENTRY id=%s coffee=%s roaster=%s", createdEntry.ID, createdEntry.CoffeeName, createdEntry.Roaster)

	if err = h.responder.WriteJSON(w, http.StatusCreated, createdEntry, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) UpdateEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	var patch coffeeEntryPatch
	if err := h.responder.DecodeJSON(w, r.Body, &patch); err != nil {
		h.infoLog.Printf("failed to decode coffee entry patch JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}
	if !validCoffeeEntryPatch(&patch) {
		h.responder.BadRequest(w)
		return
	}

	entry, err := h.coffee.GetByID(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	applyCoffeeEntryPatch(entry, &patch)
	if !h.normalizeAndValidateEntry(w, entry) {
		return
	}

	updatedEntry, err := h.coffee.Update(id, entry)
	if h.responder.HandleDataError(w, err) {
		return
	}

	h.infoLog.Printf("UPDATE_ENTRY id=%d coffee=%s roaster=%s", id, updatedEntry.CoffeeName, updatedEntry.Roaster)

	if err = h.responder.WriteJSON(w, http.StatusOK, updatedEntry, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	err := h.coffee.Delete(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	h.infoLog.Printf("DELETE_ENTRY id=%d", id)

	w.WriteHeader(http.StatusNoContent)
}

// normalizeAndValidateEntry trims an entry, fills derived lookup ids, and
// rejects one that is missing required fields. It writes the error response
// itself and reports whether the entry may be saved.
func (h *Handler) normalizeAndValidateEntry(w http.ResponseWriter, entry *coffeedata.Entry) bool {
	normalizeCoffeeEntry(entry)
	if err := isValidCoffeeEntry(entry); err != nil {
		h.infoLog.Printf("invalid coffee entry: %v", err)
		h.responder.BadRequest(w)
		return false
	}

	return true
}
