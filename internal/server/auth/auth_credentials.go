package auth

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"thom-server/internal/clientip"
)

// authConfigIssue names the setting that leaves login unavailable, or returns
// an empty string when auth is configured. It never includes a secret value, so
// it is safe to log and lets an operator fix a 503 from the logs alone.
func (h *Handler) authConfigIssue() string {
	switch {
	case h.config.AdminUsername == "":
		return "ADMIN_USERNAME is not set"
	case h.config.AdminPasswordHash == "":
		return "ADMIN_PASSWORD_HASH is not set"
	case !isValidPasswordHash(h.config.AdminPasswordHash):
		return "ADMIN_PASSWORD_HASH is not a valid bcrypt hash"
	case h.config.JWTSecret == "":
		return "JWT_SECRET is not set"
	}

	return ""
}

func isValidPasswordHash(hash string) bool {
	_, err := bcrypt.Cost([]byte(hash))
	return err == nil
}

// credentialMatch reports which half of the login matched. Logging the two
// booleans distinguishes a wrong username from a wrong password without ever
// recording the submitted password.
func (h *Handler) credentialMatch(request loginRequest) (usernameMatch, passwordMatch bool) {
	usernameMatch = subtle.ConstantTimeCompare(
		[]byte(request.Username),
		[]byte(h.config.AdminUsername),
	) == 1
	passwordMatch = bcrypt.CompareHashAndPassword(
		[]byte(h.config.AdminPasswordHash),
		[]byte(request.Password),
	) == nil

	return usernameMatch, passwordMatch
}

func loginThrottleKey(r *http.Request, username string) string {
	host := clientip.FromRequest(r)

	return strings.ToLower(host) + "|" + strings.ToLower(strings.TrimSpace(username))
}

func (h *Handler) setRetryAfter(w http.ResponseWriter, lockedUntil time.Time) {
	retryAfter := int(time.Until(lockedUntil).Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}

	w.Header().Set("Retry-After", fmt.Sprint(retryAfter))
}
