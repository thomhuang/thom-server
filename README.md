# thom-server
Web Server for Website.

Currently following along `Let's Go` by Alex Edwards: https://lets-go.alexedwards.net/

## Local development

The server defaults to the app database at `internal/thom.db`. Use `DB_PATH` or `go run ./cmd/server -db-path="./internal/thom.db"` to point at it explicitly.

## Local auth configuration

JWT cookie auth is configured with environment variables:

- `ADMIN_USERNAME`: admin login username.
- `ADMIN_PASSWORD_HASH`: bcrypt hash of the admin login password.
- `JWT_SECRET`: long random secret used to sign auth cookies.
- `CLIENT_ORIGIN_URLS`: comma-separated frontend origins allowed to send cookies.
- `SECURE_COOKIES`: set to `true` in HTTPS environments; this also uses `SameSite=None` for cross-site frontend/API cookies.
- `DB_PATH`: SQLite database path. When it points at a new path, the server copies the bundled `internal/thom.db` there before opening it.

For local development, `go run ./cmd/server` loads `.env.local` into the process before reading those variables. Existing shell or container environment variables take precedence over values in `.env.local`.

Fly deployments use `/data/thom.db`; create the matching volume once with `fly volumes create thom_data --region lax` before deploying.

Generate a bcrypt password hash with:

```sh
htpasswd -bnBC 12 "" "your-password" | tr -d ':\n'
```

The raw password is still submitted to `POST /auth/login` from the browser, but HTTPS protects it in transit and the server only stores/verifies the bcrypt hash.
