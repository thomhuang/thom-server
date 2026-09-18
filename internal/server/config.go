package server

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"thom-server/internal/r2"
	"thom-server/internal/server/auth"
)

type Config struct {
	AdminUsername     string
	AdminPasswordHash string
	JWTSecret         string
	ClientOrigins     []string
	SecureCookies     bool
	R2                r2.Config
	R2PublicBaseURL   string
	Stripe            StripeConfig
}

type StripeConfig struct {
	SecretKey     string
	WebhookSecret string
	TaxEnabled    bool
	ShippingCents int
	SuccessURL    string
	CancelURL     string
}

// minJWTSecretLength is the shortest signing secret the server accepts. A short
// secret is the difference between a forgeable session and a real one, so the
// server refuses to start rather than sign with one.
const minJWTSecretLength = 32

func (c Config) authConfig() auth.Config {
	return auth.Config{
		AdminUsername:     c.AdminUsername,
		AdminPasswordHash: c.AdminPasswordHash,
		JWTSecret:         c.JWTSecret,
		SecureCookies:     c.SecureCookies,
	}
}

// Validate rejects configuration that would be insecure if the server started.
func (c Config) Validate() error {
	if c.JWTSecret != "" && len(c.JWTSecret) < minJWTSecretLength {
		return fmt.Errorf("JWT_SECRET must be at least %d characters, got %d", minJWTSecretLength, len(c.JWTSecret))
	}

	return nil
}

func ClientOriginsFromEnv() []string {
	origins := os.Getenv("CLIENT_ORIGIN_URLS")
	if origins == "" {
		origins = os.Getenv("CLIENT_ORIGIN_URL")
	}
	if origins == "" {
		origins = "http://localhost:3000,http://localhost:3001"
	}

	parts := strings.Split(origins, ",")
	trimmedOrigins := make([]string, 0, len(parts))
	for _, origin := range parts {
		trimmedOrigin := strings.TrimSpace(origin)
		if trimmedOrigin != "" {
			trimmedOrigins = append(trimmedOrigins, trimmedOrigin)
		}
	}

	return trimmedOrigins
}

func SecureCookiesFromEnv() bool {
	return strings.EqualFold(os.Getenv("SECURE_COOKIES"), "true")
}

// R2ConfigFromEnv reads the credentials used to sign uploads and deletes.
// R2_ENDPOINT is only set by tests, mirroring D1_ENDPOINT.
func R2ConfigFromEnv() r2.Config {
	return r2.Config{
		AccountID:       strings.TrimSpace(os.Getenv("R2_ACCOUNT_ID")),
		Bucket:          strings.TrimSpace(os.Getenv("R2_BUCKET")),
		AccessKeyID:     strings.TrimSpace(os.Getenv("R2_ACCESS_KEY_ID")),
		SecretAccessKey: strings.TrimSpace(os.Getenv("R2_SECRET_ACCESS_KEY")),
		Endpoint:        strings.TrimSpace(os.Getenv("R2_ENDPOINT")),
	}
}

// R2PublicBaseURLFromEnv is the public delivery host images are served from,
// such as a bucket's r2.dev URL or a custom domain.
func R2PublicBaseURLFromEnv() string {
	return strings.TrimSpace(os.Getenv("R2_PUBLIC_BASE_URL"))
}

// StripeConfigFromEnv reads the Checkout settings. Success and cancel URLs are
// derived from the first client origin so the browser returns to the website
// rather than the API. Stripe Tax stays off unless explicitly enabled, because
// enabling it without configured tax registrations makes Checkout fail.
func StripeConfigFromEnv(clientOrigins []string) StripeConfig {
	origin := ""
	if len(clientOrigins) > 0 {
		origin = strings.TrimSuffix(strings.TrimSpace(clientOrigins[0]), "/")
	}

	shippingCents, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("STRIPE_SHIPPING_CENTS")))

	return StripeConfig{
		SecretKey:     strings.TrimSpace(os.Getenv("STRIPE_SECRET_KEY")),
		WebhookSecret: strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")),
		TaxEnabled:    strings.EqualFold(strings.TrimSpace(os.Getenv("STRIPE_TAX_ENABLED")), "true"),
		ShippingCents: shippingCents,
		SuccessURL:    origin + "/shop/order?session_id={CHECKOUT_SESSION_ID}",
		CancelURL:     origin + "/shop",
	}
}

func LoadLocalEnv(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: expected KEY=value", path, lineNumber)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return fmt.Errorf("%s:%d: env key cannot be empty", path, lineNumber)
		}

		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		value, err := parseEnvValue(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNumber, err)
		}

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNumber, err)
		}
	}

	return scanner.Err()
}

func parseEnvValue(value string) (string, error) {
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if first == '"' && last == '"' {
			return strconv.Unquote(value)
		}
		if first == '\'' && last == '\'' {
			return value[1 : len(value)-1], nil
		}
	}

	return strings.TrimSpace(stripInlineComment(value)), nil
}

func stripInlineComment(value string) string {
	for i := 0; i < len(value); i++ {
		if value[i] == '#' && (i == 0 || value[i-1] == ' ' || value[i-1] == '\t') {
			return value[:i]
		}
	}

	return value
}
