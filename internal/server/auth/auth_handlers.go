package auth

import (
	"encoding/json"
	"net/http"
	"time"
)

func (app *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		app.clientError(w, http.StatusMethodNotAllowed)
		return
	}

	if !app.authConfigured() {
		app.clientError(w, http.StatusServiceUnavailable)
		return
	}

	var request loginRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	throttleKey := loginThrottleKey(r, request.Username)
	if app.loginLimiter != nil {
		if lockedUntil, locked := app.loginLimiter.isLocked(throttleKey); locked {
			app.infoLog.Printf("LOGIN rate-limited %s (until %s)", throttleKey, lockedUntil.Format(time.RFC3339))
			app.setRetryAfter(w, lockedUntil)
			app.clientError(w, http.StatusTooManyRequests)
			return
		}
	}

	if !app.validLoginCredentials(request) {
		if app.loginLimiter != nil {
			app.loginLimiter.recordFailure(throttleKey)
		}
		app.infoLog.Printf("LOGIN failed invalid credentials %s", throttleKey)
		app.clientError(w, http.StatusUnauthorized)
		return
	}

	if app.loginLimiter != nil {
		app.loginLimiter.recordSuccess(throttleKey)
	}

	token, err := app.createAuthToken(request.Username)
	if err != nil {
		app.serverError(w, err)
		return
	}

	app.setAuthCookie(w, token, int(authTokenDuration.Seconds()))

	err = app.writeJSON(w, http.StatusOK, authResponse{
		Authenticated: true,
		Username:      request.Username,
	}, nil)
	if err != nil {
		app.serverError(w, err)
		return
	}

	app.infoLog.Printf("LOGIN success username=%s", request.Username)
}

func (app *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		app.clientError(w, http.StatusMethodNotAllowed)
		return
	}

	app.setAuthCookie(w, "", -1)

	err := app.writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false}, nil)
	if err != nil {
		app.serverError(w, err)
		return
	}

	if claims, ok := claimsFromRequest(r); ok {
		app.infoLog.Printf("LOGOUT username=%s", claims.Username)
	}
}

func (app *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		app.clientError(w, http.StatusMethodNotAllowed)
		return
	}

	claims, ok := claimsFromRequest(r)
	if !ok {
		app.clientError(w, http.StatusUnauthorized)
		return
	}

	err := app.writeJSON(w, http.StatusOK, authResponse{
		Authenticated: true,
		Username:      claims.Username,
	}, nil)
	if err != nil {
		app.serverError(w, err)
		return
	}

	app.infoLog.Printf("GET_CURRENT_USER username=%s", claims.Username)
}
