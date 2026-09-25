# Deploying thom-server to Cloudflare

The Go server is unchanged in shape (`cmd/`, `internal/`, `database/sql`). Only the
database backend and the deployment glue change:

- **SQLite → Cloudflare D1.** `internal/server/database.go` always opens D1
  from `D1_ACCOUNT_ID`, `D1_DATABASE_ID`, and `CF_API_TOKEN`; there is no local
  database fallback. `internal/d1` is a small `database/sql` driver that talks
  to the D1 HTTP API.
- **Container + Worker shim.** `wrangler.jsonc` + `worker/index.js` run the Go
  binary (`Dockerfile`) as a single Cloudflare Container and forward requests to it.
  All application logic stays in Go.

Local development points at the **test** database and test R2 bucket; see
[Local development](#local-development). The only SQLite left is the in-memory
database the Go unit tests create for themselves.

## Environments

Two parallel environments are deployed from this repository, selected by the
Wrangler config file:

| Environment | Config | Worker | D1 | R2 | Website |
|---|---|---|---|---|---|
| production | `wrangler.jsonc` | `thom-server` | `thom-db` (`d9ebb328-8e11-474f-b3c8-fe2660bd9c8c`) | `listing-images`, custom domain `thomhuang.com` | `https://www.thomhuang.com` |
| test | `wrangler.test.jsonc` | `thom-server-test` | `thom-db-test` (`de8610e7-76af-427b-a228-a742babcb989`) | `listing-images-test`, `r2.dev` | `https://thom-website-test.thomhuang.workers.dev` |

Secrets are per-Worker, so set them against the matching config file:

```sh
npx wrangler secret put CF_API_TOKEN -c wrangler.test.jsonc
```

Verified 2026-09-18: the **test** Worker has `CF_API_TOKEN` set (a
`secret_text` binding) and its plain-text vars point at `thom-db-test` and
`listing-images-test`, so the no-fallback server runs against the test database.
`thom-db-test` already holds the app tables; a container start applies any
missing tables or columns through `EnsureSchema`. Workers Builds deploys
production from the **`release`** branch, so pushes to `main` do not deploy:
promote `main` → `release` to ship, or deploy manually (below).

## Prerequisites

- A Cloudflare account on the **Workers Paid** plan ($5/mo, required by Containers).
- Node.js 22+ (`wrangler` requires it), Docker (for `wrangler deploy` locally, or
  let Workers Builds build the image).
- `npx wrangler login` once.

## 1. Create the D1 database

```sh
npx wrangler d1 create thom-db
```

Copy the printed `database_id` into the `D1_DATABASE_ID` var in `wrangler.jsonc`,
and your account ID into `D1_ACCOUNT_ID`.

## 2. Create a D1 API token for the container

The container talks to D1 over HTTP, so it needs its own token:

1. Cloudflare dashboard → **My Profile** → **API Tokens** → **Create Token**.
2. Use a custom token with **Account → D1 → Edit** scoped to your account.
3. Save the token as a Worker secret:

```sh
npx wrangler secret put CF_API_TOKEN
```

## 3. Set the remaining secrets

```sh
npx wrangler secret put ADMIN_USERNAME
npx wrangler secret put ADMIN_PASSWORD_HASH   # bcrypt hash, see README
npx wrangler secret put JWT_SECRET
```

`JWT_SECRET` must be at least 32 characters; the server refuses to start with a
shorter one. Generate it with something like `openssl rand -base64 48`.

Generate a bcrypt hash with the helper in this repository (it uses the same
`golang.org/x/crypto/bcrypt` version and cost the server verifies against):

```sh
echo "your-password" | go run ./tools/bcryptgen
```

`htpasswd -bnBC 12 "" "your-password"` also works if you have it installed;
the server accepts `$2a$`, `$2b$`, and `$2y$` hashes.

## 4. Point CORS at the website

Set `CLIENT_ORIGIN_URLS` in the matching config file to the website origin:
production uses `https://www.thomhuang.com`, test uses
`https://thom-website-test.thomhuang.workers.dev`. This is the origin the
browser sends; the website Worker proxies `/api/*` so requests stay same-origin.

## 5. Deploy

Locally (Docker running):

```sh
npx wrangler deploy                    # production (wrangler.jsonc)
npx wrangler deploy -c wrangler.test.jsonc   # test (thom-server-test)
```

Or connect the repository under **Workers & Pages → thom-server → Settings → Builds**
and use:

- Build command: `npm install`
- Deploy command: `npx wrangler deploy`

Set the **production branch** to `release`. Pushes to `main` do not trigger a
production build; promote with `git push origin main:release` (a fast-forward)
when shipping. Workers Builds deploys production only. The test Worker is
deployed manually with `-c wrangler.test.jsonc`.

The first deploy can take a few minutes to provision the container.

## 6. Load your existing coffee data

The tables are created automatically on first container start. Trigger it once.
If you still have the legacy `internal/thom.db` SQLite file from an old
checkout, export and import its rows (the server itself no longer reads it):

```sh
curl https://www.thomhuang.com/api/coffee
# test: https://thom-website-test.thomhuang.workers.dev/api/coffee

# Export rows (schema is handled by the server) and import them.
sqlite3 internal/thom.db .dump > thom-dump.sql
# PowerShell: Select-String -Path thom-dump.sql -Pattern '^INSERT' | ForEach-Object Line > thom-data.sql
grep '^INSERT' thom-dump.sql > thom-data.sql
npx wrangler d1 execute thom-db --remote --file=thom-data.sql
# test: npx wrangler d1 execute thom-db-test -c wrangler.test.jsonc --remote --file=thom-data.sql
```

`CoffeeRoasters` is also seeded from `CoffeeEntries` on startup, so roasters you did
not import explicitly are recreated.

## Rotating the admin password

The login credentials live in two Worker secrets: `ADMIN_USERNAME` and
`ADMIN_PASSWORD_HASH`. Only the bcrypt hash is stored; the plaintext password
never reaches Cloudflare.

To change the password:

```sh
cd thom-server

# 1. Hash the new password. -AsSecureString keeps it off the screen and out of
#    your shell history.
$sec = Read-Host -Prompt "New admin password" -AsSecureString
$pw  = [Runtime.InteropServices.Marshal]::PtrToStringAuto(
          [Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec))

# 2. Store the hash as a Worker secret.
$pw | go run ./tools/bcryptgen | npx wrangler secret put ADMIN_PASSWORD_HASH

# 3. Clear the variable when you are done.
Remove-Variable pw, sec
```

The new hash takes effect on the next request that spins up the container; no
redeploy is needed. Existing sessions are **not** invalidated, because they are
signed with `JWT_SECRET` rather than derived from the password. If you want to
force everyone out, rotate `JWT_SECRET` as well:

```sh
node -e "console.log(require('crypto').randomBytes(48).toString('base64url'))" |
  npx wrangler secret put JWT_SECRET
```

That invalidates every issued token, so you will need to sign in again.

`ADMIN_USERNAME` can be changed the same way, without hashing:

```sh
npx wrangler secret put ADMIN_USERNAME
```

For local development, put the same hash in `.env.local` (gitignored) as
`ADMIN_PASSWORD_HASH`.

## R2 (shop images)

Shop listings keep only the object key in D1; image bytes go straight from the
browser to R2 with a presigned `PUT`. The container therefore needs bucket
credentials. Set them as Worker secrets:

```sh
npx wrangler secret put R2_ACCESS_KEY_ID
npx wrangler secret put R2_SECRET_ACCESS_KEY
```

The non-secret values live in the `vars` block of each config file:
`R2_ACCOUNT_ID` (the same value as `D1_ACCOUNT_ID`), `R2_BUCKET`, and
`R2_PUBLIC_BASE_URL`. `R2_PUBLIC_BASE_URL` is the scheme + host that serves the
bucket's objects — the bucket's `r2.dev` URL or a custom domain — with no
trailing slash. Public buckets do not list contents, so the bare root 404s and
only full object paths resolve.

| Environment | Bucket | `R2_PUBLIC_BASE_URL` |
|---|---|---|
| production | `listing-images` | `https://img.thomhuang.com` (custom domain, verified serving) |
| test | `listing-images-test` | `https://pub-9a8dcf30844143e6a0515c4feb4987f9.r2.dev` |

`R2_ENDPOINT` is a test-only override; do not set it in a real environment.

## Stripe (shop checkout)

Checkout creates a hosted Stripe Checkout Session and redirects the browser, so
no Stripe key ever ships to the client. Set the secrets:

```sh
npx wrangler secret put STRIPE_SECRET_KEY
npx wrangler secret put STRIPE_WEBHOOK_SECRET
```

`STRIPE_TAX_ENABLED` (default `false`) and `STRIPE_SHIPPING_CENTS` (default
`1000`, a flat $10 US shipping line; set `0` for free shipping) are vars in
`wrangler.jsonc`. Leave tax off until tax
registrations are configured; Stripe rejects `automatic_tax` otherwise.

Point a webhook endpoint at `<server origin>/shop/webhooks/stripe` for the
`checkout.session.completed` event and use its signing secret as
`STRIPE_WEBHOOK_SECRET`. Through the website proxy that is
`https://www.thomhuang.com/api/shop/webhooks/stripe` in production and
`https://thom-website-test.thomhuang.workers.dev/api/shop/webhooks/stripe` in
test. Locally, `stripe listen --forward-to
localhost:4000/shop/webhooks/stripe` prints a temporary secret. The success and
cancel redirects are derived from the first `CLIENT_ORIGIN_URLS` entry
(`/shop/order?session_id=...` and `/shop`).

Checkout collects a shipping address and is restricted to **US** destinations
(`shipping_address_collection.allowed_countries` in
`internal/server/shop/stripe.go`). The address is read from the session's
`collected_information.shipping_details` on `checkout.session.completed` and
stored on `ShopOrders`. An oversold order is voided when its PaymentIntent is
still uncaptured, otherwise refunded, and moves to `refunded`.

## Cloudflare Email Service (buyer order links)

After a payment is recorded, the server emails the buyer a link to their order.
The Go container sends through the Email Service REST API (a plain HTTPS call),
not the Worker `send_email` binding. Set the secret:

```sh
npx wrangler secret put EMAIL_API_TOKEN
```

The token is a Cloudflare API token with permission to send email. The
non-secret values are vars: `EMAIL_ACCOUNT_ID` (the same value as
`D1_ACCOUNT_ID`), `EMAIL_FROM` (for example `orders@thomhuang.com`),
`EMAIL_FROM_NAME`, and optional `PUBLIC_SITE_URL` (defaults to the first
`CLIENT_ORIGIN_URLS` entry; this is the origin of the emailed link). Order email
stays silently disabled unless `EMAIL_ACCOUNT_ID`, `EMAIL_FROM`, and
`EMAIL_API_TOKEN` are all set.

The sender domain must be onboarded before mail reaches arbitrary buyers:
until then Email Sending can only deliver to verified destination addresses.
Onboard it in the dashboard (**Compute & AI > Email Service > Email Sending >
Onboard Domain**), which adds SPF and DKIM records, or via
`npx wrangler email sending enable thomhuang.com`. Emails are sent from
`EMAIL_FROM`, so the domain in that address must be the onboarded one.

Each paid order gets one random token; only its SHA-256 hash is stored
(`ShopOrders.ViewTokenHash`). `GET /shop/orders/view/{token}` returns the full
order (customer and shipping fields) because the token is the credential, and is
marked `no-store`/`no-referrer`/`noindex`. Existing orders have no token, and a
failed send is not retried.

## Notes and trade-offs

- D1 does not support interactive transactions, so `Model.Insert`/`Model.Update`
  upsert the roaster and then write the entry as separate statements. A failed entry
  insert can leave an unused roaster row; entries themselves are still atomic.
- Login rate limiting and logout token revocation are stored in the app database
  (`AuthLoginFailures` / `AuthRevokedTokens`), so they survive restarts and sleep
  and are shared if the deployment ever scales past one instance. The in-memory
  store is only the fallback for runs without a database (tests).
- The container sleeps after `sleepAfter` idle (`"1h"` here, up from the `10m`
  default); the first request after that has a cold start. Adjust `sleepAfter`
  in `worker/index.js`. Startup schema checks are gated behind a persisted
  `schemaVersion` (`internal/server/schema.go`), so a warm start skips the DDL
  and only pays two D1 round trips; bump that constant whenever a schema
  changes.
- Adopt `instance_type`/`max_instances` in `wrangler.jsonc` if you need more
  resources. `basic` is 1 GiB / 1/4 vCPU / 4 GB.

## Local development

```sh
cp .env.example .env.local   # fill in CF_API_TOKEN (and R2 creds)
go run ./cmd/server          # test D1 + test R2, API on :4000
go test ./...                # in-memory SQLite
```

`.env.example` already points `D1_DATABASE_ID`, `R2_BUCKET`, and
`R2_PUBLIC_BASE_URL` at the test environment, so local runs use test data. The
server refuses to start without D1 configured.

To run the container locally instead, use the test Wrangler config (Docker
required), with secrets from `.dev.vars`:

```sh
npx wrangler dev -c wrangler.test.jsonc
```

The D1 driver has its own tests: `go test ./internal/d1/`.
