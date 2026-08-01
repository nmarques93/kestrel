package store

import (
	"context"
	"fmt"
	"time"
)

// Incident is a single DOWN/recovery cycle, as read back for the API.
type Incident struct {
	ID         int64
	TargetID   int64
	TargetName string
	StartedAt  time.Time
	ResolvedAt *time.Time
	Cause      *string
}

// DurationSeconds reports how long a resolved incident lasted, or nil if
// it's still ongoing. Both the REST API and the MCP tools call this rather
// than each computing the subtraction themselves.
func (i Incident) DurationSeconds() *float64 {
	if i.ResolvedAt == nil {
		return nil
	}
	d := i.ResolvedAt.Sub(i.StartedAt).Seconds()
	return &d
}

// ListIncidents returns incidents newest-first, optionally filtered to a
// single target. Pass targetID nil for the incident timeline across every
// target.
func (s *Store) ListIncidents(ctx context.Context, targetID *int64, limit int) ([]Incident, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT i.id, i.target_id, t.name, i.started_at, i.resolved_at, i.cause
		FROM incidents i
		JOIN targets t ON t.id = i.target_id
		WHERE $1::bigint IS NULL OR i.target_id = $1
		ORDER BY i.started_at DESC
		LIMIT $2
	`, targetID, limit)
	if err != nil {
		return nil, fmt.Errorf("query incidents: %w", err)
	}
	defer rows.Close()

	var incidents []Incident
	for rows.Next() {
		var inc Incident
		if err := rows.Scan(&inc.ID, &inc.TargetID, &inc.TargetName, &inc.StartedAt, &inc.ResolvedAt, &inc.Cause); err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}
		incidents = append(incidents, inc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incidents: %w", err)
	}
	return incidents, nil
}
