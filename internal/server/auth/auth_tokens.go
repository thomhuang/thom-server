package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (app *Handler) createAuthToken(username string) (string, error) {
	now := time.Now()
	claims := authClaims{
		Subject:   "admin",
		Username:  username,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(authTokenDuration).Unix(),
	}

	headerSegment, err := encodeJSONSegment(map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}

	claimsSegment, err := encodeJSONSegment(claims)
	if err != nil {
		return "", err
	}

	signingInput := fmt.Sprintf("%s.%s", headerSegment, claimsSegment)
	signature := app.signAuthToken(signingInput)

	return fmt.Sprintf("%s.%s", signingInput, signature), nil
}

func (app *Handler) verifyAuthToken(token string) (*authClaims, error) {
	if app.config.JWTSecret == "" {
		return nil, errors.New("auth is not configured")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token format")
	}

	signingInput := fmt.Sprintf("%s.%s", parts[0], parts[1])
	expectedSignature := app.signAuthToken(signingInput)
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSignature)) {
		return nil, errors.New("invalid token signature")
	}

	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := decodeJSONSegment(parts[0], &header); err != nil {
		return nil, err
	}
	if header.Algorithm != "HS256" || header.Type != "JWT" {
		return nil, errors.New("unsupported token header")
	}

	var claims authClaims
	if err := decodeJSONSegment(parts[1], &claims); err != nil {
		return nil, err
	}
	if claims.ExpiresAt <= time.Now().Unix() {
		return nil, errors.New("token expired")
	}

	return &claims, nil
}

func (app *Handler) signAuthToken(input string) string {
	mac := hmac.New(sha256.New, []byte(app.config.JWTSecret))
	mac.Write([]byte(input))

	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func encodeJSONSegment(value interface{}) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeJSONSegment(segment string, value interface{}) error {
	data, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, value)
}
