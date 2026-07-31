package store

import (
	"context"
	"fmt"
	"time"
)

// Check is a single recorded check result, as read back for the API.
type Check struct {
	ID         int64
	TargetID   int64
	CheckedAt  time.Time
	Success    bool
	StatusCode *int32
	LatencyMS  int64
	Err        *string
}

// ListChecks returns a target's most recent checks, newest first.
func (s *Store) ListChecks(ctx context.Context, targetID int64, limit int) ([]Check, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, target_id, checked_at, success, status_code, latency_ms, error
		FROM checks WHERE target_id = $1
		ORDER BY checked_at DESC
		LIMIT $2
	`, targetID, limit)
	if err != nil {
		return nil, fmt.Errorf("query checks: %w", err)
	}
	defer rows.Close()

	var checks []Check
	for rows.Next() {
		var c Check
		if err := rows.Scan(&c.ID, &c.TargetID, &c.CheckedAt, &c.Success, &c.StatusCode, &c.LatencyMS, &c.Err); err != nil {
			return nil, fmt.Errorf("scan check: %w", err)
		}
		checks = append(checks, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checks: %w", err)
	}
	return checks, nil
}
