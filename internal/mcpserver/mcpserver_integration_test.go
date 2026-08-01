//go:build integration

// Run with: make test-integration (requires Docker).
package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nmarques93/kestrel/internal/checker"
	"github.com/nmarques93/kestrel/internal/store"
	"github.com/nmarques93/kestrel/internal/testutil"
)

// connect wires an in-process MCP client to a server backed by a real,
// migrated Postgres instance — exercising the actual protocol (schema
// validation, JSON encoding) rather than calling Go functions directly.
func connect(t *testing.T) (*mcp.ClientSession, *store.Store) {
	t.Helper()
	s := store.New(testutil.NewPool(t))
	server := New(s)

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	ctx := context.Background()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })

	return session, s
}

func callTool[T any](t *testing.T, session *mcp.ClientSession, name string, args map[string]any) T {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	if result.IsError {
		t.Fatalf("CallTool(%s) returned a tool error: %+v", name, result.Content)
	}
	b, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal into %T: %v (raw: %s)", out, err, b)
	}
	return out
}

func callToolExpectError(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		// A protocol-level error (e.g. schema validation) also counts.
		return
	}
	if !result.IsError {
		t.Fatalf("CallTool(%s) succeeded, want a tool error", name)
	}
}

func TestMCPCreateListGetDeleteTarget(t *testing.T) {
	session, _ := connect(t)

	created := callTool[targetOutput](t, session, "create_target", map[string]any{
		"name": "example", "url": "https://example.com", "interval_seconds": 30,
	})
	if created.Name != "example" || created.IntervalSeconds != 30 || created.TimeoutMS != 5000 {
		t.Fatalf("created = %+v, defaults not applied as expected", created)
	}

	list := callTool[listTargetsOutput](t, session, "list_targets", nil)
	if len(list.Targets) != 1 || list.Targets[0].ID != created.ID || !list.Targets[0].Up {
		t.Fatalf("list_targets = %+v, want exactly the created target, up=true", list)
	}

	updated := callTool[targetOutput](t, session, "update_target", map[string]any{
		"target_id": created.ID, "name": "renamed", "url": "https://example.com", "interval_seconds": 45,
	})
	if updated.Name != "renamed" || updated.IntervalSeconds != 45 {
		t.Fatalf("updated = %+v, want name=renamed interval=45", updated)
	}

	deleted := callTool[deleteTargetOutput](t, session, "delete_target", map[string]any{"target_id": created.ID})
	if !deleted.Deleted {
		t.Fatalf("delete_target = %+v, want deleted=true", deleted)
	}

	afterDelete := callTool[listTargetsOutput](t, session, "list_targets", nil)
	if len(afterDelete.Targets) != 0 {
		t.Fatalf("list_targets after delete = %+v, want empty", afterDelete)
	}
}

func TestMCPCreateTargetValidation(t *testing.T) {
	session, _ := connect(t)

	callToolExpectError(t, session, "create_target", map[string]any{"name": "", "url": "https://example.com"})
	callToolExpectError(t, session, "create_target", map[string]any{"name": "x", "url": "not-a-url"})
}

func TestMCPChecksIncidentsAndUptime(t *testing.T) {
	session, s := connect(t)

	created, err := s.CreateTarget(t.Context(), store.TargetParams{
		Name: "flaky", URL: "https://example.com",
		ExpectedStatusMin: 200, ExpectedStatusMax: 300,
		IntervalSeconds: 60, TimeoutMS: 5000, ConsecutiveThreshold: 2,
	})
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}

	record := func(success bool) {
		t.Helper()
		errText := ""
		if !success {
			errText = "boom"
		}
		if err := s.Record(t.Context(), checker.Result{
			TargetID: created.ID, CheckedAt: time.Now(), Success: success, StatusCode: 200, LatencyMS: 5, Err: errText,
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	record(true)
	record(false)
	record(false) // trips DOWN (threshold 2)

	checks := callTool[listChecksOutput](t, session, "list_checks", map[string]any{"target_id": created.ID})
	if len(checks.Checks) != 3 {
		t.Fatalf("list_checks = %d, want 3", len(checks.Checks))
	}

	incidents := callTool[listIncidentsOutput](t, session, "list_incidents", map[string]any{"target_id": created.ID})
	if len(incidents.Incidents) != 1 || !incidents.Incidents[0].Ongoing || incidents.Incidents[0].DurationSeconds != nil {
		t.Fatalf("list_incidents = %+v, want exactly one ongoing incident with no duration yet", incidents)
	}

	global := callTool[listIncidentsOutput](t, session, "list_incidents", nil)
	if len(global.Incidents) != 1 {
		t.Fatalf("global list_incidents = %+v, want exactly one", global)
	}

	uptime := callTool[getUptimeOutput](t, session, "get_uptime", map[string]any{"target_id": created.ID, "window_hours": 1})
	if !uptime.HasData || uptime.SampleSize != 3 {
		t.Fatalf("get_uptime = %+v, want has_data=true sample_size=3", uptime)
	}

	record(true)
	record(true) // recovers (threshold 2)

	resolved := callTool[listIncidentsOutput](t, session, "list_incidents", map[string]any{"target_id": created.ID})
	if len(resolved.Incidents) != 1 || resolved.Incidents[0].Ongoing || resolved.Incidents[0].DurationSeconds == nil {
		t.Fatalf("list_incidents after recovery = %+v, want one resolved incident with a duration", resolved)
	}
}
