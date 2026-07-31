// Package checker implements the concurrent checking engine: a scheduler
// dispatches due targets into a bounded worker pool, and a single writer
// goroutine drains results so nothing writes to the database concurrently.
package checker

import (
	"context"
	"time"
)

// Target is the subset of a monitored target's configuration the checker
// engine needs to run a check against it.
type Target struct {
	ID                   int64
	Name                 string
	URL                  string
	ExpectedStatusMin    int32 // inclusive
	ExpectedStatusMax    int32 // exclusive
	IntervalSeconds      int32
	TimeoutMS            int32
	ConsecutiveThreshold int32
}

// Result is the outcome of a single check.
type Result struct {
	TargetID   int64
	CheckedAt  time.Time
	Success    bool
	StatusCode int // 0 if no response was received
	LatencyMS  int64
	Err        string // empty when Success is true
}

// Prober performs a single check against a target. ctx carries the
// per-target timeout; a real implementation must respect it.
type Prober interface {
	Probe(ctx context.Context, target Target) Result
}

// TargetSource returns the targets due for a check as of now.
type TargetSource interface {
	DueTargets(ctx context.Context, now time.Time) ([]Target, error)
}

// ResultRecorder persists a check result and applies any resulting incident
// state transition. Implementations are called from a single goroutine, so
// they don't need to guard against concurrent calls for the same target.
type ResultRecorder interface {
	Record(ctx context.Context, result Result) error
}
