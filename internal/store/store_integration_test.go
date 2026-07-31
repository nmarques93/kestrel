//go:build integration

// Run with: make test-integration (requires Docker).
package store

import (
	"context"
	"testing"
	"time"

	"github.com/nmarques93/kestrel/internal/checker"
	"github.com/nmarques93/kestrel/internal/testutil"
	"github.com/nmarques93/kestrel/internal/webhook"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	return New(testutil.NewPool(t))
}

func insertTarget(t *testing.T, s *Store, threshold int32) int64 {
	t.Helper()
	var id int64
	err := s.pool.QueryRow(context.Background(), `
		INSERT INTO targets (name, url, interval_seconds, timeout_ms, consecutive_threshold)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, "test target", "http://example.invalid", 60, 5000, threshold).Scan(&id)
	if err != nil {
		t.Fatalf("insert target: %v", err)
	}
	return id
}

func TestStoreDueTargetsIncludesNeverCheckedTargets(t *testing.T) {
	s := newTestStore(t)
	id := insertTarget(t, s, 3)

	due, err := s.DueTargets(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("DueTargets: %v", err)
	}
	if len(due) != 1 || due[0].ID != id {
		t.Fatalf("DueTargets = %+v, want exactly the newly created target", due)
	}
}

func TestStoreDueTargetsExcludesRecentlyCheckedTargets(t *testing.T) {
	s := newTestStore(t)
	id := insertTarget(t, s, 3)

	if err := s.Record(context.Background(), checker.Result{
		TargetID: id, CheckedAt: time.Now(), Success: true, StatusCode: 200, LatencyMS: 10,
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	due, err := s.DueTargets(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("DueTargets: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("DueTargets = %+v, want none (just checked, interval not elapsed)", due)
	}
}

func TestStoreRecordOpensAndResolvesIncidentOnThreshold(t *testing.T) {
	s := newTestStore(t)
	id := insertTarget(t, s, 3)
	ctx := context.Background()
	now := time.Now()

	record := func(success bool) {
		t.Helper()
		now = now.Add(time.Second)
		if err := s.Record(ctx, checker.Result{
			TargetID: id, CheckedAt: now, Success: success, StatusCode: 200, LatencyMS: 5,
			Err: map[bool]string{false: "boom"}[success],
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	openIncidents := func() int {
		t.Helper()
		var n int
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM incidents WHERE target_id = $1 AND resolved_at IS NULL`, id).Scan(&n); err != nil {
			t.Fatalf("count open incidents: %v", err)
		}
		return n
	}

	record(false)
	record(false)
	if openIncidents() != 0 {
		t.Fatal("incident opened before threshold reached")
	}

	record(false) // 3rd consecutive failure: threshold reached
	if openIncidents() != 1 {
		t.Fatal("expected exactly one open incident after 3 consecutive failures")
	}

	record(true)
	record(true)
	if openIncidents() != 1 {
		t.Fatal("incident resolved before recovery threshold reached")
	}

	record(true) // 3rd consecutive success: recovers
	if openIncidents() != 0 {
		t.Fatal("expected the incident to be resolved after 3 consecutive successes")
	}

	var resolvedCount int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM incidents WHERE target_id = $1 AND resolved_at IS NOT NULL`, id).Scan(&resolvedCount); err != nil {
		t.Fatalf("count resolved incidents: %v", err)
	}
	if resolvedCount != 1 {
		t.Fatalf("resolved incidents = %d, want 1", resolvedCount)
	}
}

// channelNotifier hands every event to a channel so a test can wait for the
// background goroutine store.notify spawns instead of racing it.
type channelNotifier struct {
	events chan webhook.Event
}

func newChannelNotifier() *channelNotifier {
	return &channelNotifier{events: make(chan webhook.Event, 10)}
}

func (n *channelNotifier) Notify(_ context.Context, event webhook.Event) error {
	n.events <- event
	return nil
}

func (n *channelNotifier) awaitEvent(t *testing.T) webhook.Event {
	t.Helper()
	select {
	case e := <-n.events:
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook notification")
		panic("unreachable")
	}
}

func TestMCPTokenCreateAndAuthenticate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	token, err := s.CreateMCPToken(ctx)
	if err != nil {
		t.Fatalf("CreateMCPToken: %v", err)
	}
	if token == "" {
		t.Fatal("CreateMCPToken returned an empty token")
	}

	valid, err := s.AuthenticateMCPToken(ctx, token)
	if err != nil {
		t.Fatalf("AuthenticateMCPToken: %v", err)
	}
	if !valid {
		t.Fatal("AuthenticateMCPToken(valid token) = false, want true")
	}

	var lastUsedAt *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT last_used_at FROM mcp_tokens WHERE token_hash = $1`, hashToken(token)).Scan(&lastUsedAt); err != nil {
		t.Fatalf("query last_used_at: %v", err)
	}
	if lastUsedAt == nil {
		t.Fatal("last_used_at was not set after a successful authentication")
	}

	invalid, err := s.AuthenticateMCPToken(ctx, "kestrel_not-a-real-token")
	if err != nil {
		t.Fatalf("AuthenticateMCPToken(bogus): %v", err)
	}
	if invalid {
		t.Fatal("AuthenticateMCPToken(bogus token) = true, want false")
	}

	empty, err := s.AuthenticateMCPToken(ctx, "")
	if err != nil {
		t.Fatalf("AuthenticateMCPToken(empty): %v", err)
	}
	if empty {
		t.Fatal("AuthenticateMCPToken(empty token) = true, want false")
	}
}

func TestStoreFiresWebhookOnTransitions(t *testing.T) {
	s := newTestStore(t)
	notifier := newChannelNotifier()
	s.SetNotifier(notifier)

	id := insertTarget(t, s, 2)
	ctx := context.Background()
	now := time.Now()

	record := func(success bool) {
		t.Helper()
		now = now.Add(time.Second)
		errText := ""
		if !success {
			errText = "boom"
		}
		if err := s.Record(ctx, checker.Result{
			TargetID: id, CheckedAt: now, Success: success, StatusCode: 200, LatencyMS: 5, Err: errText,
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	record(false)
	record(false) // trips DOWN (threshold 2)

	downEvent := notifier.awaitEvent(t)
	if downEvent.Type != "down" || downEvent.TargetID != id || downEvent.Cause == nil || *downEvent.Cause != "boom" {
		t.Fatalf("down event = %+v, want type=down target_id=%d cause=boom", downEvent, id)
	}
	incidentID := downEvent.IncidentID

	record(true)
	record(true) // recovers

	upEvent := notifier.awaitEvent(t)
	if upEvent.Type != "up" || upEvent.TargetID != id || upEvent.IncidentID != incidentID || upEvent.ResolvedAt == nil {
		t.Fatalf("up event = %+v, want type=up target_id=%d incident_id=%d with resolved_at set", upEvent, id, incidentID)
	}

	select {
	case extra := <-notifier.events:
		t.Fatalf("unexpected extra webhook event: %+v", extra)
	default:
	}
}
