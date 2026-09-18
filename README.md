# thom-server

HTTP API for the coffee journal, backing
[thom-website](https://github.com/thomhuang/thom-website).
Currently following along `Let's Go` by Alex Edwards: https://lets-go.alexedwards.net/

## Deployment

The server runs on Cloudflare: the Go binary runs as a Cloudflare Container and
stores data in Cloudflare D1. See [CLOUDFLARE.md](CLOUDFLARE.md) for setup,
secrets, data import, and deployment. Local development and tests continue to use
SQLite.

## Routes

Public:

- `GET /ping`
- `GET /coffee`, `GET /coffee/roasters`, `GET /coffee/{id}`
- `POST /auth/login`

Authenticated with the session cookie:

- `GET /auth/me`, `POST /auth/logout`
- `POST /coffee`, `PATCH /coffee/{id}`, `DELETE /coffee/{id}`
- `POST /coffee/roasters`

## Local development

```sh
cp .env.example .env.local   # fill in the auth values
go run ./cmd/server          # API on :4000
```

The server defaults to the SQLite database at `internal/thom.db`. Use `DB_PATH` or
`go run ./cmd/server -db-path="./internal/thom.db"` to point at it explicitly.
`go run ./cmd/server` loads `.env.local` before reading those variables; existing
shell or container environment variables take precedence.

```sh
go test ./...           # all tests (local SQLite, no C toolchain needed)
go test ./internal/d1/  # D1 driver tests only
go build ./cmd/server
```

## Configuration

JWT cookie auth is configured with environment variables:

- `ADMIN_USERNAME`: admin login username.
- `ADMIN_PASSWORD_HASH`: bcrypt hash of the admin login password.
- `JWT_SECRET`: long random secret used to sign auth cookies.
- `CLIENT_ORIGIN_URLS`: comma-separated frontend origins allowed to send cookies.
- `SECURE_COOKIES`: set to `true` in HTTPS environments; this also uses `SameSite=None` for frontend/API cookies.
- `DB_PATH`: local SQLite database path. When it points at a new path, the server copies the bundled `internal/thom.db` there before opening it.
- `D1_ACCOUNT_ID`, `D1_DATABASE_ID`, `CF_API_TOKEN`, `D1_ENDPOINT`: when the first three are set, the server uses Cloudflare D1 instead of SQLite. `D1_ENDPOINT` defaults to the public Cloudflare API.

Generate a bcrypt password hash with:

```sh
htpasswd -bnBC 12 "" "your-password" | tr -d ':\n'
```

The raw password is submitted to `POST /auth/login` from the browser, but HTTPS
protects it in transit and the server only stores/verifies the bcrypt hash.
