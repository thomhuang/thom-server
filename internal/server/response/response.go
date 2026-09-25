package response

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"
)

const maxJSONBodySize = 1 << 20

// publicCacheMaxAge bounds how long a shared cache may serve a public response
// whose price or stock can change at any time (shop listings).
const publicCacheMaxAge = 30 * time.Second

// publicCacheLongMaxAge bounds how long a shared cache may serve a public
// response that changes only when an admin edits it (coffee journal, blog
// posts, brands, categories), so it can survive the gaps between visitors.
const publicCacheLongMaxAge = 5 * time.Minute

// PublicCache returns the Cache-Control header for a shared-cacheable public
// GET response.
func PublicCache() http.Header {
	return publicCache(publicCacheMaxAge)
}

// PublicCacheLong returns the Cache-Control header for a public GET response
// that changes only on admin edits and may be shared longer.
func PublicCacheLong() http.Header {
	return publicCache(publicCacheLongMaxAge)
}

func publicCache(maxAge time.Duration) http.Header {
	return http.Header{
		"Cache-Control": {"public, max-age=" + strconv.Itoa(int(maxAge/time.Second))},
	}
}

// NoStore returns the Cache-Control header for a response that must never be
// cached.
func NoStore() http.Header {
	return http.Header{
		"Cache-Control": {"no-store"},
	}
}

type Responder struct {
	ErrorLog *log.Logger
}

func (r Responder) DecodeJSON(w http.ResponseWriter, body io.ReadCloser, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, body, maxJSONBodySize))
	return decoder.Decode(dst)
}

func (r Responder) WriteJSON(w http.ResponseWriter, status int, data any, headers http.Header) error {
	js, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}
	js = append(js, '\n')
	for key, value := range headers {
		w.Header()[key] = value
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(js)
	if err != nil {
		return err
	}

	return nil
}

func (r Responder) ServerError(w http.ResponseWriter, err error) {
	trace := fmt.Sprintf("%s\n%s", err.Error(), debug.Stack())
	if r.ErrorLog != nil {
		r.ErrorLog.Println(trace)
	}

	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (r Responder) ClientError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

// ReadPositiveIntPath parses a positive integer path parameter, writing a 400
// and returning false when it is missing or not a positive integer.
func (r Responder) ReadPositiveIntPath(w http.ResponseWriter, req *http.Request, key string) (int, bool) {
	value, err := strconv.Atoi(req.PathValue(key))
	if err != nil || value < 1 {
		r.BadRequest(w)
		return 0, false
	}

	return value, true
}

func (r Responder) BadRequest(w http.ResponseWriter) {
	r.ClientError(w, http.StatusBadRequest)
}

func (r Responder) NotFound(w http.ResponseWriter) {
	r.ClientError(w, http.StatusNotFound)
}
