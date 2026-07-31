//go:build integration

// Run with: make test-integration (requires Docker).
package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/nmarques93/kestrel/internal/checker"
	"github.com/nmarques93/kestrel/migrations"

	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
	goose "github.com/pressly/goose/v3"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("kestrel"),
		postgres.WithUsername("kestrel"),
		postgres.WithPassword("kestrel"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database/sql: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	return New(pool)
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
