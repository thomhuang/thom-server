package auth

import (
	"net/http"
	"time"
)

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if !h.authConfigured() {
		h.clientError(w, http.StatusServiceUnavailable)
		return
	}

	var request loginRequest
	if err := h.responder.DecodeJSON(w, r.Body, &request); err != nil {
		h.clientError(w, http.StatusBadRequest)
		return
	}

	throttleKey := loginThrottleKey(r, request.Username)
	if lockedUntil, locked := h.loginLimiter.isLocked(throttleKey); locked {
		h.infoLog.Printf("LOGIN rate-limited %s (until %s)", throttleKey, lockedUntil.Format(time.RFC3339))
		h.setRetryAfter(w, lockedUntil)
		h.clientError(w, http.StatusTooManyRequests)
		return
	}

	if !h.validLoginCredentials(request) {
		h.loginLimiter.recordFailure(throttleKey)
		h.infoLog.Printf("LOGIN failed invalid credentials %s", throttleKey)
		h.clientError(w, http.StatusUnauthorized)
		return
	}

	h.loginLimiter.recordSuccess(throttleKey)

	token, err := h.createAuthToken(request.Username)
	if err != nil {
		h.serverError(w, err)
		return
	}

	h.setAuthCookie(w, token, int(authTokenDuration.Seconds()))

	err = h.writeJSON(w, http.StatusOK, authResponse{
		Authenticated: true,
		Username:      request.Username,
	}, nil)
	if err != nil {
		h.serverError(w, err)
		return
	}

	h.infoLog.Printf("LOGIN success username=%s", request.Username)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	if claims != nil {
		if cookie, err := r.Cookie(h.authCookieName()); err == nil {
			h.tokenDenylist.revoke(cookie.Value, time.Unix(claims.ExpiresAt, 0))
		}
	}

	h.setAuthCookie(w, "", -1)

	err := h.writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false}, nil)
	if err != nil {
		h.serverError(w, err)
		return
	}

	if claims != nil {
		h.infoLog.Printf("LOGOUT username=%s", claims.Username)
	}
}

func (h *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromRequest(r)
	if !ok {
		h.clientError(w, http.StatusUnauthorized)
		return
	}

	err := h.writeJSON(w, http.StatusOK, authResponse{
		Authenticated: true,
		Username:      claims.Username,
	}, nil)
	if err != nil {
		h.serverError(w, err)
		return
	}

	h.infoLog.Printf("GET_CURRENT_USER username=%s", claims.Username)
}
