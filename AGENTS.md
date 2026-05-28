# Repository Guidelines

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

This is a small Go 1.22 HTTP API server. The executable entrypoint lives in `cmd/server/main.go`; HTTP app wiring lives in `internal/server`. Shared server infrastructure is in `internal/server` and `internal/server/response`, auth HTTP behavior is in `internal/server/auth`, and coffee HTTP handlers are in `internal/server/coffee`. Domain and data-access code lives in `internal/coffee`, with shared data-layer errors in `internal/data`. The SQLite database used by local runs is `internal/thom.db`; treat it as application data, not a test fixture. Root-level deployment files include `Dockerfile`, `docker-compose.yaml`, `fly.toml`, and the Fly.io GitHub Actions workflow in `.github/workflows/fly-deploy.yml`.

## Build, Test, and Development Commands

- `go run ./cmd/server`: start the API locally on `:4000`.
- `go run ./cmd/server -addr=":8080"`: run on a different port.
- `go run ./cmd/server -db-path="./internal/thom.db"`: run against a specific SQLite database.
- `go test ./...`: run all Go tests.
- `go build ./cmd/server`: compile the server package.
- `docker compose up --build`: build and run the containerized service on port `4000`.
- `flyctl deploy --remote-only`: deploy using the same command as CI.

For local authenticated routes, copy `.env.example` to `.env.local` and replace the placeholder auth values. `go run ./cmd/server` loads `.env.local` before reading configuration, while existing shell or container variables take precedence. Because `github.com/mattn/go-sqlite3` is used, builds need CGO support unless the build is changed to use a pure-Go SQLite driver.

## Coding Style & Naming Conventions

Use standard Go formatting: run `gofmt` on changed `.go` files before committing. Keep package names short and lowercase. Export only types or methods used across packages, such as `coffee.Model` and `server.App`; keep route handlers descriptive and action-oriented, such as `CreateEntry`. Prefer small helpers in focused `internal/server` packages when behavior is shared by handlers.

## Testing Guidelines

Place tests next to the code they cover using Go's standard `testing` package and name files `*_test.go`. Prefer table-driven tests for handlers, auth/config helpers, and model methods. For database behavior, use temporary SQLite databases and the existing test helper patterns instead of mutating `internal/thom.db`. Run `go test ./...` before opening a PR.

## Commit & Pull Request Guidelines

Recent commits use short, lowercase, imperative summaries, for example `docker + fly.io deployment` and `update routes, data base, and overall qol`. Keep commits focused and mention the affected area when helpful.

Pull requests should include a brief description, the routes or models changed, how the change was tested, and any deployment/configuration impact. For API behavior changes, include an example request such as `curl localhost:4000/coffee` and summarize the expected response shape. Mention auth impact when changing protected routes, cookie behavior, or CORS origins.

## Security & Configuration Tips

Do not commit secrets, real `.env.local` values, JWT secrets, or production password hashes. Fly deployments require `FLY_API_TOKEN` in GitHub Actions secrets. Auth configuration uses `ADMIN_USERNAME`, `ADMIN_PASSWORD_HASH`, `JWT_SECRET`, `CLIENT_ORIGIN_URLS`, and `SECURE_COOKIES`; keep `.env.example` placeholder-only. Set `SECURE_COOKIES=true` for HTTPS environments that need cross-site cookies. Treat `internal/thom.db` as application data: review schema/data changes carefully and avoid accidental local-only mutations.
