package auth

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func (app *Handler) authConfigured() bool {
	return app.config.AdminUsername != "" &&
		app.config.AdminPasswordHash != "" &&
		isValidPasswordHash(app.config.AdminPasswordHash) &&
		app.config.JWTSecret != ""
}

func isValidPasswordHash(hash string) bool {
	_, err := bcrypt.Cost([]byte(hash))
	return err == nil
}

func (app *Handler) validLoginCredentials(request loginRequest) bool {
	usernameMatches := subtle.ConstantTimeCompare(
		[]byte(request.Username),
		[]byte(app.config.AdminUsername),
	) == 1
	passwordMatches := bcrypt.CompareHashAndPassword(
		[]byte(app.config.AdminPasswordHash),
		[]byte(request.Password),
	) == nil

	return usernameMatches && passwordMatches
}

func loginThrottleKey(r *http.Request, username string) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		host = r.RemoteAddr
	}

	return strings.ToLower(strings.TrimSpace(host)) + "|" + strings.ToLower(strings.TrimSpace(username))
}

func (app *Handler) setRetryAfter(w http.ResponseWriter, lockedUntil time.Time) {
	retryAfter := int(time.Until(lockedUntil).Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}

	w.Header().Set("Retry-After", fmt.Sprint(retryAfter))
}
