package posts

import (
	"net/http"
	"strconv"

	postdata "thom-server/internal/posts"
	"thom-server/internal/server/response"
)

type Handler struct {
	posts     *postdata.Model
	responder response.Responder
}

func New(model *postdata.Model, responder response.Responder) *Handler {
	return &Handler{
		posts:     model,
		responder: responder,
	}
}

func (h *Handler) GetCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.posts.GetCategories()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, categories, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) GetByCategory(w http.ResponseWriter, r *http.Request) {
	categoryID, ok := h.readPositiveIntQuery(w, r, "category")
	if !ok {
		return
	}

	posts, err := h.posts.GetPostsByCategory(categoryID)
	if h.responder.HandleDataError(w, err) {
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, posts, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) GetContentByID(w http.ResponseWriter, r *http.Request) {
	postID, ok := h.readPositiveIntQuery(w, r, "post")
	if !ok {
		return
	}

	post, err := h.posts.GetPostWithContentByID(postID)
	if h.responder.HandleDataError(w, err) {
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, post, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) GetContentByPathName(w http.ResponseWriter, r *http.Request) {
	pathName := r.URL.Query().Get("pathName")
	if pathName == "" {
		h.responder.BadRequest(w)
		return
	}

	post, err := h.posts.GetPostWithContentByPathName(pathName)
	if h.responder.HandleDataError(w, err) {
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, post, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) readPositiveIntQuery(w http.ResponseWriter, r *http.Request, key string) (int, bool) {
	id, err := strconv.Atoi(r.URL.Query().Get(key))
	if err != nil || id < 1 {
		h.responder.BadRequest(w)
		return 0, false
	}

	return id, true
}
