# Operator checklist — thom-server

Action items for the human operator. Agents work from `AGENTS.md`; this file is
what the owner still needs to do. Check items off as they are done.

## Cloudflare / Stripe / email

- [x] Onboard `thomhuang.com` for Email Sending (SPF/DKIM/DMARC).
- [x] Set `ORDER_NOTIFICATION_EMAIL` on production `thom-server`.
- [ ] Verify remaining secrets on both Workers: `ADMIN_USERNAME`,
      `ADMIN_PASSWORD_HASH`, `JWT_SECRET`, `CF_API_TOKEN`, `R2_ACCESS_KEY_ID`,
      `R2_SECRET_ACCESS_KEY` (Stripe pair verified; production
      `STRIPE_SECRET_KEY` was a test key, now set to live).
- [ ] Ship from `release`: promote `main` → `release` or `npx wrangler deploy`.
- [x] Stripe webhook subscribes to `checkout.session.completed` and
      `checkout.session.expired` in both live and test mode.
- [x] Zone `browser_cache_ttl` = `0` and `cache_level` = `basic`; cache purged.
- [x] Order-email deliverability: Email Routing on `thomhuang.com`, `orders@` /
      `dmarc@` forwarding, DMARC reject.
- [ ] Improve Gmail placement for `orders@thomhuang.com` (Postmaster Tools,
      watch spam score).
- [ ] Decide the buyer email's contact address: keep the hardcoded
      `supportEmail` in `internal/server/shop/email.go` or move to a Worker var.

## Website repo (thom-website)

- [x] Public-API cache fix in `worker/index.js` (always-public paths stay cached
      with the auth cookie).
- [x] Promote `main` → `release` when the shop is ready.

## Stock

- [x] Decrement stock on payment, not at checkout. Reversed the 2026-09-19
      reservation: abandoned checkouts no longer hold stock. Last-unit races are
      guarded by the conditional decrement — the loser is oversold and
      auto-refunded.
