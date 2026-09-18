package server

import (
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (app *App) commonMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The Allow-Origin response depends on the request Origin, so Vary must
		// be set even when the origin is not allowlisted.
		w.Header().Add("Vary", "Origin")

		if app.isAllowedOrigin(r.Header.Get("Origin")) {
			w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		// Static assets get these from _headers; Worker/API responses do not, so
		// set the one that matters for a JSON API here.
		w.Header().Set("X-Content-Type-Options", "nosniff")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r)

		app.infoLog.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.statusCode, time.Since(start))
	})
}

func (app *App) isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}

	for _, allowedOrigin := range app.config.ClientOrigins {
		if origin == allowedOrigin {
			return true
		}
	}

	return false
}

// requireAllowedOrigin rejects state-changing browser requests whose Origin is
// not in the allowlist. Authenticated mutating routes already check this inside
// RequireAuth; this wraps the public ones (login, checkout). The Stripe webhook
// is excluded because it authenticates with a signature, not an Origin header.
func (app *App) requireAllowedOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isMutatingRequest(r.Method) && !app.isAllowedOrigin(r.Header.Get("Origin")) {
			app.infoLog.Printf("origin forbidden %s %s origin=%q", r.Method, r.URL.Path, r.Header.Get("Origin"))
			app.responder.ClientError(w, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	}
}

func isMutatingRequest(method string) bool {
	return method != http.MethodGet &&
		method != http.MethodHead &&
		method != http.MethodOptions
}
