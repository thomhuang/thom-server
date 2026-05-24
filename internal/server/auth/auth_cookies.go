package auth

import "net/http"

func (app *Handler) setAuthCookie(w http.ResponseWriter, value string, maxAge int) {
	sameSiteMode := http.SameSiteLaxMode
	if app.config.SecureCookies {
		sameSiteMode = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   app.config.SecureCookies,
		SameSite: sameSiteMode,
	})
}
