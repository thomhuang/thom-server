# Operator checklist — thom-server

Action items for the human operator. Agents work from `AGENTS.md`; this file is
what the owner still needs to do. Check items off as they are done and add a
dated note when something changes.

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
- [ ] Production deploys run from the `release` branch (Workers Builds), so
      shipping means promoting `main` → `release` (`git push origin main:release`)
      or deploying manually with `npx wrangler deploy`. Confirm the build for
      `27547c3` succeeded under Workers & Pages → thom-server → Builds.
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
      steps: ship via the `release` branch (the production branch) or a manual
      deploy, and confirm the live webhook signing secret matches
      `STRIPE_WEBHOOK_SECRET` — see the webhook item above.)
