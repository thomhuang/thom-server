// Package clientip resolves the client address used as a rate-limit key. The
// trust decision lives here once so the login throttle and the checkout limiter
// cannot drift apart.
package clientip

import (
	"net"
	"net/http"
	"strings"
)

// FromRequest returns the client address. Cloudflare overwrites CF-Connecting-IP
// at the edge, so it carries the real client address rather than a value the
// caller can choose; RemoteAddr is the fallback for local runs and tests.
func FromRequest(r *http.Request) string {
	if host := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); host != "" {
		return host
	}

	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
		return host
	}

	return r.RemoteAddr
}
