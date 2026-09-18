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

func (h *Handler) authConfigured() bool {
	return h.config.AdminUsername != "" &&
		h.config.AdminPasswordHash != "" &&
		isValidPasswordHash(h.config.AdminPasswordHash) &&
		h.config.JWTSecret != ""
}

func isValidPasswordHash(hash string) bool {
	_, err := bcrypt.Cost([]byte(hash))
	return err == nil
}

func (h *Handler) validLoginCredentials(request loginRequest) bool {
	usernameMatches := subtle.ConstantTimeCompare(
		[]byte(request.Username),
		[]byte(h.config.AdminUsername),
	) == 1
	passwordMatches := bcrypt.CompareHashAndPassword(
		[]byte(h.config.AdminPasswordHash),
		[]byte(request.Password),
	) == nil

	return usernameMatches && passwordMatches
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
