# Kestrel

A concurrent uptime monitor written in Go, with an MCP layer so an AI agent
can query status and manage monitored targets directly.

> **Status:** early stage. The scaffold (module, database, migrations) is in
> place; the checker engine, HTTP API, webhooks, and MCP server are still
> being built. This README will grow with the project — see below for what's
> done.

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

**Planned (see [PLAN.md](PLAN.md), kept local/untracked):**
- Concurrent checker engine with a bounded worker pool and per-target timeout
- Incident state machine with flap prevention (N consecutive failures/successes)
- REST API + minimal server-rendered status page
- Webhook notifications on state transitions, with retry/backoff
- MCP server exposing read (status, history, uptime %) and write
  (add/remove/update target) tools, API-key protected

## Stack

- **Language:** Go, stdlib `net/http` — no web framework
- **Database:** PostgreSQL via [`pgx`](https://github.com/jackc/pgx)
- **Migrations:** [`goose`](https://github.com/pressly/goose), embedded in the binary
- **MCP:** official Go MCP SDK (or `mark3labs/mcp-go` as a fallback)
- **Deployment:** Docker + Fly.io

## Local development

Requires Go 1.23+ and Docker.

```bash
make up           # start Postgres in Docker
make migrate-up    # apply migrations
make run           # run the service
```

Other targets: `make down`, `make migrate-down`, `make test`.

By default the service expects Postgres reachable at
`postgres://kestrel:kestrel@localhost:55432/kestrel` (see
[docker-compose.yml](docker-compose.yml) — port `55432` is used on the host
to avoid clashing with a locally installed Postgres on `5432`). Override with
the `DATABASE_URL` environment variable.

## Deployment

Not yet set up — tracked as a milestone in progress.
