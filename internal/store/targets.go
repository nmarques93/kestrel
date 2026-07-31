package store

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nmarques93/kestrel/internal/checker"
)

// ErrNotFound is returned by Get/Update/Delete calls for an id that
// doesn't exist.
var ErrNotFound = errors.New("not found")

// Defaults mirror the targets table's column defaults (migrations/00001).
// Exported so every caller that builds TargetParams from partial input —
// the REST API and the MCP tools alike — defaults missing fields the same
// way, rather than each guessing its own values.
const (
	DefaultExpectedStatusMin    int32 = 200
	DefaultExpectedStatusMax    int32 = 300
	DefaultIntervalSeconds      int32 = 60
	DefaultTimeoutMS            int32 = 5000
	DefaultConsecutiveThreshold int32 = 3
)

// TargetStatus is a target plus the read-only status the status page and
// the targets list API report alongside it.
type TargetStatus struct {
	checker.Target
	Up            bool
	LastCheckedAt *time.Time
}

// TargetParams holds the user-supplied fields for creating or updating a
// target. Callers should run ValidateTargetParams before persisting —
// CreateTarget and UpdateTarget persist exactly what they're given.
type TargetParams struct {
	Name                 string
	URL                  string
	ExpectedStatusMin    int32
	ExpectedStatusMax    int32
	IntervalSeconds      int32
	TimeoutMS            int32
	ConsecutiveThreshold int32
}

// ValidateTargetParams checks that params are sane before they're persisted.
// It's the single source of truth for what makes a target valid, called by
// both the REST API and the MCP write tools.
func ValidateTargetParams(p TargetParams) error {
	if p.Name == "" {
		return errors.New("name is required")
	}
	u, err := url.Parse(p.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("url must be a valid http:// or https:// URL")
	}
	if p.ExpectedStatusMin < 100 || p.ExpectedStatusMax > 599 || p.ExpectedStatusMin >= p.ExpectedStatusMax {
		return errors.New("expected_status_min must be less than expected_status_max, within 100-599")
	}
	if p.IntervalSeconds <= 0 {
		return errors.New("interval_seconds must be positive")
	}
	if p.TimeoutMS <= 0 {
		return errors.New("timeout_ms must be positive")
	}
	if p.ConsecutiveThreshold <= 0 {
		return errors.New("consecutive_threshold must be positive")
	}
	return nil
}

// ListTargets returns every target along with whether it's currently up
// (no open incident) and when it was last checked.
func (s *Store) ListTargets(ctx context.Context) ([]TargetStatus, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.name, t.url, lower(t.expected_status_range), upper(t.expected_status_range),
		       t.interval_seconds, t.timeout_ms, t.consecutive_threshold, t.created_at,
		       (open_incident.id IS NOT NULL) AS down, last_check.checked_at
		FROM targets t
		LEFT JOIN incidents open_incident
		       ON open_incident.target_id = t.id AND open_incident.resolved_at IS NULL
		LEFT JOIN LATERAL (
			SELECT checked_at FROM checks c WHERE c.target_id = t.id ORDER BY c.checked_at DESC LIMIT 1
		) last_check ON true
		ORDER BY t.name
	`)
	if err != nil {
		return nil, fmt.Errorf("query targets: %w", err)
	}
	defer rows.Close()

	var targets []TargetStatus
	for rows.Next() {
		var ts TargetStatus
		var down bool
		if err := rows.Scan(&ts.ID, &ts.Name, &ts.URL, &ts.ExpectedStatusMin, &ts.ExpectedStatusMax,
			&ts.IntervalSeconds, &ts.TimeoutMS, &ts.ConsecutiveThreshold, &ts.CreatedAt,
			&down, &ts.LastCheckedAt); err != nil {
			return nil, fmt.Errorf("scan target: %w", err)
		}
		ts.Up = !down
		targets = append(targets, ts)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate targets: %w", err)
	}
	return targets, nil
}

// GetTarget returns a single target by id, or ErrNotFound.
func (s *Store) GetTarget(ctx context.Context, id int64) (checker.Target, error) {
	var t checker.Target
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, url, lower(expected_status_range), upper(expected_status_range),
		       interval_seconds, timeout_ms, consecutive_threshold, created_at
		FROM targets WHERE id = $1
	`, id).Scan(&t.ID, &t.Name, &t.URL, &t.ExpectedStatusMin, &t.ExpectedStatusMax,
		&t.IntervalSeconds, &t.TimeoutMS, &t.ConsecutiveThreshold, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return checker.Target{}, ErrNotFound
	}
	if err != nil {
		return checker.Target{}, fmt.Errorf("query target: %w", err)
	}
	return t, nil
}

// CreateTarget inserts a new target and returns it with its assigned id and
// created_at.
func (s *Store) CreateTarget(ctx context.Context, p TargetParams) (checker.Target, error) {
	t := checker.Target{
		Name:                 p.Name,
		URL:                  p.URL,
		ExpectedStatusMin:    p.ExpectedStatusMin,
		ExpectedStatusMax:    p.ExpectedStatusMax,
		IntervalSeconds:      p.IntervalSeconds,
		TimeoutMS:            p.TimeoutMS,
		ConsecutiveThreshold: p.ConsecutiveThreshold,
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO targets (name, url, expected_status_range, interval_seconds, timeout_ms, consecutive_threshold)
		VALUES ($1, $2, int4range($3, $4), $5, $6, $7)
		RETURNING id, created_at
	`, p.Name, p.URL, p.ExpectedStatusMin, p.ExpectedStatusMax, p.IntervalSeconds, p.TimeoutMS, p.ConsecutiveThreshold,
	).Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		return checker.Target{}, fmt.Errorf("insert target: %w", err)
	}
	return t, nil
}

// UpdateTarget replaces a target's mutable fields. Returns ErrNotFound if
// no target with that id exists.
func (s *Store) UpdateTarget(ctx context.Context, id int64, p TargetParams) (checker.Target, error) {
	t := checker.Target{ID: id}
	err := s.pool.QueryRow(ctx, `
		UPDATE targets
		SET name = $1, url = $2, expected_status_range = int4range($3, $4),
		    interval_seconds = $5, timeout_ms = $6, consecutive_threshold = $7
		WHERE id = $8
		RETURNING name, url, lower(expected_status_range), upper(expected_status_range),
		          interval_seconds, timeout_ms, consecutive_threshold, created_at
	`, p.Name, p.URL, p.ExpectedStatusMin, p.ExpectedStatusMax, p.IntervalSeconds, p.TimeoutMS, p.ConsecutiveThreshold, id,
	).Scan(&t.Name, &t.URL, &t.ExpectedStatusMin, &t.ExpectedStatusMax,
		&t.IntervalSeconds, &t.TimeoutMS, &t.ConsecutiveThreshold, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return checker.Target{}, ErrNotFound
	}
	if err != nil {
		return checker.Target{}, fmt.Errorf("update target: %w", err)
	}
	return t, nil
}

// DeleteTarget removes a target and, via ON DELETE CASCADE, its checks and
// incidents. Returns ErrNotFound if no target with that id exists.
func (s *Store) DeleteTarget(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM targets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete target: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Uptime reports the fraction of checks that succeeded for a target since
// the given time, along with how many checks that's based on. sampleSize
// is 0 when the target has no checks in the window, in which case percent
// is meaningless and callers should say so rather than showing "0%".
func (s *Store) Uptime(ctx context.Context, targetID int64, since time.Time) (percent float64, sampleSize int, err error) {
	var successCount, total int
	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE success), count(*)
		FROM checks WHERE target_id = $1 AND checked_at >= $2
	`, targetID, since).Scan(&successCount, &total)
	if err != nil {
		return 0, 0, fmt.Errorf("query uptime: %w", err)
	}
	if total == 0 {
		return 0, 0, nil
	}
	return float64(successCount) / float64(total) * 100, total, nil
}
