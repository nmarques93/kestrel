package mcpserver

import "time"

type emptyInput struct{}

type targetStatus struct {
	ID                   int64      `json:"id"`
	Name                 string     `json:"name"`
	URL                  string     `json:"url"`
	ExpectedStatusMin    int32      `json:"expected_status_min"`
	ExpectedStatusMax    int32      `json:"expected_status_max"`
	IntervalSeconds      int32      `json:"interval_seconds"`
	TimeoutMS            int32      `json:"timeout_ms"`
	ConsecutiveThreshold int32      `json:"consecutive_threshold"`
	CreatedAt            time.Time  `json:"created_at"`
	Up                   bool       `json:"up"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
}

type listTargetsOutput struct {
	Targets []targetStatus `json:"targets"`
}

type checkOut struct {
	ID         int64     `json:"id"`
	CheckedAt  time.Time `json:"checked_at"`
	Success    bool      `json:"success"`
	StatusCode *int32    `json:"status_code,omitempty"`
	LatencyMS  int64     `json:"latency_ms"`
	Error      *string   `json:"error,omitempty"`
}

type listChecksInput struct {
	TargetID int64 `json:"target_id" jsonschema:"the target's id"`
	Limit    int   `json:"limit,omitempty" jsonschema:"maximum checks to return, defaults to 50, capped at 500"`
}

type listChecksOutput struct {
	Checks []checkOut `json:"checks"`
}

type incidentOut struct {
	ID         int64      `json:"id"`
	TargetID   int64      `json:"target_id"`
	TargetName string     `json:"target_name"`
	StartedAt  time.Time  `json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Ongoing    bool       `json:"ongoing"`
	Cause      *string    `json:"cause,omitempty"`
}

type listIncidentsInput struct {
	TargetID *int64 `json:"target_id,omitempty" jsonschema:"limit to this target's incidents; omit for the timeline across every target"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum incidents to return, defaults to 50, capped at 500"`
}

type listIncidentsOutput struct {
	Incidents []incidentOut `json:"incidents"`
}

type getUptimeInput struct {
	TargetID    int64 `json:"target_id" jsonschema:"the target's id"`
	WindowHours int   `json:"window_hours,omitempty" jsonschema:"time window in hours, defaults to 24"`
}

type getUptimeOutput struct {
	WindowHours int     `json:"window_hours"`
	Percent     float64 `json:"percent"`
	SampleSize  int     `json:"sample_size"`
	HasData     bool    `json:"has_data" jsonschema:"false if there were no checks in the window, in which case percent is meaningless"`
}

// createTargetInput and updateTargetInput deliberately don't share a
// struct via embedding — jsonschema inference over embedded fields isn't
// something this codebase has verified, so each is spelled out explicitly.
// Pointer fields are optional and defaulted the same way every other
// TargetParams caller defaults them (see store.Default*).
type createTargetInput struct {
	Name                 string `json:"name" jsonschema:"a short human-readable name for the target"`
	URL                  string `json:"url" jsonschema:"the http:// or https:// URL to check"`
	ExpectedStatusMin    *int32 `json:"expected_status_min,omitempty" jsonschema:"lowest acceptable HTTP status code, inclusive; defaults to 200"`
	ExpectedStatusMax    *int32 `json:"expected_status_max,omitempty" jsonschema:"highest acceptable HTTP status code, exclusive; defaults to 300"`
	IntervalSeconds      *int32 `json:"interval_seconds,omitempty" jsonschema:"seconds between checks; defaults to 60"`
	TimeoutMS            *int32 `json:"timeout_ms,omitempty" jsonschema:"per-check timeout in milliseconds; defaults to 5000"`
	ConsecutiveThreshold *int32 `json:"consecutive_threshold,omitempty" jsonschema:"consecutive failures (or successes) required to flip DOWN/UP; defaults to 3"`
}

type updateTargetInput struct {
	TargetID             int64  `json:"target_id" jsonschema:"the target's id"`
	Name                 string `json:"name" jsonschema:"a short human-readable name for the target"`
	URL                  string `json:"url" jsonschema:"the http:// or https:// URL to check"`
	ExpectedStatusMin    *int32 `json:"expected_status_min,omitempty" jsonschema:"lowest acceptable HTTP status code, inclusive; defaults to 200"`
	ExpectedStatusMax    *int32 `json:"expected_status_max,omitempty" jsonschema:"highest acceptable HTTP status code, exclusive; defaults to 300"`
	IntervalSeconds      *int32 `json:"interval_seconds,omitempty" jsonschema:"seconds between checks; defaults to 60"`
	TimeoutMS            *int32 `json:"timeout_ms,omitempty" jsonschema:"per-check timeout in milliseconds; defaults to 5000"`
	ConsecutiveThreshold *int32 `json:"consecutive_threshold,omitempty" jsonschema:"consecutive failures (or successes) required to flip DOWN/UP; defaults to 3"`
}

type targetOutput struct {
	ID                   int64     `json:"id"`
	Name                 string    `json:"name"`
	URL                  string    `json:"url"`
	ExpectedStatusMin    int32     `json:"expected_status_min"`
	ExpectedStatusMax    int32     `json:"expected_status_max"`
	IntervalSeconds      int32     `json:"interval_seconds"`
	TimeoutMS            int32     `json:"timeout_ms"`
	ConsecutiveThreshold int32     `json:"consecutive_threshold"`
	CreatedAt            time.Time `json:"created_at"`
}

type deleteTargetInput struct {
	TargetID int64 `json:"target_id" jsonschema:"the target's id"`
}

type deleteTargetOutput struct {
	Deleted bool `json:"deleted"`
}
