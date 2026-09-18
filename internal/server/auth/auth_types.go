package auth

import (
	"io"
	"log"
	"net/http"
	"time"

	"thom-server/internal/server/response"
)

const (
	defaultAuthCookieName = "thom_auth"
	authTokenDuration     = 12 * time.Hour
)

// StateStore persists the auth decisions that must outlive a process: login
// lockouts and revoked tokens. authstate.Model implements it against the
// application database; memoryState is the in-process default.
type StateStore interface {
	LoginLockedUntil(key string) (time.Time, bool, error)
	RecordLoginFailure(key string) error
	ClearLoginFailures(key string) error
	RevokeToken(token string, expiresAt time.Time) error
	IsTokenRevoked(token string) (bool, error)
}

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
	state         StateStore
	allowedOrigin func(string) bool
	infoLog       *log.Logger
}

func New(config Config, responder response.Responder, allowedOrigin func(string) bool, infoLog *log.Logger) *Handler {
	if infoLog == nil {
		infoLog = log.New(io.Discard, "", 0)
	}
	return &Handler{
		config:        config,
		responder:     responder,
		state:         newMemoryState(),
		allowedOrigin: allowedOrigin,
		infoLog:       infoLog,
	}
}

// WithState swaps in a persistent store. A nil store leaves the in-memory
// default, which is what tests and database-less local runs use.
func (h *Handler) WithState(state StateStore) *Handler {
	if state != nil {
		h.state = state
	}

	return h
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data any, headers http.Header) error {
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
