# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make run          # API server (go run ./cmd/api -f etc/taskpilot-api.yaml)
make run-worker   # parse worker, needs TASKPILOT_AI_API_KEY exported
make build        # both binaries into bin/
make test         # go test ./...
make fmt          # go fmt ./...
make tidy         # go mod tidy
```

Run `make fmt`, `make test`, `make tidy` before committing (repo convention from README).

Local setup: `cp etc/taskpilot-api.example.yaml etc/taskpilot-api.yaml`, `docker compose up -d` (PostgreSQL 16 + Redis 7), then `make migrate`.

Single test / package:

```bash
go test ./internal/logic/project/ -run TestCreate -v
go test ./pkg/ai/ -v
```

Integration tests in `internal/logic/integration_test.go` skip unless `TASKPILOT_TEST_DATABASE_DSN` is set. When set, each test creates a throwaway PostgreSQL schema, applies `scripts/migrate.sql` statement-by-statement, and drops it on cleanup:

```bash
TASKPILOT_TEST_DATABASE_DSN='postgres://taskpilot:taskpilot@localhost:5432/taskpilot?sslmode=disable' \
  go test ./internal/logic/ -run TestIntegration -v
```

Migrations are plain SQL under `scripts/`, applied via `docker compose exec postgres psql`. `scripts/migrate.sql` is the full schema for a fresh database; the `migrate-*` Make targets are incremental upgrades for existing databases. **When adding a new incremental migration, also wire it into `scripts/deploy_prod.sh` and `scripts/deploy_dev.sh`** — otherwise the SQL lands in the repo but never runs on a server. All current incremental migrations, including email normalization, are wired into both deployment scripts and fail closed on conflicting legacy data.

## Architecture

Two binaries share one config file, one `ServiceContext`, and one Docker image; they differ only in entrypoint.

- `cmd/api` — Gin HTTP server. Explicit server timeouts, graceful shutdown on SIGINT/SIGTERM.
- `cmd/worker` — Redis Streams consumer that calls the AI API. Builds `ai.NewResponsesParser` itself and assigns it onto the `ServiceContext` (the API process leaves `Parser` nil).

Request flow is a fixed four-layer chain:

```
internal/handler/<domain>/   HTTP binding, principal extraction, status codes
internal/logic/<domain>/     business rules, transactions, all validation
model/<x>model/              Gorm structs
internal/types/              request/response DTOs
```

`internal/svc.ServiceContext` is the single dependency container (`Config`, `DB`, `JWT`, `Redis`, `RefreshSessions`, `ParseJobs`, `Parser`, `Logger`). It degrades rather than fails: a bad DSN or empty `Cache.Host` logs a warning and leaves the field nil, so every logic method starts with a `requireDB()`-style nil check returning `logicerrors.ErrDatabaseUnavailable`. Tests exploit this by constructing `&svc.ServiceContext{JWT: ...}` with nothing else.

### Async parse pipeline

This is the core of the system and the part worth understanding before touching anything nearby.

1. `POST /api/v1/parse-jobs` inserts a `pending` row inside a transaction that `SELECT ... FOR UPDATE`s the document, then publishes to Redis Streams **outside** the transaction. Publish failure is logged, not returned.
2. `internal/worker/parsejob` runs N `consumeLoop` goroutines plus four maintenance loops:
   - `reclaimLoop` — `XAUTOCLAIM` idle messages, and reset DB rows stuck in `processing` past `LeaseTimeout` back to `pending` (up to `MaxRecoveries`, then fail them).
   - `reconcileLoop` — scan PostgreSQL for `pending` rows older than `PendingGrace` and republish. **This is what makes a dropped publish self-healing; PostgreSQL is the source of truth, Redis is only a wakeup.**
   - `heartbeatLoop` — SET a TTL key used by `/healthz` status reporting and strict `/readyz` readiness.
   - `trimLoop` — `XTRIM` past `StreamRetention`.
3. State transitions are compare-and-swap `UPDATE ... WHERE status = ?` and only count as done when `rowsAffected == 1`. Messages are acked only on a terminal state (`success`/`failed`), so a crash mid-parse leaves the message pending for reclaim.
4. `pkg/ai` calls SoruxGPT `/responses` with `text.format.type = json_schema`, `strict: true`. The response is decoded with `DisallowUnknownFields` and then re-validated in Go (`normalizeAndValidate`) — the schema is not trusted on its own. One retry, only for retryable failures (5xx, 429, transport, malformed output). `PublicErrorMessage` maps errors to user-safe strings stored in `parse_jobs.error_message`; raw errors stay in logs.

Document text is treated as untrusted data — the parser instructions explicitly tell the model to ignore instructions inside the `<document>` block.

### Idempotency and concurrency contracts

These are enforced in both application logic and database constraints; changing one without the other breaks the guarantee.

- One active parse job per document — `uq_parse_jobs_active_document` partial unique index on `status IN ('pending','processing')`, plus a counting check under `FOR UPDATE`. Concurrent creates: one 201, the rest `ErrConflict`.
- One project per parse result — `uq_projects_parse_result_id`. `POST /projects` returns 201 on first create and **200** with the existing project and tasks on repeat or concurrent calls; `gorm.ErrDuplicatedKey` is caught and turned into the lookup-and-return path. Project plus its initial tasks are created in one transaction.
- Parse results use a `version` column for optimistic locking while unconfirmed, become immutable once `is_confirmed`, and confirm is idempotent.
- Documents are soft-deleted (`gorm.DeletedAt`); delete conflicts if an active parse job exists, and derived projects/tasks survive via `ON DELETE RESTRICT` on `projects.parse_result_id`.

Every query is scoped `WHERE id = ? AND user_id = ?`. There is no shared-ownership model — a missing row and someone else's row both surface as `ErrNotFound`.

### Errors and responses

All responses use `pkg/response.Envelope` — `{code, message, data}`, `code: 0` on success, `data: null` on error. Business codes live in `pkg/errors/code.go` (10001–10014) and are documented in `docs/openapi.yaml`.

Logic layers return sentinel errors from `internal/logic/errors.go` (`ErrInvalidInput`, `ErrNotFound`, `ErrConflict`, `ErrInvalidState`, `ErrDatabaseUnavailable`, `ErrCacheUnavailable`), optionally wrapped with `fmt.Errorf("%w: detail", ...)`. `internal/handler/common.WriteError` is the single place that maps them to HTTP status + business code; the `default` branch logs and returns a generic 500 so internal messages never leak. Handlers should not build error responses themselves.

### Auth

Access tokens are hand-rolled HS256 JWTs (`pkg/auth/jwt.go`, no JWT library). Refresh tokens are `sessionID.secret`, stored in Redis as a SHA-256 hash and rotated through a Lua script that atomically detects reuse (`ErrRefreshTokenReused`) and deletes the session.

Two auth transports, one middleware: `RequireAuth` prefers the `Authorization: Bearer` header and falls back to the `access_token` cookie, recording which was used. `RequireCSRFForCookieAuth` then demands a `X-CSRF-Token` header matching the `csrf_token` cookie (constant-time compare) **only for cookie-authenticated unsafe methods** — Bearer callers are exempt. `/auth/refresh`, `/auth/logout`, and `PUT /users/me` are cookie-and-CSRF only; the refresh cookie is scoped to `/api/v1`. Profile updates atomically rotate the current device session. Register/login use a Redis sliding-window limiter and fail closed when Redis is unavailable.

`Secure(CookieSecure)` trusts `X-Forwarded-Proto` for HTTPS detection and 308-redirects plain HTTP, so a reverse proxy must set that header in production. `/healthz` and `/readyz` are registered **before** the `Secure` middleware, so they are always reachable over plain HTTP from inside the container (Compose healthchecks and deployment scripts rely on this); only `/api/v1` business routes are HTTPS-gated.

## Configuration

YAML is the base (`etc/taskpilot-api.yaml`, gitignored; `.example.yaml` variants are committed), with `TASKPILOT_*` environment variables overriding every field via `applyEnvOverrides`. Note the failure mode: an unparseable numeric env var silently falls back to the YAML value rather than erroring, so verify runtime config through logs, `/healthz`, and `/readyz` after a deploy. Defaults and cross-field validation (e.g. `HeartbeatTTL > HeartbeatInterval`) live in `config.Load`.

Nothing auto-loads `.env` — neither the Go process nor the Makefile. Source it explicitly (`set -a; . ./.env; set +a`).

Production splits secrets: `.env.prod` for app/database/auth, `.env.worker.prod` for `TASKPILOT_AI_API_KEY` and AI settings.

## Deployment

Single remote server, Docker Compose. `app` binds `127.0.0.1:8888` behind a reverse proxy; `postgres` and `redis` stay on the internal network. The retained `scripts/deploy_dev.sh` validates the AI key, waits for PostgreSQL, runs migrations, rebuilds the shared image, rolls both containers, and waits for strict `/readyz`. GitHub Actions runs unit tests, PostgreSQL integration tests, and a real API/Worker process smoke test before deploying the renamed `main` branch to the only remote environment. Details in `docs/deployment.md`.

The dev environment reuses the production `taskpilot-postgres` container over an external network but **must** point at the separate `taskpilot_dev` database.

## Current scope

Implemented: auth including profile update/session rotation and register/login rate limiting; text/PDF documents and extraction; parse jobs/results; project/task CRUD, status and ordering; history; liveness/readiness; deployment migrations and CI process smoke tests. Parse-status caching remains intentionally deferred until measurements justify it.

`docs/openapi.yaml` is the API contract and is kept in sync by hand — update it when adding or changing an endpoint.
