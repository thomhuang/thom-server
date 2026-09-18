package auth

import "time"

// memoryState keeps auth state in-process. It is the default for tests and for
// runs without a database; deployments swap in authstate.Model so lockouts and
// revocations survive restarts and are shared across instances.
type memoryState struct {
	limiter  *loginRateLimiter
	denylist *tokenDenylist
}

func newMemoryState() *memoryState {
	return &memoryState{
		limiter:  newLoginRateLimiter(),
		denylist: newTokenDenylist(),
	}
}

func (m *memoryState) LoginLockedUntil(key string) (time.Time, bool, error) {
	until, locked := m.limiter.isLocked(key)

	return until, locked, nil
}

func (m *memoryState) RecordLoginFailure(key string) error {
	m.limiter.recordFailure(key)
	return nil
}

func (m *memoryState) ClearLoginFailures(key string) error {
	m.limiter.recordSuccess(key)
	return nil
}

func (m *memoryState) RevokeToken(token string, expiresAt time.Time) error {
	m.denylist.revoke(token, expiresAt)
	return nil
}

func (m *memoryState) IsTokenRevoked(token string) (bool, error) {
	return m.denylist.isRevoked(token), nil
}
