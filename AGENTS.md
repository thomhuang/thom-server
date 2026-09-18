# Repository Guidelines

## Session board

Concurrent agent sessions working under `D:\Repos` log status to
`D:\Repos\BOARD.md`. **Read it before starting work**, and when you stop, append
an entry above the END sentinel using `edit` (never `write`, which replaces the
whole file). It is a shared live log, not a lock, and the repos remain the
source of truth.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

## Project Structure & Module Organization

This is a small Go 1.25 HTTP API server. The executable entrypoint lives in `cmd/server/main.go`; HTTP app wiring lives in `internal/server`. Shared server infrastructure is in `internal/server` and `internal/server/response`, auth HTTP behavior is in `internal/server/auth`, and coffee HTTP handlers are in `internal/server/coffee`. Domain and data-access code lives in `internal/coffee`, with shared data-layer errors in `internal/data` and the Cloudflare D1 `database/sql` driver in `internal/d1`. The server always talks to Cloudflare D1 — there is no local database — and the schema is applied by `EnsureSchema` at startup. Test and production run the same binary as a Cloudflare Container (`Dockerfile`) fronted by `worker/index.js`, selected by `wrangler.jsonc` (production) and `wrangler.test.jsonc` (test). Local development points at the **test** D1 database and test R2 bucket; only the Go unit tests use in-memory SQLite. See `CLOUDFLARE.md`.

## Build, Test, and Development Commands

- `go run ./cmd/server`: start the API locally on `:4000` against the test D1 database.
- `go run ./cmd/server -addr=":8080"`: run on a different port.
- `go test ./...`: run all Go tests (in-memory SQLite, no C toolchain needed).
- `go test ./internal/d1/`: run the D1 driver tests only.
- `go build ./cmd/server`: compile the server package.
- `npx wrangler deploy`: build and deploy the Cloudflare Container and Worker (see `CLOUDFLARE.md`).
- `npx wrangler deploy -c wrangler.test.jsonc`: build and deploy the test Worker (`thom-server-test`).

For local authenticated routes, keep the **non-secret** values in `.env.local` (copy `.env.example`) and run the server with `scripts/dev-op.ps1`, which injects secrets from 1Password (see `README.md`). `go run ./cmd/server` loads `.env.local` before reading configuration, while existing shell or container variables take precedence. The server uses Cloudflare D1 and refuses to start unless `D1_ACCOUNT_ID`, `D1_DATABASE_ID`, and `CF_API_TOKEN` are all set — there is no local SQLite fallback. Because `modernc.org/sqlite` is a pure-Go driver, the in-memory unit tests need no C toolchain; the D1 path does not either.

## Coding Style & Naming Conventions

Use standard Go formatting: run `gofmt` on changed `.go` files before committing. Keep package names short and lowercase. Export only types or methods used across packages, such as `coffee.Model` and `server.App`; keep route handlers descriptive and action-oriented, such as `CreateEntry`. Prefer small helpers in focused `internal/server` packages when behavior is shared by handlers.

## Testing Guidelines

Place tests next to the code they cover using Go's standard `testing` package and name files `*_test.go`. Prefer table-driven tests for handlers, auth/config helpers, and model methods. For database behavior, use in-memory SQLite databases and the existing test helper patterns; the D1 driver tests use a fake HTTP endpoint and need no CGO. Run `go test ./...` before opening a PR.

## Commit & Pull Request Guidelines

Recent commits use short, lowercase, imperative summaries, for example `add logging, fix db schema, add logging, and cleanups` and `update routes, data base, and overall qol`. Keep commits focused and mention the affected area when helpful.

Pull requests should include a brief description, the routes or models changed, how the change was tested, and any deployment/configuration impact. For API behavior changes, include an example request such as `curl localhost:4000/coffee` and summarize the expected response shape. Mention auth impact when changing protected routes, cookie behavior, or CORS origins.

## Security & Configuration Tips

Do not commit secrets, real `.env.local`/`.dev.vars` values, JWT secrets, or production password hashes. Cloudflare deployments use Worker secrets (`ADMIN_USERNAME`, `ADMIN_PASSWORD_HASH`, `JWT_SECRET`, `CF_API_TOKEN`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `EMAIL_API_TOKEN`) and vars (`CLIENT_ORIGIN_URLS`, `SECURE_COOKIES`, `D1_ACCOUNT_ID`, `D1_DATABASE_ID`, `R2_ACCOUNT_ID`, `R2_BUCKET`, `R2_PUBLIC_BASE_URL`, `STRIPE_TAX_ENABLED`, `STRIPE_SHIPPING_CENTS`, `EMAIL_ACCOUNT_ID`, `EMAIL_FROM`, `EMAIL_FROM_NAME`, `PUBLIC_SITE_URL`); keep `.env.example` and `.dev.vars.example` placeholder-only. `CF_API_TOKEN` is a D1 API token scoped to the account and must never be committed. Set `SECURE_COOKIES=true` for HTTPS environments (this also switches the auth cookie to the `__Host-` prefixed name; the cookie is always `SameSite=Lax` because the API is same-origin with the site). `JWT_SECRET` must be at least 32 characters or the server refuses to start. Login rate limiting and logout token revocation live in the app database (`internal/authstate`), so they survive restarts; only the in-memory fallback used by tests resets. `internal/thom.db` is a legacy, gitignored SQLite file the server no longer reads; local runs now mutate the **test** D1 database, so avoid running them with production credentials.

**Local secret files.** `.env*` and `.dev.vars*` (except the `.example` files) are gitignored and denied to the `read` tool via global OpenCode permissions. Do not work around that with `grep` or shell commands — a `grep '^CF_API_TOKEN=.+' .env*` prints the secret into the transcript. Read the `.example` files for the shape of the config instead.

**Local secrets use 1Password.** `.env.op` is a committed, non-secret list of `op://` references; `scripts/dev-op.ps1` runs `op run`, which resolves them into the server process only. The desktop app's developer integration plus Windows Hello is the human-unlock boundary — keep auto-lock short. This is a guardrail, not OS-level isolation: while the vault is unlocked, `shell` can still reach anything readable. `.env.local` holds only non-secrets. Never print resolved values into the transcript.

**Wrangler loads `.env`/`.env.local` from its working directory.** A `CF_API_TOKEN` there is picked up as wrangler's own API token, shadowing the OAuth login and causing `403 No access to the specified resource` on `wrangler secret`/`deploy`. Run wrangler from a directory without those files, or unset the variable for the command.

## Outstanding work — deployment & shop (2026-09-18)

The shop (listings, R2 image uploads), Stripe backend, grinder/brand lookups,
indexes, client-side filters, and the admin orders UI are committed
(`main` == `origin/main` == `3ff274d`). Live deploy state and the running task
list live in `D:\Repos\BOARD.md`; this section only records durable gotchas.

- **Two environments, one repo.** Production is `wrangler.jsonc` →
  `thom-server`; test is `wrangler.test.jsonc` → `thom-server-test`. Secrets are
  per-Worker, so a value set for one is absent on the other. See `CLOUDFLARE.md`.
- **Known shop follow-ups:** only `checkout.session.completed` is handled (async
  payment methods leave orders `pending`). Checkout now collects a **US-only**
  shipping address (`CollectedInformation.ShippingDetails`), and an oversold
  webhook is voided if the PaymentIntent is uncaptured, otherwise refunded, with
  the order moved to `refunded`.
- **Orders API (2026-09-18).** `GET /shop/orders` is paginated:
  `?limit=<1..100, default 20>&cursor=<last order id>`, response
  `{ "orders": [...], "nextCursor": "<id>|" }` (`""` means no more pages).
  `cursor` is the keyset of the previous page, not an offset. This replaced the
  old bare-array response, so the website admin page must send/consume the
  wrapper. Order `status` is one of `pending`, `paid`, `refund_pending`,
  `refunded`. `ShopOrders` carries structured `ShipName`/`ShipLine1`/`ShipLine2`/
  `ShipCity`/`ShipState`/`ShipPostalCode`/`ShipCountry` (plus the legacy formatted
  `ShippingAddress`), and `RefundedAt`/`RefundReason`; `ensureShopOrderColumns`
  adds them to existing databases. A `refund_pending` order means a refund was
  requested but Stripe had not confirmed it; Stripe webhook redelivery retries
  it, and a `refund_pending` order with no payment intent is left for an operator.
- **`GET /shop/orders/{sessionId}` is public and returns no personal data.** It
  serves the buyer's confirmation page, keyed only on the Stripe session id, so
  it returns status/total/lines and omits customer name, email, and every
  shipping field (`publicOrder` in `internal/server/shop/orders.go`). The session
  id can leak via browser history, shared links, or logs, so it is not sufficient
  authorization for PII. The full order (with PII) is only returned by the
  authenticated `GET /shop/orders` admin list.
- **Buyer order-view links (magic link) — code done, infra pending.** On
  `checkout.session.completed` the webhook mints a random 32-byte token, stores
  **only its SHA-256 hash** (`ShopOrders.ViewTokenHash`, added by
  `ensureShopOrderColumns`), and emails
  `<PUBLIC_SITE_URL>/shop/order/view?token=<raw>` via Cloudflare Email Service.
  `GET /shop/orders/view/{token}` returns the **full order** (customer + ship
  fields) because the token is the credential; it is `no-store`, `no-referrer`,
  and `noindex`. The raw token exists only in the email. Mail is best-effort:
  a send failure is logged and the webhook still returns `200` (the payment is
  already recorded). **No backfill** — orders paid before this change have no
  token. **No resend** — a failed send is not retried. Prerequisite before it
  works live: onboard `thomhuang.com` for Email Sending (adds SPF/DKIM) and set
  the `EMAIL_API_TOKEN` Worker secret on both Workers; order email is silently
  disabled until `EMAIL_ACCOUNT_ID`, `EMAIL_FROM`, and the token are all set.
- **`POST /shop/checkout` is rate-limited per client** (10 per 10 minutes,
  fixed window) by `internal/server/ratelimit.go`, keyed on `CF-Connecting-IP`.
  The limiter is in-memory, so it resets when the container restarts and is not
  shared across instances; that is acceptable for the single-instance
  deployment. The Stripe webhook is deliberately unlimited — it is
  signature-verified and Stripe retries failed deliveries.
- **Image upload size is deliberately not server-enforced.** Presign is
  admin-only, so the browser-side check is an accidental-oversize guard, not a
  security control. R2 offers no pre-upload size gate (no POST form policies, no
  per-bucket object cap, and the S3 endpoint is outside our zone), and signing
  `content-length` into the presigned PUT is unverified against R2. Accepted
  risk, not an oversight — revisit only if uploads stop being admin-only.
- **The browser compresses images before upload** (`ThomWeb/src/Pages/Shop/
  imageUpload.ts`): downscale to 2000px and re-encode to WebP when that shrinks
  the file, otherwise upload the original. The 10 MB limit applies to the
  prepared bytes; the 50 MB source ceiling only guards against decode hangs.
  Because compression can change the type, the server must sign the *prepared*
  content type. GIFs are passed through untouched to preserve animation.
- **Garment measurements (inches) — backend done, UI pending.** `ShopItems` has
  three optional columns, all `REAL NOT NULL DEFAULT 0`:
  `PitToPitInches`, `BackLengthInches`, `ShoulderInches`. `0` means "not
  provided" — a non-clothing listing simply leaves them at zero, and the API
  always emits all three. API field names are camelCase:
  `pitToPitInches`, `backLengthInches`, `shoulderInches`. They are accepted on
  `POST /shop/items` (all three optional) and `PATCH /shop/items/{id}` (omitted
  leaves the stored value untouched). Values are rounded to **one decimal**
  server-side (24.46 → 24.5), so the UI should display one decimal; accepted
  range is 0–100 inches, and negative/NaN/Inf are rejected with 400. Storage is
  inches; the UI is expected to convert to cm for display. `GetItems` summaries
  do **not** include measurements — only `GET /shop/items/{id}` does.
- **`go test ./...` fails under Windows Smart App Control** ("An Application
  Control policy has blocked this file") because test binaries in the temp build
  dir are blocked. Workaround: `go test -c -o
  "$env:LOCALAPPDATA\Temp\opencode\<pkg>.exe" <pkg>` and run that exe directly.

## Garment measurements reference (2026-09-18)

| Column (D1) | JSON field | Meaning |
|---|---|---|
| `PitToPitInches` | `pitToPitInches` | Chest width, seam to seam |
| `BackLengthInches` | `backLengthInches` | Collar seam to hem, down the back |
| `ShoulderInches` | `shoulderInches` | Shoulder seam to shoulder seam |

All three are `REAL NOT NULL DEFAULT 0`; `0` means "not provided", so a
non-clothing listing leaves them at zero. Added to existing databases by
`ensureShopItemColumns` in `internal/shop/shop.go` via `ALTER TABLE ADD COLUMN`
guards — no migration file, existing rows default to `0`.
