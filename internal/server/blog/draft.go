package blog

import (
	"net/http"

	blogdata "thom-server/internal/blog"
	"thom-server/internal/server/response"
)

// GetDraft returns the admin's in-progress "new post" form, empty when none has
// been autosaved.
func (h *Handler) GetDraft(w http.ResponseWriter, r *http.Request) {
	draft, err := h.blog.GetDraft()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, draft, response.NoStore()); err != nil {
		h.responder.ServerError(w, err)
	}
}

// PutDraft autosaves the admin's in-progress form. Unlike a real post, only
// length caps apply — a scratch buffer has no required fields.
func (h *Handler) PutDraft(w http.ResponseWriter, r *http.Request) {
	var draft blogdata.Draft
	if err := h.responder.DecodeJSON(w, r.Body, &draft); err != nil {
		h.infoLog.Printf("failed to decode blog draft JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	if err := isValidDraft(&draft); err != nil {
		h.infoLog.Printf("invalid blog draft: %v", err)
		h.responder.BadRequest(w)
		return
	}

	if err := h.blog.SaveDraft(&draft); err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err := h.responder.WriteJSON(w, http.StatusOK, draft, response.NoStore()); err != nil {
		h.responder.ServerError(w, err)
	}
}

// DeleteDraft clears the autosaved draft after the post is saved.
func (h *Handler) DeleteDraft(w http.ResponseWriter, r *http.Request) {
	if err := h.blog.DeleteDraft(); err != nil {
		h.responder.ServerError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
