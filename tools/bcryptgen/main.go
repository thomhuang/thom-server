// Command bcryptgen prints a bcrypt hash for a password read from stdin.
//
// It exists so that the value stored in ADMIN_PASSWORD_HASH (and the matching
// value in .env.local for local development) can be regenerated with the same
// library, cost, and hash format the server verifies against
// (internal/server/auth/auth_credentials.go).
//
// Usage:
//
//	echo "my-new-password" | go run ./tools/bcryptgen
//
// See CLOUDFLARE.md, "Rotating the admin password".
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// passwordCost matches the cost used for existing hashes in the repository.
const passwordCost = 12

func main() {
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		fmt.Fprintln(os.Stderr, "no password on stdin")
		os.Exit(1)
	}

	password := strings.TrimRight(sc.Text(), "\r\n")
	if password == "" {
		fmt.Fprintln(os.Stderr, "refusing to hash an empty password")
		os.Exit(1)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bcrypt:", err)
		os.Exit(1)
	}

	fmt.Println(string(hash))
}
