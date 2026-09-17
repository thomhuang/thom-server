# Deploying thom-server to Cloudflare

The Go server is unchanged in shape (`cmd/`, `internal/`, `database/sql`). Only the
database backend and the deployment glue change:

- **SQLite → Cloudflare D1.** `internal/server/database.go` opens D1 when
  `D1_ACCOUNT_ID`, `D1_DATABASE_ID`, and `CF_API_TOKEN` are set, otherwise it opens
  the local SQLite file exactly as before. `internal/d1` is a small `database/sql`
  driver that talks to the D1 HTTP API.
- **Container + Worker shim.** `wrangler.jsonc` + `worker/index.js` run the Go
  binary (`Dockerfile`) as a single Cloudflare Container and forward requests to it.
  All application logic stays in Go.

Local development is unchanged: `go run ./cmd/server` uses `DB_PATH`/SQLite.

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

Generate a bcrypt hash with the helper in this repository (it uses the same
`golang.org/x/crypto/bcrypt` version and cost the server verifies against):

```sh
echo "your-password" | go run ./tools/bcryptgen
```

`htpasswd -bnBC 12 "" "your-password"` also works if you have it installed;
the server accepts `$2a$`, `$2b$`, and `$2y$` hashes.

## 4. Point CORS at the website

Set `CLIENT_ORIGIN_URLS` in `wrangler.jsonc` to the `thom-website` origin (for
example `https://thom-website.your-subdomain.workers.dev`). This is the origin the
browser sends; the website Worker proxies `/api/*` so requests stay same-origin.

## 5. Deploy

Locally (Docker running):

```sh
npx wrangler deploy
```

Or connect the repository under **Workers & Pages → thom-server → Settings → Builds**
and use:

- Build command: `npm install`
- Deploy command: `npx wrangler deploy`

The first deploy can take a few minutes to provision the container.

## 6. Load your existing coffee data

The tables are created automatically on first container start. Trigger it once,
then import the local rows:

```sh
curl https://thom-server.your-subdomain.workers.dev/coffee

# Export rows (schema is handled by the server) and import them.
sqlite3 internal/thom.db .dump > thom-dump.sql
# PowerShell: Select-String -Path thom-dump.sql -Pattern '^INSERT' | ForEach-Object Line > thom-data.sql
grep '^INSERT' thom-dump.sql > thom-data.sql
npx wrangler d1 execute thom-db --remote --file=thom-data.sql
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

## Notes and trade-offs

- D1 does not support interactive transactions, so `Model.Insert`/`Model.Update`
  upsert the roaster and then write the entry as separate statements. A failed entry
  insert can leave an unused roaster row; entries themselves are still atomic.
- Login rate limiting and logout token revocation remain in-memory and reset when
  the container restarts or sleeps. A single container instance keeps them
  consistent while it is running.
- The container sleeps after 10 minutes idle; the first request after that has a
  cold start (usually 1–3 seconds). Adjust `sleepAfter` in `worker/index.js`.
- Adopt `instance_type`/`max_instances` in `wrangler.jsonc` if you need more
  resources. `basic` is 1 GiB / 1/4 vCPU / 4 GB.

## Local development

```sh
cp .env.example .env.local   # fill in the auth values
go run ./cmd/server          # SQLite, unchanged
go test ./...
```

`wrangler dev` (which runs the container) needs Docker; the D1 driver has its own
tests that run without CGO: `go test ./internal/d1/`.
