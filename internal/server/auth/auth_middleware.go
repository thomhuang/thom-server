package auth

import (
	"context"
	"net/http"
)

func (app *Handler) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(authCookieName)
		if err != nil {
			app.clientError(w, http.StatusUnauthorized)
			return
		}

		claims, err := app.verifyAuthToken(cookie.Value)
		if err != nil {
			app.clientError(w, http.StatusUnauthorized)
			return
		}

		if isMutatingMethod(r.Method) && !app.isAllowedOrigin(r.Header.Get("Origin")) {
			app.clientError(w, http.StatusForbidden)
			return
		}

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
