# Kestrel

A concurrent uptime monitor written in Go, with an MCP layer so an AI agent
can query status and manage monitored targets directly.

> **Status:** early stage. The scaffold, checker engine, incident state
> machine, HTTP API/status page, and webhook notifications are in place; the
> MCP server is still being built. This README will grow with the project —
> see below for what's done.

## Why this project exists

Two things this project is built to demonstrate:

1. **Idiomatic concurrent Go.** The checker engine is a hand-rolled worker
   pool — goroutines, channels, `context` cancellation, backpressure — not a
   sequential loop that happens to be written in Go.
2. **An MCP server with real read/write access.** An agent should be able to
   ask "what's been flaky this week" or register a new target to watch, not
   just consume a summary someone else generated.

## Features

**Done:**
- Postgres schema for targets, checks, incidents, and MCP tokens
- Embedded migrations (`cmd/migrate`), runnable identically in dev and in
  the deployed binary
- Concurrent checker engine: a scheduler dispatches due targets into a
  bounded worker pool over an unbuffered channel (so a spike of due targets
  can't open unbounded concurrent checks), each check runs under its own
  `context.WithTimeout` so one slow target can't hold up the others, and a
  single writer goroutine drains results so nothing writes to Postgres
  concurrently
- Incident state machine with flap prevention (a target only trips DOWN
  after N consecutive failures, and recovers only after N consecutive
  successes), implemented as a pure function and covered by table-driven
  tests including the alternating pass/fail edge case
- REST API for targets (CRUD), checks, incidents, and uptime %, plus a
  minimal server-rendered status page (target list + incident timeline, no
  JS) — both are thin wrappers over `internal/store`, so the same methods
  will back the MCP tools later rather than duplicating the logic
- Webhook notifications on DOWN and recovery, POSTed as JSON with retry and
  exponential backoff on delivery failure; delivery runs in its own
  goroutine so a slow or unreachable webhook endpoint never blocks the
  checker engine's result writer

**Planned (see [PLAN.md](PLAN.md), kept local/untracked):**
- MCP server exposing read (status, history, uptime %) and write
  (add/remove/update target) tools, API-key protected

## Stack

- **Language:** Go, stdlib `net/http` — no web framework
- **Database:** PostgreSQL via [`pgx`](https://github.com/jackc/pgx)
- **Migrations:** [`goose`](https://github.com/pressly/goose), embedded in the binary
- **MCP:** official Go MCP SDK (or `mark3labs/mcp-go` as a fallback)
- **Deployment:** Docker + Fly.io

## API

| Method | Path | |
|---|---|---|
| GET | `/api/targets` | list targets with current status |
| POST | `/api/targets` | create a target |
| GET | `/api/targets/{id}` | get a target |
| PUT | `/api/targets/{id}` | update a target |
| DELETE | `/api/targets/{id}` | delete a target |
| GET | `/api/targets/{id}/checks` | recent check history |
| GET | `/api/targets/{id}/incidents` | a target's incidents |
| GET | `/api/targets/{id}/uptime?window_hours=24` | uptime % over a window |
| GET | `/api/incidents` | incident timeline across all targets |

Plus `GET /` (target list) and `GET /incidents` (incident timeline) as
server-rendered HTML.

## Local development

Requires Go 1.23+ and Docker.

```bash
make up           # start Postgres in Docker
make migrate-up    # apply migrations
make run           # run the service
```

Other targets: `make down`, `make migrate-down`, `make test`,
`make test-integration` (spins up a real Postgres container via
testcontainers-go to verify the store's SQL and incident-transition logic;
requires Docker).

By default the service expects Postgres reachable at
`postgres://kestrel:kestrel@localhost:55432/kestrel` (see
[docker-compose.yml](docker-compose.yml) — port `55432` is used on the host
to avoid clashing with a locally installed Postgres on `5432`). Override with
the `DATABASE_URL` environment variable.

Set `WEBHOOK_URL` to enable webhook notifications on DOWN/recovery; leave it
unset to disable them entirely. `HTTP_ADDR` overrides the API/status page
listen address (default `:8080`).

## Deployment

Not yet set up — tracked as a milestone in progress.
