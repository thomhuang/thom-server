package auth

import (
	"io"
	"log"
	"testing"

	"thom-server/internal/server/response"
)

const testAdminPasswordHash = "$2a$04$NcZBWqKwXIayYJceXw24N.PyNMu7vrnvSgWwROy1o9AMCIa3Rdl5i"

func newTestHandler(t *testing.T) *Handler {
	t.Helper()

	return New(
		Config{
			AdminUsername:     "admin",
			AdminPasswordHash: testAdminPasswordHash,
			JWTSecret:         "test-secret",
		},
		response.Responder{ErrorLog: log.New(io.Discard, "", 0)},
		func(origin string) bool {
			return origin == "http://localhost:3000" || origin == "https://app.example.com"
		},
	)
}
