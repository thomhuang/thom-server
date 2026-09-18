package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"thom-server/internal/clientip"
)

// Checkout is public and every call creates a Stripe Checkout Session and a
// pending order row, so it is the cheapest endpoint to abuse. The limit is
// per-client and generous enough that a buyer retrying a few times is never
// blocked.
const (
	checkoutRateLimit  = 10
	checkoutRateWindow = 10 * time.Minute
)

// rateLimiter is a fixed-window request counter keyed by client. A fixed window
// can admit up to twice the limit across a boundary, which is fine for burst
// protection and avoids storing a timestamp per request.
type rateLimiter struct {
	mu        sync.Mutex
	entries   map[string]rateLimitEntry
	limit     int
	window    time.Duration
	lastPrune time.Time
	now       func() time.Time
}

type rateLimitEntry struct {
	count       int
	windowStart time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		entries: make(map[string]rateLimitEntry),
		limit:   limit,
		window:  window,
		now:     time.Now,
	}
}

// allow records one request for key and reports whether it is within the limit.
// When it is not, it also returns how long until the window resets so the caller
// can set Retry-After.
func (l *rateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	// Prune at most once per window: doing it on every request would make a
	// burst from many clients quadratic.
	if now.Sub(l.lastPrune) >= l.window {
		for existingKey, entry := range l.entries {
			if now.Sub(entry.windowStart) >= l.window {
				delete(l.entries, existingKey)
			}
		}
		l.lastPrune = now
	}

	entry := l.entries[key]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= l.window {
		entry = rateLimitEntry{windowStart: now}
	}

	entry.count++
	l.entries[key] = entry

	if entry.count > l.limit {
		return false, l.window - now.Sub(entry.windowStart)
	}

	return true, 0
}

// limitCheckout rejects a client that has started too many Checkout Sessions
// recently. It wraps the route so a throttled request never reaches Stripe or
// inserts a pending order.
//
// The Stripe webhook is deliberately not limited: it is signature-verified and
// Stripe retries failed deliveries, so throttling it would drop real payments.
func (app *App) limitCheckout(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := clientip.FromRequest(r)

		allowed, retryAfter := app.checkoutLimiter.allow(key)
		if !allowed {
			app.infoLog.Printf("checkout rate-limited ip=%s", key)
			w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			app.responder.ClientError(w, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	}
}
