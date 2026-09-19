# Operator checklist — thom-server

Action items for the human operator. Agents work from `AGENTS.md`; this file is
what the owner still needs to do. Check items off as they are done and add a
dated note when something changes.

## This repo (uncommitted work)

- [x] Commit the pending working-tree changes: the flat $10 shipping default
      (`STRIPE_SHIPPING_CENTS=1000` in `wrangler.jsonc`, `wrangler.test.jsonc`,
      `.env.example`, `.dev.vars.example`, and `defaultShippingCents` in
      `internal/server/config.go`) and the login diagnostics
      (`authConfigIssue`/`credentialMatch` logging in
      `internal/server/auth`). Both are covered by tests.
      (2026-09-19: committed as `459718f` and `04cf1ce`.)
- [x] Delete `SERVER_HANDOFF.md` — items A (worker email var forwarding) and B
      (cache headers, D1 round-trip collapse) are implemented and committed.
      (2026-09-19: deleted from disk; never committed.)
- [ ] Refresh `~/Repos/STATUS.md` — it still says `thom-server` main is
      `c93ab1a` and clean; it is `2e092ff` with pending changes.

## Cloudflare / Stripe / email

- [ ] Onboard `thomhuang.com` for Email Sending (dashboard: Compute & AI →
      Email Service → Email Sending → Onboard Domain; adds SPF/DKIM). Until
      then, buyer order emails only reach verified destination addresses.
- [ ] Set `EMAIL_API_TOKEN` and `ORDER_NOTIFICATION_EMAIL` Worker secrets on
      **both** `thom-server` and `thom-server-test` (secrets are per-Worker).
- [ ] Verify the remaining secrets on both Workers: `ADMIN_USERNAME`,
      `ADMIN_PASSWORD_HASH`, `JWT_SECRET`, `CF_API_TOKEN`, `R2_ACCESS_KEY_ID`,
      `R2_SECRET_ACCESS_KEY`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`
      (see `CLOUDFLARE.md`).
- [ ] Fix the production Workers Builds trigger (pushes currently produce no
      builds, so the deployed `thom-server` can lag `main`) or keep deploying
      manually with `npx wrangler deploy`.
- [ ] Verify the Stripe webhook endpoint (`/shop/webhooks/stripe`) subscribes
      to `checkout.session.completed` in both live and test mode with the
      matching signing secret.

## Website repo (thom-website)

- [ ] Promote `origin/main` → `origin/release` when the shop is ready
      (`release` was 1 commit behind `main` as of 2026-09-19).

## Agreed but not started

- [ ] Stock reservation at checkout (agreed 2026-09-19): reserve stock when the
      Checkout Session is created (conditional decrement, loser gets 409),
      30-minute hold via `ExpiresAt`, release on `checkout.session.expired`,
      `StockReserved` column so pre-existing pending orders keep the legacy
      decrement-at-webhook path. Requires enabling
      `checkout.session.expired` in the Stripe webhook subscription and a small
      admin-UI change for the new `expired` order status.
