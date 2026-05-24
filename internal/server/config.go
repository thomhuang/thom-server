package server

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"thom-server/internal/server/auth"
)

type Config struct {
	AdminUsername     string
	AdminPasswordHash string
	JWTSecret         string
	ClientOrigins     []string
	SecureCookies     bool
}

func (c Config) authConfig() auth.Config {
	return auth.Config{
		AdminUsername:     c.AdminUsername,
		AdminPasswordHash: c.AdminPasswordHash,
		JWTSecret:         c.JWTSecret,
		SecureCookies:     c.SecureCookies,
	}
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
