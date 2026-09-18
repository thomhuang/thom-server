package auth

import "net/http"

func (h *Handler) authCookieName() string {
	// The __Host- prefix requires Secure, Path=/, and no Domain attribute,
	// so it is only usable when SecureCookies is enabled.
	if h.config.SecureCookies {
		return "__Host-" + defaultAuthCookieName
	}

	return defaultAuthCookieName
}

func (h *Handler) setAuthCookie(w http.ResponseWriter, value string, maxAge int) {
	// The API is always same-origin with the site (the Worker proxies /api to
	// thom-server), so Lax still sends the cookie on every legitimate request
	// while blocking cross-site ones. SameSite=None would only widen CSRF
	// exposure, so it is not needed here even with Secure enabled.
	http.SetCookie(w, &http.Cookie{
		Name:     h.authCookieName(),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.config.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}
