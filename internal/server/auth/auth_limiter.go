package auth

import (
	"sync"
	"time"
)

type loginAttempt struct {
	failures     int
	firstFailure time.Time
	lockedUntil  time.Time
}

type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
	now      func() time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		attempts: make(map[string]loginAttempt),
		now:      time.Now,
	}
}

func (l *loginRateLimiter) isLocked(key string) (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.currentTime()
	attempt := l.attempts[key]
	if attempt.lockedUntil.IsZero() {
		return time.Time{}, false
	}
	if now.Before(attempt.lockedUntil) {
		return attempt.lockedUntil, true
	}

	delete(l.attempts, key)
	return time.Time{}, false
}

func (l *loginRateLimiter) recordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.currentTime()
	l.pruneExpired(now)

	attempt := l.attempts[key]
	if !attempt.lockedUntil.IsZero() && now.Before(attempt.lockedUntil) {
		return
	}
	if attempt.firstFailure.IsZero() || now.Sub(attempt.firstFailure) > loginFailureWindow {
		attempt = loginAttempt{firstFailure: now}
	}

	attempt.failures++
	if attempt.failures >= loginFailureLimit {
		attempt.lockedUntil = now.Add(loginLockoutDuration)
	}

	l.attempts[key] = attempt
}

func (l *loginRateLimiter) recordSuccess(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, key)
}

func (l *loginRateLimiter) currentTime() time.Time {
	if l.now != nil {
		return l.now()
	}

	return time.Now()
}

func (l *loginRateLimiter) pruneExpired(now time.Time) {
	for key, attempt := range l.attempts {
		if !attempt.lockedUntil.IsZero() {
			if !now.Before(attempt.lockedUntil) {
				delete(l.attempts, key)
			}
			continue
		}
		if !attempt.firstFailure.IsZero() && now.Sub(attempt.firstFailure) > loginFailureWindow {
			delete(l.attempts, key)
		}
	}
}
