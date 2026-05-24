package response

import (
	"errors"
	"net/http"

	"thom-server/internal/data"
)

func (r Responder) HandleDataError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, data.ErrNoRecord) {
		r.NotFound(w)
		return true
	}

	r.ServerError(w, err)
	return true
}
