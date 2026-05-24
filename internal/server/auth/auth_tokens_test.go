package auth

import (
	"testing"
	"time"
)

func TestAuthTokenRoundTrip(t *testing.T) {
	app := &Handler{
		config: Config{
			JWTSecret: "test-secret",
		},
	}

	token, err := app.createAuthToken("thom")
	if err != nil {
		t.Fatal(err)
	}

	claims, err := app.verifyAuthToken(token)
	if err != nil {
		t.Fatal(err)
	}

	if claims.Subject != "admin" {
		t.Fatalf("expected subject admin, got %q", claims.Subject)
	}
	if claims.Username != "thom" {
		t.Fatalf("expected username thom, got %q", claims.Username)
	}
}

func TestAuthTokenRejectsTampering(t *testing.T) {
	app := &Handler{
		config: Config{
			JWTSecret: "test-secret",
		},
	}

	token, err := app.createAuthToken("thom")
	if err != nil {
		t.Fatal(err)
	}

	replacement := "x"
	if token[len(token)-1:] == replacement {
		replacement = "y"
	}

	tamperedToken := token[:len(token)-1] + replacement
	if _, err := app.verifyAuthToken(tamperedToken); err == nil {
		t.Fatal("expected tampered token to fail verification")
	}
}

func TestAuthTokenRejectsExpiredClaims(t *testing.T) {
	app := &Handler{
		config: Config{
			JWTSecret: "test-secret",
		},
	}

	headerSegment, err := encodeJSONSegment(map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	})
	if err != nil {
		t.Fatal(err)
	}

	claimsSegment, err := encodeJSONSegment(authClaims{
		Subject:   "admin",
		Username:  "thom",
		IssuedAt:  time.Now().Add(-2 * time.Hour).Unix(),
		ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	signingInput := headerSegment + "." + claimsSegment
	expiredToken := signingInput + "." + app.signAuthToken(signingInput)

	if _, err := app.verifyAuthToken(expiredToken); err == nil {
		t.Fatal("expected expired token to fail verification")
	}
}

func TestAuthTokenRejectsInvalidInputs(t *testing.T) {
	app := &Handler{}
	if _, err := app.verifyAuthToken("header.claims.signature"); err == nil {
		t.Fatal("expected token verification to fail when auth is not configured")
	}

	app.config.JWTSecret = "test-secret"
	if _, err := app.verifyAuthToken("not-a-token"); err == nil {
		t.Fatal("expected malformed token to fail verification")
	}

	headerSegment, err := encodeJSONSegment(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		t.Fatal(err)
	}
	claimsSegment, err := encodeJSONSegment(authClaims{
		Subject:   "admin",
		Username:  "thom",
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	signingInput := headerSegment + "." + claimsSegment
	token := signingInput + "." + app.signAuthToken(signingInput)
	if _, err := app.verifyAuthToken(token); err == nil {
		t.Fatal("expected unsupported token header to fail verification")
	}
}
