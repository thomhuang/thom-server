package auth

import (
	"net/http"
	"time"

	"thom-server/internal/server/response"
)

const (
	authCookieName       = "thom_auth"
	authTokenDuration    = 12 * time.Hour
	loginFailureLimit    = 5
	loginFailureWindow   = 15 * time.Minute
	loginLockoutDuration = 15 * time.Minute
)

type authContextKey string

const authClaimsContextKey authContextKey = "authClaims"

type authClaims struct {
	Subject   string `json:"sub"`
	Username  string `json:"username"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

type authResponse struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Config struct {
	AdminUsername     string
	AdminPasswordHash string
	JWTSecret         string
	SecureCookies     bool
}

type Handler struct {
	config        Config
	responder     response.Responder
	loginLimiter  *loginRateLimiter
	allowedOrigin func(string) bool
}

func New(config Config, responder response.Responder, allowedOrigin func(string) bool) *Handler {
	return &Handler{
		config:        config,
		responder:     responder,
		loginLimiter:  newLoginRateLimiter(),
		allowedOrigin: allowedOrigin,
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}, headers http.Header) error {
	return h.responder.WriteJSON(w, status, data, headers)
}

func (h *Handler) serverError(w http.ResponseWriter, err error) {
	h.responder.ServerError(w, err)
}

func (h *Handler) clientError(w http.ResponseWriter, status int) {
	h.responder.ClientError(w, status)
}

func (h *Handler) isAllowedOrigin(origin string) bool {
	return h.allowedOrigin != nil && h.allowedOrigin(origin)
}
