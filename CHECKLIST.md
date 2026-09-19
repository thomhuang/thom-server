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
- [x] Refresh `~/Repos/STATUS.md` — it still says `thom-server` main is
      `c93ab1a` and clean; it is `2e092ff` with pending changes.
      (2026-09-19: refreshed to `b0bb547` / `d208795`, both pushed.)

## Cloudflare / Stripe / email

- [x] Onboard `thomhuang.com` for Email Sending (dashboard: Compute & AI →
      Email Service → Email Sending → Onboard Domain; adds SPF/DKIM). Until
      then, buyer order emails only reach verified destination addresses.
      (2026-09-19: already done 2026-09-18 — zone enabled with DKIM
      `cf-bounce`, bounce MX, DMARC reject; the dashboard "subdomain already
      exists" error was the onboarding flow refusing an already-onboarded
      domain.)
- [x] Set `ORDER_NOTIFICATION_EMAIL` on production `thom-server`
      (2026-09-19: set by the operator.)
- [ ] Verify the remaining secrets on both Workers: `ADMIN_USERNAME`,
      `ADMIN_PASSWORD_HASH`, `JWT_SECRET`, `CF_API_TOKEN`, `R2_ACCESS_KEY_ID`,
      `R2_SECRET_ACCESS_KEY`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`
      (see `CLOUDFLARE.md`).
- [ ] Fix the production Workers Builds trigger (pushes currently produce no
      builds, so the deployed `thom-server` can lag `main`) or keep deploying
      manually with `npx wrangler deploy`.
- [ ] Verify the Stripe webhook endpoint (`/shop/webhooks/stripe`) subscribes
      to `checkout.session.completed` in both live and test mode with the
      matching signing secret. (2026-09-19: sandbox test-mode endpoints for
      `thom-server` and `thom-server-test` subscribe to
      `checkout.session.completed` **and** `checkout.session.expired`; the
      live endpoint was created in Workbench with both events. Remaining:
      confirm its signing secret matches the production
      `STRIPE_WEBHOOK_SECRET` Worker secret.)

## Website repo (thom-website)

- [x] Promote `origin/main` → `origin/release` when the shop is ready
      (2026-09-19: fast-forwarded `535b89f..d208795`; local `release` branch
      updated too.)

## Stock reservation

- [x] Stock reservation at checkout (agreed 2026-09-19): reserve stock when the
      Checkout Session is created (conditional decrement, loser gets 409),
      30-minute hold via `ExpiresAt`, release on `checkout.session.expired`,
      `StockReserved` column so pre-existing pending orders keep the legacy
      decrement-at-webhook path. (2026-09-19: committed in thom-server
      `2073951` and thom-website `55795f8`, both pushed. Remaining operator
      steps: deploy both repos, confirm the live webhook signing secret
      matches `STRIPE_WEBHOOK_SECRET` — see the webhook item above.)
