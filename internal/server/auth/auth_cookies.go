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
	sameSiteMode := http.SameSiteLaxMode
	if h.config.SecureCookies {
		sameSiteMode = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     h.authCookieName(),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.config.SecureCookies,
		SameSite: sameSiteMode,
	})
}
