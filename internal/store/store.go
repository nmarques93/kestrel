// Package store is the only thing that talks to Postgres. It implements
// the checker package's TargetSource and ResultRecorder interfaces for the
// checking engine, and provides the target/check/incident CRUD and read
// methods the HTTP API (and, later, the MCP server) build on — the same
// methods back both, so business logic lives in exactly one place.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nmarques93/kestrel/internal/checker"
	"github.com/nmarques93/kestrel/internal/incident"
)

// Store implements checker.TargetSource and checker.ResultRecorder against
// Postgres.
type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

var (
	_ checker.TargetSource   = (*Store)(nil)
	_ checker.ResultRecorder = (*Store)(nil)
)

// DueTargets returns every target whose most recent check (if any) is older
// than its configured interval as of now. A target with no checks yet is
// always due.
func (s *Store) DueTargets(ctx context.Context, now time.Time) ([]checker.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.name, t.url, lower(t.expected_status_range), upper(t.expected_status_range),
		       t.interval_seconds, t.timeout_ms, t.consecutive_threshold
		FROM targets t
		LEFT JOIN LATERAL (
			SELECT checked_at FROM checks c
			WHERE c.target_id = t.id
			ORDER BY c.checked_at DESC
			LIMIT 1
		) last_check ON true
		WHERE last_check.checked_at IS NULL
		   OR last_check.checked_at + (t.interval_seconds * interval '1 second') <= $1
	`, now)
	if err != nil {
		return nil, fmt.Errorf("query due targets: %w", err)
	}
	defer rows.Close()

	var targets []checker.Target
	for rows.Next() {
		var t checker.Target
		if err := rows.Scan(&t.ID, &t.Name, &t.URL, &t.ExpectedStatusMin, &t.ExpectedStatusMax,
			&t.IntervalSeconds, &t.TimeoutMS, &t.ConsecutiveThreshold); err != nil {
			return nil, fmt.Errorf("scan target: %w", err)
		}
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due targets: %w", err)
	}
	return targets, nil
}

// Record persists a check result and, in the same transaction, evaluates
// and applies any resulting incident state transition. Running both in one
// transaction keeps a check row and an incident open/close in sync even if
// the process dies partway through.
func (s *Store) Record(ctx context.Context, result checker.Result) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	var statusCode *int32
	if result.StatusCode != 0 {
		v := int32(result.StatusCode)
		statusCode = &v
	}
	var errText *string
	if result.Err != "" {
		errText = &result.Err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO checks (target_id, checked_at, success, status_code, latency_ms, error)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, result.TargetID, result.CheckedAt, result.Success, statusCode, result.LatencyMS, errText); err != nil {
		return fmt.Errorf("insert check: %w", err)
	}

	var threshold int32
	if err := tx.QueryRow(ctx, `SELECT consecutive_threshold FROM targets WHERE id = $1`, result.TargetID).Scan(&threshold); err != nil {
		return fmt.Errorf("load threshold: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT success FROM checks WHERE target_id = $1 ORDER BY checked_at DESC LIMIT $2
	`, result.TargetID, threshold)
	if err != nil {
		return fmt.Errorf("load recent checks: %w", err)
	}
	var recent []bool
	for rows.Next() {
		var success bool
		if err := rows.Scan(&success); err != nil {
			rows.Close()
			return fmt.Errorf("scan recent check: %w", err)
		}
		recent = append(recent, success)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate recent checks: %w", err)
	}

	var openIncidentID int64
	currentState := incident.Up
	switch err := tx.QueryRow(ctx, `
		SELECT id FROM incidents WHERE target_id = $1 AND resolved_at IS NULL
	`, result.TargetID).Scan(&openIncidentID); {
	case err == nil:
		currentState = incident.Down
	case errors.Is(err, pgx.ErrNoRows):
		// No open incident: target is currently considered up.
	default:
		return fmt.Errorf("load open incident: %w", err)
	}

	switch incident.Evaluate(currentState, recent, int(threshold)) {
	case incident.ToDown:
		if _, err := tx.Exec(ctx, `
			INSERT INTO incidents (target_id, started_at, cause) VALUES ($1, $2, $3)
		`, result.TargetID, result.CheckedAt, errText); err != nil {
			return fmt.Errorf("open incident: %w", err)
		}
	case incident.ToUp:
		if _, err := tx.Exec(ctx, `
			UPDATE incidents SET resolved_at = $1 WHERE id = $2
		`, result.CheckedAt, openIncidentID); err != nil {
			return fmt.Errorf("resolve incident: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
