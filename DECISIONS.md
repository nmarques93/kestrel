# Design decisions

The two pieces of this project worth explaining in detail: the concurrent
checker engine and the incident state machine. Everything else is fairly
conventional Go/Postgres/REST plumbing.

## Concurrent checker engine ([internal/checker/engine.go](internal/checker/engine.go))

**The shape:** a scheduler goroutine, a bounded pool of worker goroutines,
and exactly one writer goroutine, connected by two channels.

```
scheduler --jobs(unbuffered)--> [worker]×N --results(unbuffered)--> writer
```

**Why the jobs channel is unbuffered.** Every second, the scheduler asks
the store for due targets and tries to send each one into `jobs`. If all
`Workers` goroutines are busy, that send blocks. This is deliberate
backpressure: a spike of due targets (e.g. after startup, when everything
looks "due" at once) can never open more than `Workers` concurrent HTTP
checks, because there's nowhere for the excess to queue up. A buffered
channel would just move the problem — pick a buffer size and you've
picked a number of targets that can silently pile up before the
scheduler itself starts blocking anyway.

**Why the per-check timeout is `context.WithTimeout`, not a client
timeout.** Each worker wraps the run context in `context.WithTimeout(ctx,
target.TimeoutMS)` before calling the prober. This means a single slow or
hanging target can cost at most one worker for at most its own configured
timeout — it can never block the other `Workers-1` checks in flight, and
it can never hold a worker past its own limit even if the target never
responds at all. The alternative (a shared `http.Client.Timeout`) would
apply the same timeout to every target regardless of what was configured
for it.

**Why there's exactly one writer goroutine.** All workers write their
`Result` to a single `results` channel, and only one goroutine ever reads
from it and calls `Recorder.Record`. This means `store.Record` never has
to worry about concurrent calls for the same (or different) targets
fighting over a `pgxpool` connection or racing on the incident-transition
logic — writes to Postgres are serialized by construction, not by a
mutex or a database-level lock.

**Why "due" targets come from a query, not a `last_checked_at` column on
`targets`.** `DueTargets` finds the most recent row in `checks` per
target via a `LATERAL` join, rather than reading (and the writer having to
update) a `last_checked_at` column on `targets` itself. This means only
one code path ever writes to `checks`/`incidents` — the writer — and the
scheduler's read is always consistent with whatever's actually been
persisted, including across restarts. The tradeoff is one extra join on
every scheduler tick; at the scale this project targets, that's free
(and the `checks(target_id, checked_at desc)` index exists specifically
for this query).

**What this doesn't do.** There's no dynamic worker pool sizing and no
priority between targets — every due target is equally due. Given the
MVP's scale (a handful to a few hundred targets, checked on the order of
seconds to minutes), a fixed pool size configured at startup is enough;
a work-stealing or auto-scaling pool would be solving a problem this
project doesn't have yet.

## Incident state machine ([internal/incident/incident.go](internal/incident/incident.go))

**The rule:** a target only transitions UP→DOWN after N consecutive
failed checks, and DOWN→UP only after N consecutive successful checks,
where N is `consecutive_threshold`. This is what stops a single blip (or
an alternating pass/fail pattern) from opening and closing incidents on
every check.

**Why it's a pure function, not a stateful object.** `Evaluate(current
State, recent []bool, threshold int) Transition` takes the target's
current state and its most recent check results (newest first) and
returns whether that's a transition — no receiver, no hidden state, no
I/O. That's what makes it trivial to write table-driven tests for the
edge case that actually matters here: an alternating
true/false/true/false sequence should never trip a transition, no matter
how long it runs, because no run of `threshold` consecutive identical
results ever appears in the trailing window.

**Why "recent results" comes from a query, not an in-memory counter.**
The obvious alternative — keep a running `consecutiveFailures` /
`consecutiveSuccesses` counter per target in memory, incrementing or
resetting it on each check — is more efficient (no extra query) but has a
real correctness problem: that counter lives only in process memory, so a
restart mid-streak silently forgets how close a target was to tripping,
and there's no way to reconstruct it except from history that's already
in the database. `store.Record` instead queries the last `threshold`
checks for the target (again using the `checks(target_id, checked_at
desc)` index) and evaluates the trailing window fresh every time. It costs
one more query per check; in exchange, the incident state machine is
fully derivable from what's persisted, with no in-memory state to lose or
desync — after a crash and restart, the very next check picks up exactly
where the history left off.

**Why the check insert, the threshold read, and the incident open/close
all happen in one transaction** (`store.Record`). If the process died
between "insert the check" and "open the incident", a target could be
failing in the database without an incident ever being recorded for it —
or vice versa, an incident could reference a check that was never
actually committed. Wrapping all of it in one `pgx.Tx` means a crash at
any point leaves either the old state or the new state, never something
in between.

**Why the webhook fires after commit, from a goroutine, not inside the
transaction.** The incident transition is the source of truth — it's
written to Postgres and that's authoritative regardless of whether
anyone downstream hears about it. The webhook is a best-effort side
effect layered on top: `store.notify` spawns it in its own goroutine
(with its own bounded timeout, detached from the request context) after
the transaction commits, specifically so a slow or unreachable webhook
endpoint can never delay the single writer goroutine from draining the
next result. If the webhook delivery ultimately fails after all retries,
the incident is still correctly open or resolved in the database — the
failure is logged, not surfaced as a reason to reconsider the state
transition.
