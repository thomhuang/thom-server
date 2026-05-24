package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthCookieModes(t *testing.T) {
	localApp := &Handler{}
	localRecorder := httptest.NewRecorder()

	localApp.setAuthCookie(localRecorder, "token", 60)

	localCookies := localRecorder.Result().Cookies()
	if len(localCookies) != 1 {
		t.Fatalf("expected 1 local auth cookie, got %d", len(localCookies))
	}
	if localCookies[0].Secure {
		t.Fatal("expected local auth cookie to be insecure")
	}
	if localCookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("expected local auth cookie SameSite=Lax, got %v", localCookies[0].SameSite)
	}

	secureApp := &Handler{
		config: Config{
			SecureCookies: true,
		},
	}
	secureRecorder := httptest.NewRecorder()

	secureApp.setAuthCookie(secureRecorder, "token", 60)

	secureCookies := secureRecorder.Result().Cookies()
	if len(secureCookies) != 1 {
		t.Fatalf("expected 1 secure auth cookie, got %d", len(secureCookies))
	}
	if !secureCookies[0].Secure {
		t.Fatal("expected secure auth cookie")
	}
	if secureCookies[0].SameSite != http.SameSiteNoneMode {
		t.Fatalf("expected secure auth cookie SameSite=None, got %v", secureCookies[0].SameSite)
	}
}
