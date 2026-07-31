# Kestrel — concurrent uptime monitor with an MCP layer

(Working name — swap it before you commit if something better comes to mind. Avoid anything too close to Pingdom/UptimeRobot/Checkly/Sentry/Uptime Kuma.)

## Purpose (keep this in view while building)

This is a portfolio project, not just a working app. Two goals drive every decision below, and if a choice would trade one of these away for convenience, don't make it without flagging it first:

1. **Prove idiomatic Go, not "Go-flavored Elixir."** The concurrent checker engine is the centerpiece — goroutines, channels, context cancellation, backpressure. A sequential for-loop that happens to be written in Go defeats the point of the project.
2. **Repeat the Budgeteer signature.** Budgeteer's differentiator was the MCP server — a tool an AI agent can actually query and act on, not just "I used AI to help write this." Kestrel needs the same layer: an agent should be able to ask "what's been flaky this week" or add a new target to watch, with real read/write access.

Everything else (dashboard polish, auth, notification channels) is secondary to those two.

## Stack

- **Language:** Go (modules, no framework crutch — stdlib `net/http` with Go 1.22+ routing is enough)
- **Database:** PostgreSQL — `pgx` for the driver, `goose` or `golang-migrate` for migrations
- **Scheduler / checker engine:** hand-rolled worker pool (see Architecture below) — do not reach for a job queue library, the point is to show this being built, not imported
- **MCP server:** official Go MCP SDK if it covers what's needed (`modelcontextprotocol/go-sdk`); fall back to `mark3labs/mcp-go` if not
- **Testing:** table-driven unit tests for the checker/state machine logic; `testcontainers-go` for Postgres integration tests
- **Deployment:** Docker + Fly.io, matching Budgeteer's deployment pattern for a consistent portfolio story

## Scope

**MVP (build this):**
- Register HTTP(S) targets to monitor (URL, expected status code range, interval, timeout)
- Concurrent checker: worker pool checks all due targets in parallel, respects per-target timeout, doesn't let one slow target block others
- Incident state machine: a target goes DOWN only after N consecutive failed checks, and back UP only after N consecutive successful checks (flapping prevention — this is the actual domain expertise signal, implement it properly, don't hand-wave it)
- Incident history: every state transition persisted with start/end time and duration
- Minimal server-rendered status page (one target list + one incident timeline, no JS framework)
- Webhook notification on state transition (DOWN and recovery)
- MCP server exposing:
  - Read: current status of all targets, incident history, uptime % over a window
  - Write: add/remove/update a monitored target
- API-key protected (single operator — don't rebuild Budgeteer's full auth system, that's already demonstrated elsewhere)

**Stretch (only after MVP is deployed and working):**
- TCP/gRPC checks in addition to HTTP
- Multi-region checking (would be the next concurrency showcase — fan-out checks from multiple goroutine pools tagged by region)
- Public status page per "project" (grouping of targets), closer to what PagerDuty's status pages actually do — be deliberate about this one given the domain overlap, keep it clearly your own design rather than a feature-for-feature copy

**Explicit non-goals (don't build these, they dilute the point of the project):**
- Multi-tenant user accounts / teams
- Email/SMS notification channels (webhook is enough to prove the pattern)
- A JS frontend framework — this project's job is to show backend/systems Go, not frontend work
- Configurable alerting rules engine — a fixed N-consecutive-failures threshold is enough

## Data model (sketch)

- `targets` — id, name, url, expected_status_range, interval_seconds, timeout_ms, consecutive_threshold, created_at
- `checks` — id, target_id, checked_at, success (bool), status_code, latency_ms, error
- `incidents` — id, target_id, started_at, resolved_at (nullable), cause (last error before DOWN)
- `mcp_tokens` — id, token_hash, created_at, last_used_at (mirror Budgeteer's PAT pattern)

## Architecture notes

**Checker engine:** a scheduler goroutine ticks every second, finds targets due for a check (`last_checked_at + interval <= now`), and dispatches them into a bounded worker pool via a channel. Each worker does the HTTP check with `context.WithTimeout`, writes the result, and reports back on a results channel that a single writer goroutine drains into Postgres (avoid concurrent writes fighting over the DB connection). Bound the pool size so a spike of due targets doesn't open unbounded connections. This is the part to spend real design time on — it's the whole point of doing this in Go.

**Incident state machine:** keep it as a pure function of "last N check results for this target" → state, unit-tested in isolation from the DB and the HTTP layer. This makes it trivial to test flapping edge cases (e.g., alternating success/fail should not flip state on every check).

**MCP layer:** thin wrapper over the same service layer the HTTP API uses — don't duplicate business logic between the two. If the read/write handlers underneath both the REST API and the MCP tools are the same functions, that's a good sign of clean layering; if they diverge, that's a signal the design needs another pass.

## Milestones (in order — each should be a working, committed state)

1. **Scaffold:** Go module, Postgres connection, migrations for the four tables above, `docker-compose.yml` for local Postgres, empty `main.go` that connects and exits cleanly
2. **Checker engine + state machine:** worker pool checking targets, results persisted, incident state machine with unit tests covering flap prevention — no HTTP API yet, drive it from a CLI flag or test harness
3. **HTTP API + status page:** REST endpoints for targets/incidents/checks, minimal server-rendered dashboard
4. **Webhook notifications:** fire on DOWN and on recovery, with retry/backoff on delivery failure
5. **MCP server:** read tools first (status, history, uptime %), then write tools (add/remove target), reusing the service layer from step 3
6. **Deployment:** Dockerfile, `fly.toml`, secrets management, deploy a live instance — ideally monitoring a few real endpoints (Budgeteer itself, this account, a couple of public APIs) so the demo isn't empty
7. **Docs:** README (mirror Budgeteer's structure — features, stack, local dev, deployment) plus a short DECISIONS.md capturing the concurrency design and the flap-prevention logic, since those are the two things worth explaining in an interview

## Handoff notes for Claude Code

- Work through the milestones in order; don't jump to the MCP layer before the checker engine and state machine are solid and tested — the whole narrative depends on the concurrency piece being genuinely good, not rushed.
- Flag it explicitly if any milestone would be meaningfully faster with a library that hides the concurrency pattern (e.g., a generic job queue package) — the answer will almost always be "write it by hand," but surface the tradeoff rather than silently picking the shortcut.
- Keep commits small and message them descriptively — a clean commit history is itself part of the portfolio artifact.
