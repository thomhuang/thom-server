package server

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"thom-server/internal/r2"
)

func TestLoadLocalEnv(t *testing.T) {
	t.Setenv("EXISTING_VALUE", "from-shell")
	restoreEnv(t,
		"ADMIN_USERNAME",
		"ADMIN_PASSWORD_HASH",
		"QUOTED_VALUE",
		"SINGLE_QUOTED_VALUE",
		"CLIENT_ORIGIN_URLS",
		"HASH_VALUE",
		"SECURE_COOKIES",
	)

	envPath := filepath.Join(t.TempDir(), ".env.local")
	content := []byte(`
# comment
ADMIN_USERNAME=local-admin
ADMIN_PASSWORD_HASH=$2y$12$example
EXISTING_VALUE=from-file
QUOTED_VALUE="hello\nworld"
SINGLE_QUOTED_VALUE='literal value'
CLIENT_ORIGIN_URLS=http://localhost:3000,http://localhost:3001 # local origins
HASH_VALUE=foo#bar
export SECURE_COOKIES=false
`)
	if err := os.WriteFile(envPath, content, 0600); err != nil {
		t.Fatal(err)
	}

	if err := LoadLocalEnv(envPath); err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{
		"ADMIN_USERNAME":      "local-admin",
		"ADMIN_PASSWORD_HASH": "$2y$12$example",
		"EXISTING_VALUE":      "from-shell",
		"QUOTED_VALUE":        "hello\nworld",
		"SINGLE_QUOTED_VALUE": "literal value",
		"CLIENT_ORIGIN_URLS":  "http://localhost:3000,http://localhost:3001",
		"HASH_VALUE":          "foo#bar",
		"SECURE_COOKIES":      "false",
	}

	for key, want := range tests {
		if got := os.Getenv(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestLoadLocalEnvIgnoresMissingFile(t *testing.T) {
	err := LoadLocalEnv(filepath.Join(t.TempDir(), ".env.local"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadLocalEnvRejectsMalformedLine(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env.local")
	if err := os.WriteFile(envPath, []byte("ADMIN_USERNAME\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := LoadLocalEnv(envPath); err == nil {
		t.Fatal("expected malformed env file to return an error")
	}
}

func TestClientOriginsFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		origins string
		legacy  string
		want    []string
	}{
		{
			name: "default origins",
			want: []string{"http://localhost:3000", "http://localhost:3001"},
		},
		{
			name:   "legacy single origin",
			legacy: "https://legacy.example.com",
			want:   []string{"https://legacy.example.com"},
		},
		{
			name:    "comma separated origins are trimmed",
			origins: " https://app.example.com, ,https://admin.example.com ",
			want:    []string{"https://app.example.com", "https://admin.example.com"},
		},
		{
			name:    "origin list takes precedence",
			origins: "https://primary.example.com",
			legacy:  "https://legacy.example.com",
			want:    []string{"https://primary.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLIENT_ORIGIN_URLS", tt.origins)
			t.Setenv("CLIENT_ORIGIN_URL", tt.legacy)

			got := ClientOriginsFromEnv()
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ClientOriginsFromEnv() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestConfigValidateJWTSecret(t *testing.T) {
	tests := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{
			name:   "empty secret is allowed",
			secret: "",
		},
		{
			name:    "31 characters is rejected",
			secret:  strings.Repeat("a", 31),
			wantErr: true,
		},
		{
			name:   "32 characters is allowed",
			secret: strings.Repeat("a", 32),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Config{JWTSecret: tt.secret}.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSecureCookiesFromEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "true", value: "true", want: true},
		{name: "uppercase true", value: "TRUE", want: true},
		{name: "false", value: "false", want: false},
		{name: "one", value: "1", want: false},
		{name: "empty", value: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SECURE_COOKIES", tt.value)

			if got := SecureCookiesFromEnv(); got != tt.want {
				t.Fatalf("SecureCookiesFromEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripeConfigFromEnv(t *testing.T) {
	origins := []string{"https://example.com/"}

	tests := []struct {
		name          string
		taxEnabled    string
		shippingCents string
		wantTax       bool
		wantShipping  int
	}{
		{
			name: "defaults",
		},
		{
			name:       "tax enabled",
			taxEnabled: "true",
			wantTax:    true,
		},
		{
			name:       "tax disabled",
			taxEnabled: "false",
		},
		{
			name:          "shipping cents parsed",
			shippingCents: "500",
			wantShipping:  500,
		},
		{
			// A non-numeric value silently falls back to 0; this pins that
			// current behavior rather than asserting it is desirable.
			name:          "non-numeric shipping falls back to zero",
			shippingCents: "abc",
			wantShipping:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("STRIPE_TAX_ENABLED", tt.taxEnabled)
			t.Setenv("STRIPE_SHIPPING_CENTS", tt.shippingCents)

			got := StripeConfigFromEnv(origins)
			if got.TaxEnabled != tt.wantTax {
				t.Fatalf("TaxEnabled = %v, want %v", got.TaxEnabled, tt.wantTax)
			}
			if got.ShippingCents != tt.wantShipping {
				t.Fatalf("ShippingCents = %d, want %d", got.ShippingCents, tt.wantShipping)
			}
			if want := "https://example.com/shop/order?session_id={CHECKOUT_SESSION_ID}"; got.SuccessURL != want {
				t.Fatalf("SuccessURL = %q, want %q", got.SuccessURL, want)
			}
			if want := "https://example.com/shop"; got.CancelURL != want {
				t.Fatalf("CancelURL = %q, want %q", got.CancelURL, want)
			}
		})
	}
}

func TestEmailConfigFromEnv(t *testing.T) {
	origins := []string{"https://example.com/"}
	restoreEnv(t, "EMAIL_API_TOKEN", "EMAIL_ACCOUNT_ID", "EMAIL_FROM", "EMAIL_FROM_NAME", "PUBLIC_SITE_URL")

	t.Run("trims values and defaults the site url to the first origin", func(t *testing.T) {
		t.Setenv("EMAIL_API_TOKEN", " token ")
		t.Setenv("EMAIL_ACCOUNT_ID", " acct ")
		t.Setenv("EMAIL_FROM", " orders@example.com ")
		t.Setenv("EMAIL_FROM_NAME", " Thom Huang ")
		t.Setenv("PUBLIC_SITE_URL", "")

		got := EmailConfigFromEnv(origins)
		want := EmailConfig{
			APIToken:  "token",
			AccountID: "acct",
			From:      "orders@example.com",
			FromName:  "Thom Huang",
			SiteURL:   "https://example.com",
		}
		if got != want {
			t.Fatalf("EmailConfigFromEnv() = %#v, want %#v", got, want)
		}
	})

	t.Run("public site url overrides the origin", func(t *testing.T) {
		t.Setenv("PUBLIC_SITE_URL", "https://shop.example.com/")

		if got := EmailConfigFromEnv(origins).SiteURL; got != "https://shop.example.com" {
			t.Fatalf("SiteURL = %q, want the trimmed override", got)
		}
	})

	t.Run("no origins leaves the site url empty", func(t *testing.T) {
		t.Setenv("PUBLIC_SITE_URL", "")

		if got := EmailConfigFromEnv(nil).SiteURL; got != "" {
			t.Fatalf("SiteURL = %q, want empty without an origin", got)
		}
	})
}

func TestR2ConfigFromEnvTrimsWhitespace(t *testing.T) {
	t.Setenv("R2_ACCOUNT_ID", "  account  ")
	t.Setenv("R2_BUCKET", " bucket ")
	t.Setenv("R2_ACCESS_KEY_ID", " key ")
	t.Setenv("R2_SECRET_ACCESS_KEY", " secret ")
	t.Setenv("R2_ENDPOINT", " https://example.com ")

	want := r2.Config{
		AccountID:       "account",
		Bucket:          "bucket",
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
		Endpoint:        "https://example.com",
	}

	if got := R2ConfigFromEnv(); !reflect.DeepEqual(got, want) {
		t.Fatalf("R2ConfigFromEnv() = %#v, want %#v", got, want)
	}
}

func restoreEnv(t *testing.T, keys ...string) {
	t.Helper()

	for _, key := range keys {
		value, exists := os.LookupEnv(key)
		t.Cleanup(func() {
			if exists {
				if err := os.Setenv(key, value); err != nil {
					t.Fatal(err)
				}
				return
			}

			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		})
	}
}
