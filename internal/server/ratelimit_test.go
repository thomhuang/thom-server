package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"thom-server/internal/clientip"
)

func TestRateLimiterAllowsUpToLimitThenBlocks(t *testing.T) {
	limiter := newRateLimiter(3, time.Minute)
	base := time.Unix(1000, 0)
	limiter.now = func() time.Time { return base }

	for attempt := 1; attempt <= 3; attempt++ {
		if allowed, _ := limiter.allow("1.2.3.4"); !allowed {
			t.Fatalf("attempt %d should be allowed", attempt)
		}
	}

	allowed, retryAfter := limiter.allow("1.2.3.4")
	if allowed {
		t.Fatal("expected the fourth attempt to be blocked")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected a positive retry-after, got %s", retryAfter)
	}
}

func TestRateLimiterKeysAreIndependent(t *testing.T) {
	limiter := newRateLimiter(1, time.Minute)
	limiter.now = func() time.Time { return time.Unix(1000, 0) }

	if allowed, _ := limiter.allow("1.2.3.4"); !allowed {
		t.Fatal("first client should be allowed")
	}
	if allowed, _ := limiter.allow("1.2.3.4"); allowed {
		t.Fatal("first client should now be blocked")
	}

	if allowed, _ := limiter.allow("5.6.7.8"); !allowed {
		t.Fatal("a different client should not be affected")
	}
}

func TestRateLimiterResetsAfterWindow(t *testing.T) {
	limiter := newRateLimiter(1, time.Minute)
	now := time.Unix(1000, 0)
	limiter.now = func() time.Time { return now }

	if allowed, _ := limiter.allow("1.2.3.4"); !allowed {
		t.Fatal("first attempt should be allowed")
	}
	if allowed, _ := limiter.allow("1.2.3.4"); allowed {
		t.Fatal("second attempt should be blocked")
	}

	now = now.Add(time.Minute)

	if allowed, _ := limiter.allow("1.2.3.4"); !allowed {
		t.Fatal("expected the window to have reset")
	}
}

func TestRateLimiterPrunesStaleEntries(t *testing.T) {
	limiter := newRateLimiter(1, time.Minute)
	now := time.Unix(1000, 0)
	limiter.now = func() time.Time { return now }

	limiter.allow("1.2.3.4")

	now = now.Add(2 * time.Minute)
	limiter.allow("5.6.7.8")

	if _, stale := limiter.entries["1.2.3.4"]; stale {
		t.Fatal("expected the stale entry to be pruned")
	}
}

func TestClientIPPrefersCFConnectingIP(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/shop/checkout", nil)
	request.Header.Set("CF-Connecting-IP", "203.0.113.7")
	request.RemoteAddr = "10.0.0.1:5555"

	if got, want := clientip.FromRequest(request), "203.0.113.7"; got != want {
		t.Fatalf("clientip.FromRequest() = %q, want %q", got, want)
	}
}

func TestClientIPFallsBackToRemoteAddr(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/shop/checkout", nil)
	request.RemoteAddr = "10.0.0.1:5555"

	if got, want := clientip.FromRequest(request), "10.0.0.1"; got != want {
		t.Fatalf("clientip.FromRequest() = %q, want %q", got, want)
	}
}

func TestLimitCheckoutBlocksAfterLimit(t *testing.T) {
	app := newTestApp(t)
	app.checkoutLimiter = newRateLimiter(1, time.Minute)

	handler := app.limitCheckout(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	newRequest := func() *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/shop/checkout", nil)
		request.Header.Set("CF-Connecting-IP", "203.0.113.7")
		return request
	}

	first := httptest.NewRecorder()
	handler(first, newRequest())
	if first.Code != http.StatusCreated {
		t.Fatalf("first request status = %d, want %d", first.Code, http.StatusCreated)
	}

	second := httptest.NewRecorder()
	handler(second, newRequest())
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("expected a Retry-After header on a throttled response")
	}
}
