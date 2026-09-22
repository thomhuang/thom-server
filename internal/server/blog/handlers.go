package blog

import (
	"io"
	"log"
	"net/http"
	"strings"

	blogdata "thom-server/internal/blog"
	authhttp "thom-server/internal/server/auth"
	"thom-server/internal/server/response"
)

type Handler struct {
	blog      *blogdata.Model
	responder response.Responder
	infoLog   *log.Logger
}

func New(model *blogdata.Model, responder response.Responder, infoLog *log.Logger) *Handler {
	if infoLog == nil {
		infoLog = log.New(io.Discard, "", 0)
	}
	return &Handler{
		blog:      model,
		responder: responder,
		infoLog:   infoLog,
	}
}

// GetPosts lists posts, optionally restricted to one category via
// `?category=<id>`. Anonymous callers see published posts only; a valid admin
// session also sees drafts.
func (h *Handler) GetPosts(w http.ResponseWriter, r *http.Request) {
	categoryID := strings.TrimSpace(r.URL.Query().Get("category"))

	posts, err := h.blog.GetAll(!authhttp.IsAuthenticated(r), categoryID)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, posts, cacheHeaders(r)); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

// GetCategories lists the post categories for the public filter and the admin
// form's suggestions.
func (h *Handler) GetCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.blog.GetCategories()
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, categories, response.PublicCache()); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) GetPostByID(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	post, err := h.blog.GetByID(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	if !post.Published && !authhttp.IsAuthenticated(r) {
		h.responder.NotFound(w)
		return
	}

	if err = h.responder.WriteJSON(w, http.StatusOK, post, cacheHeaders(r)); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) CreatePost(w http.ResponseWriter, r *http.Request) {
	var request postPatch
	if err := h.responder.DecodeJSON(w, r.Body, &request); err != nil {
		h.infoLog.Printf("failed to decode blog post JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	post := &blogdata.Post{}
	applyPostPatch(post, &request)

	if !h.normalizeAndValidatePost(w, post) {
		return
	}

	createdPost, err := h.blog.Insert(post)
	if err != nil {
		h.responder.ServerError(w, err)
		return
	}

	h.infoLog.Printf("CREATE_POST id=%s title=%s", createdPost.ID, createdPost.Title)

	if err = h.responder.WriteJSON(w, http.StatusCreated, createdPost, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) UpdatePost(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	var patch postPatch
	if err := h.responder.DecodeJSON(w, r.Body, &patch); err != nil {
		h.infoLog.Printf("failed to decode blog post patch JSON: %v", err)
		h.responder.BadRequest(w)
		return
	}

	post, err := h.blog.GetByID(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	applyPostPatch(post, &patch)
	if !h.normalizeAndValidatePost(w, post) {
		return
	}

	updatedPost, err := h.blog.Update(id, post)
	if h.responder.HandleDataError(w, err) {
		return
	}

	h.infoLog.Printf("UPDATE_POST id=%d title=%s", id, updatedPost.Title)

	if err = h.responder.WriteJSON(w, http.StatusOK, updatedPost, nil); err != nil {
		h.responder.ServerError(w, err)
		return
	}
}

func (h *Handler) DeletePost(w http.ResponseWriter, r *http.Request) {
	id, ok := h.responder.ReadPositiveIntPath(w, r, "id")
	if !ok {
		return
	}

	err := h.blog.Delete(id)
	if h.responder.HandleDataError(w, err) {
		return
	}

	h.infoLog.Printf("DELETE_POST id=%d", id)

	w.WriteHeader(http.StatusNoContent)
}

// normalizeAndValidatePost trims a post and rejects one missing required
// fields. It writes the error response itself and reports whether the post may
// be saved.
func (h *Handler) normalizeAndValidatePost(w http.ResponseWriter, post *blogdata.Post) bool {
	normalizePost(post)
	if err := isValidPost(post); err != nil {
		h.infoLog.Printf("invalid blog post: %v", err)
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
