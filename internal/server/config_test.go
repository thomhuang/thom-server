package server

import (
	"os"
	"path/filepath"
	"testing"
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
