package auth

import "testing"

func TestAuthConfigured(t *testing.T) {
	app := &Handler{
		config: Config{
			AdminUsername:     "admin",
			AdminPasswordHash: testAdminPasswordHash,
			JWTSecret:         "secret",
		},
	}
	if !app.authConfigured() {
		t.Fatal("expected auth to be configured")
	}

	app.config.JWTSecret = ""
	if app.authConfigured() {
		t.Fatal("expected missing JWT secret to leave auth unconfigured")
	}

	app.config.JWTSecret = "secret"
	app.config.AdminPasswordHash = ""
	if app.authConfigured() {
		t.Fatal("expected missing password hash to leave auth unconfigured")
	}

	app.config.AdminPasswordHash = "not-a-bcrypt-hash"
	if app.authConfigured() {
		t.Fatal("expected invalid password hash to leave auth unconfigured")
	}
}
