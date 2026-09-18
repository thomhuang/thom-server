# thom-server

HTTP API for the coffee journal, backing
[thom-website](https://github.com/thomhuang/thom-website).
Currently following along `Let's Go` by Alex Edwards: https://lets-go.alexedwards.net/

## Deployment

The server runs on Cloudflare: the Go binary runs as a Cloudflare Container and
stores data in Cloudflare D1. Two environments deploy from this repository —
production (`wrangler.jsonc` → `thom-server`) and test
(`wrangler.test.jsonc` → `thom-server-test`). See [CLOUDFLARE.md](CLOUDFLARE.md)
for setup, secrets, data import, and deployment. There is no local database:
local development points at the test D1 database and test R2 bucket, and Go
tests use in-memory SQLite.

```sh
npx wrangler deploy                        # production
npx wrangler deploy -c wrangler.test.jsonc # test
```

## Routes

Public:

- `GET /ping`
- `GET /coffee`, `GET /coffee/roasters`, `GET /coffee/grinders`, `GET /coffee/{id}`
- `GET /shop/items`, `GET /shop/items/{id}`, `GET /shop/brands` (admins also see drafts)
- `GET /shop/orders/{sessionId}` (lookup by unguessable Stripe session id)
- `POST /auth/login`, `POST /shop/checkout`, `POST /shop/webhooks/stripe`

Authenticated with the session cookie:

- `GET /auth/me`, `POST /auth/logout`
- `POST /coffee`, `PATCH /coffee/{id}`, `DELETE /coffee/{id}`
- `POST /coffee/roasters`, `POST /coffee/grinders`
- `POST /shop/items`, `PATCH /shop/items/{id}`, `DELETE /shop/items/{id}`
- `POST /shop/items/{id}/images/presign`, `POST /shop/items/{id}/images`, `DELETE /shop/items/{id}/images/{imageId}`
- `POST /shop/brands`, `GET /shop/orders`

## Local development

```sh
cp .env.example .env.local   # fill in CF_API_TOKEN (and R2 creds)
go run ./cmd/server          # API on :4000
```

`go run ./cmd/server` loads `.env.local` before reading the environment, and
existing shell or container variables take precedence. The server has no local
database: `.env.example` already points `D1_DATABASE_ID` and the R2 values at the
**test** environment (`wrangler.test.jsonc`), so fill in `CF_API_TOKEN` (and the
R2 credentials) and you are working against test data. The server refuses to
start without D1 configured.

To run the real container locally instead, use the test Wrangler config so its
vars point at test:

```sh
npx wrangler dev -c wrangler.test.jsonc   # needs Docker and .dev.vars
```

```sh
go test ./...           # in-memory SQLite, no C toolchain needed
go test ./internal/d1/  # D1 driver tests only
go build ./cmd/server
```

## Configuration

JWT cookie auth is configured with environment variables:

- `ADMIN_USERNAME`: admin login username.
- `ADMIN_PASSWORD_HASH`: bcrypt hash of the admin login password.
- `JWT_SECRET`: long random secret used to sign auth cookies.
- `CLIENT_ORIGIN_URLS`: comma-separated frontend origins allowed to send cookies.
- `SECURE_COOKIES`: set to `true` in HTTPS environments; this also switches the auth cookie to the `__Host-` prefixed name. The cookie is always `SameSite=Lax`, since the API is same-origin with the site.
- `D1_ACCOUNT_ID`, `D1_DATABASE_ID`, `CF_API_TOKEN`, `D1_ENDPOINT`: required. The server only talks to Cloudflare D1. `D1_DATABASE_ID` selects test or production and `D1_ENDPOINT` defaults to the public Cloudflare API. `CF_API_TOKEN` is a secret and is never committed.

Generate a bcrypt password hash with:

```sh
htpasswd -bnBC 12 "" "your-password" | tr -d ':\n'
```

The raw password is submitted to `POST /auth/login` from the browser, but HTTPS
protects it in transit and the server only stores/verifies the bcrypt hash.
