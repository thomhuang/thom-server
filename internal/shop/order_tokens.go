package shop

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// NewOrderViewToken returns a random bearer token for the buyer's order link and
// the hash stored for it. Only the hash is persisted; the raw token is emailed
// and never written to the database.
func NewOrderViewToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}

	token := base64.RawURLEncoding.EncodeToString(raw)

	return token, HashOrderViewToken(token), nil
}

// HashOrderViewToken hashes a view token for storage and lookup. Tokens are
// high-entropy random values, so a plain SHA-256 is enough; there is no
// dictionary to slow down.
func HashOrderViewToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
