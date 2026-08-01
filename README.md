# Kestrel

A concurrent uptime monitor written in Go, with an MCP layer so an AI agent
can query status and manage monitored targets directly.

> **Status:** early stage. The scaffold, checker engine, incident state
> machine, HTTP API/status page, webhook notifications, and MCP server are
> all in place; deployment is next. This README will grow with the project —
> see below for what's done.

## Why this project exists

Two things this project is built to demonstrate:

1. **Idiomatic concurrent Go.** The checker engine is a hand-rolled worker
   pool — goroutines, channels, `context` cancellation, backpressure — not a
   sequential loop that happens to be written in Go.
2. **An MCP server with real read/write access.** An agent can ask "what's
   been flaky this week" or register a new target to watch, not just
   consume a summary someone else generated — see [MCP](#mcp) below.

The reasoning behind the two hardest parts — the checker engine's
concurrency design and the incident state machine's flap prevention — is
written up in [DECISIONS.md](DECISIONS.md).

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
- MCP server (`/mcp`, streamable HTTP) exposing the same read/write
  operations as the REST API — `list_targets`, `list_checks`,
  `list_incidents`, `get_uptime`, `create_target`, `update_target`,
  `delete_target` — as tools an agent can call, API-key protected
- Dockerfile (multi-stage, ~15MB Alpine runtime image) and fly.toml,
  verified locally — builds, migrates, serves, and shuts down cleanly on
  SIGTERM inside the container

**Planned (see [PLAN.md](PLAN.md), kept local/untracked):**
- A live deployed instance (infrastructure not yet provisioned)

## Stack

- **Language:** Go, stdlib `net/http` — no web framework
- **Database:** PostgreSQL via [`pgx`](https://github.com/jackc/pgx)
- **Migrations:** [`goose`](https://github.com/pressly/goose), embedded in the binary
- **MCP:** [official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk)
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

## MCP

`POST /mcp` speaks the MCP streamable HTTP transport and requires
`Authorization: Bearer <token>`. Mint a token with:

```bash
go run ./cmd/mcptoken
```

The raw token is printed once and never stored — save it immediately.
Tools available: `list_targets`, `list_checks`, `list_incidents`,
`get_uptime`, `create_target`, `update_target`, `delete_target`. Every tool
is a thin wrapper over the same `internal/store` methods the REST API uses,
so an agent and a human operator always see identical state.

## Local development

Requires Go 1.25+ (or an older Go with `GOTOOLCHAIN=auto`, which will
fetch the pinned version) and Docker.

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

Docker + [Fly.io](https://fly.io). The Dockerfile is a multi-stage build
producing a small Alpine image containing all three binaries
(`kestrel`, `migrate`, `mcptoken`); `fly.toml` runs `migrate -direction up`
as the `release_command` before every deploy, so the schema is always
current before new code starts serving traffic.

Build and run locally:

```bash
docker build -t kestrel .
docker run --rm -e DATABASE_URL=... -p 8080:8080 kestrel
```

To deploy (not yet done — infrastructure isn't provisioned):

```bash
fly launch --no-deploy         # first time only, creates the app
fly postgres create            # or attach an existing cluster
fly secrets set DATABASE_URL=... WEBHOOK_URL=...
fly deploy
```

`DATABASE_URL` and `WEBHOOK_URL` are set via `fly secrets`, never committed —
`fly.toml` only holds non-sensitive config (`HTTP_ADDR`, the release
command, VM size).
