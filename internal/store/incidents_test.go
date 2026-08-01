package store

import (
	"testing"
	"time"
)

func TestIncidentDurationSeconds(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	ongoing := Incident{StartedAt: start}
	if d := ongoing.DurationSeconds(); d != nil {
		t.Errorf("DurationSeconds() for an ongoing incident = %v, want nil", d)
	}

	resolved := Incident{StartedAt: start, ResolvedAt: ptrTime(start.Add(90 * time.Second))}
	d := resolved.DurationSeconds()
	if d == nil || *d != 90 {
		t.Errorf("DurationSeconds() = %v, want 90", d)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
