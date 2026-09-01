package auth

import (
	"context"
	"net/http"
)

func (h *Handler) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(h.authCookieName())
		if err != nil {
			h.infoLog.Printf("AUTH missing cookie %s %s", r.Method, r.URL.Path)
			h.clientError(w, http.StatusUnauthorized)
			return
		}

		claims, err := h.verifyAuthToken(cookie.Value)
		if err != nil {
			h.infoLog.Printf("AUTH invalid token %s %s", r.Method, r.URL.Path)
			h.clientError(w, http.StatusUnauthorized)
			return
		}

		if isMutatingMethod(r.Method) && !h.isAllowedOrigin(r.Header.Get("Origin")) {
			h.infoLog.Printf("AUTH origin forbidden %s %s origin=%q", r.Method, r.URL.Path, r.Header.Get("Origin"))
			h.clientError(w, http.StatusForbidden)
			return
		}

		w.Header().Set("Cache-Control", "no-store")

		ctx := context.WithValue(r.Context(), authClaimsContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func isMutatingMethod(method string) bool {
	return method != http.MethodGet &&
		method != http.MethodHead &&
		method != http.MethodOptions
}

func claimsFromRequest(r *http.Request) (*authClaims, bool) {
	claims, ok := r.Context().Value(authClaimsContextKey).(*authClaims)
	return claims, ok
}
