package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// tokenDenylist revokes individual tokens (e.g. on logout) until they expire.
// It is in-memory: revocations reset when the process restarts.
type tokenDenylist struct {
	mu      sync.Mutex
	revoked map[string]time.Time
	now     func() time.Time
}

func newTokenDenylist() *tokenDenylist {
	return &tokenDenylist{
		revoked: make(map[string]time.Time),
		now:     time.Now,
	}
}

func (d *tokenDenylist) revoke(token string, expiresAt time.Time) {
	sum := sha256.Sum256([]byte(token))
	key := hex.EncodeToString(sum[:])

	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.currentTime()
	for k, exp := range d.revoked {
		if !now.Before(exp) {
			delete(d.revoked, k)
		}
	}

	if now.Before(expiresAt) {
		d.revoked[key] = expiresAt
	}
}

func (d *tokenDenylist) isRevoked(token string) bool {
	sum := sha256.Sum256([]byte(token))
	key := hex.EncodeToString(sum[:])

	d.mu.Lock()
	defer d.mu.Unlock()

	expiresAt, ok := d.revoked[key]
	if !ok {
		return false
	}
	if !d.currentTime().Before(expiresAt) {
		delete(d.revoked, key)
		return false
	}

	return true
}

func (d *tokenDenylist) currentTime() time.Time {
	if d.now != nil {
		return d.now()
	}

	return time.Now()
}
