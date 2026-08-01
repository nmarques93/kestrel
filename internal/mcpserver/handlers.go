package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nmarques93/kestrel/internal/checker"
	"github.com/nmarques93/kestrel/internal/store"
)

func listTargetsHandler(s *store.Store) mcp.ToolHandlerFor[emptyInput, listTargetsOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, listTargetsOutput, error) {
		targets, err := s.ListTargets(ctx)
		if err != nil {
			return nil, listTargetsOutput{}, fmt.Errorf("list targets: %w", err)
		}
		out := listTargetsOutput{Targets: make([]targetStatus, len(targets))}
		for i, t := range targets {
			out.Targets[i] = targetStatus{
				ID: t.ID, Name: t.Name, URL: t.URL,
				ExpectedStatusMin: t.ExpectedStatusMin, ExpectedStatusMax: t.ExpectedStatusMax,
				IntervalSeconds: t.IntervalSeconds, TimeoutMS: t.TimeoutMS,
				ConsecutiveThreshold: t.ConsecutiveThreshold, CreatedAt: t.CreatedAt,
				Up: t.Up, LastCheckedAt: t.LastCheckedAt,
			}
		}
		return nil, out, nil
	}
}

func listChecksHandler(s *store.Store) mcp.ToolHandlerFor[listChecksInput, listChecksOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in listChecksInput) (*mcp.CallToolResult, listChecksOutput, error) {
		limit := clampLimit(in.Limit)
		checks, err := s.ListChecks(ctx, in.TargetID, limit)
		if err != nil {
			return nil, listChecksOutput{}, fmt.Errorf("list checks: %w", err)
		}
		out := listChecksOutput{Checks: make([]checkOut, len(checks))}
		for i, c := range checks {
			out.Checks[i] = checkOut{
				ID: c.ID, CheckedAt: c.CheckedAt, Success: c.Success,
				StatusCode: c.StatusCode, LatencyMS: c.LatencyMS, Error: c.Err,
			}
		}
		return nil, out, nil
	}
}

func listIncidentsHandler(s *store.Store) mcp.ToolHandlerFor[listIncidentsInput, listIncidentsOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in listIncidentsInput) (*mcp.CallToolResult, listIncidentsOutput, error) {
		limit := clampLimit(in.Limit)
		incidents, err := s.ListIncidents(ctx, in.TargetID, limit)
		if err != nil {
			return nil, listIncidentsOutput{}, fmt.Errorf("list incidents: %w", err)
		}
		out := listIncidentsOutput{Incidents: make([]incidentOut, len(incidents))}
		for i, inc := range incidents {
			out.Incidents[i] = incidentOut{
				ID: inc.ID, TargetID: inc.TargetID, TargetName: inc.TargetName,
				StartedAt: inc.StartedAt, ResolvedAt: inc.ResolvedAt, Cause: inc.Cause,
				Ongoing: inc.ResolvedAt == nil, DurationSeconds: inc.DurationSeconds(),
			}
		}
		return nil, out, nil
	}
}

func getUptimeHandler(s *store.Store) mcp.ToolHandlerFor[getUptimeInput, getUptimeOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in getUptimeInput) (*mcp.CallToolResult, getUptimeOutput, error) {
		windowHours := in.WindowHours
		if windowHours <= 0 {
			windowHours = 24
		}
		percent, sampleSize, err := s.Uptime(ctx, in.TargetID, time.Now().Add(-time.Duration(windowHours)*time.Hour))
		if err != nil {
			return nil, getUptimeOutput{}, fmt.Errorf("get uptime: %w", err)
		}
		return nil, getUptimeOutput{
			WindowHours: windowHours, Percent: percent, SampleSize: sampleSize, HasData: sampleSize > 0,
		}, nil
	}
}

func createTargetHandler(s *store.Store) mcp.ToolHandlerFor[createTargetInput, targetOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in createTargetInput) (*mcp.CallToolResult, targetOutput, error) {
		params := store.TargetParams{
			Name:                 in.Name,
			URL:                  in.URL,
			ExpectedStatusMin:    valueOr(in.ExpectedStatusMin, store.DefaultExpectedStatusMin),
			ExpectedStatusMax:    valueOr(in.ExpectedStatusMax, store.DefaultExpectedStatusMax),
			IntervalSeconds:      valueOr(in.IntervalSeconds, store.DefaultIntervalSeconds),
			TimeoutMS:            valueOr(in.TimeoutMS, store.DefaultTimeoutMS),
			ConsecutiveThreshold: valueOr(in.ConsecutiveThreshold, store.DefaultConsecutiveThreshold),
		}
		if err := store.ValidateTargetParams(params); err != nil {
			return nil, targetOutput{}, err
		}
		t, err := s.CreateTarget(ctx, params)
		if err != nil {
			return nil, targetOutput{}, fmt.Errorf("create target: %w", err)
		}
		return nil, toTargetOutput(t), nil
	}
}

func updateTargetHandler(s *store.Store) mcp.ToolHandlerFor[updateTargetInput, targetOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in updateTargetInput) (*mcp.CallToolResult, targetOutput, error) {
		params := store.TargetParams{
			Name:                 in.Name,
			URL:                  in.URL,
			ExpectedStatusMin:    valueOr(in.ExpectedStatusMin, store.DefaultExpectedStatusMin),
			ExpectedStatusMax:    valueOr(in.ExpectedStatusMax, store.DefaultExpectedStatusMax),
			IntervalSeconds:      valueOr(in.IntervalSeconds, store.DefaultIntervalSeconds),
			TimeoutMS:            valueOr(in.TimeoutMS, store.DefaultTimeoutMS),
			ConsecutiveThreshold: valueOr(in.ConsecutiveThreshold, store.DefaultConsecutiveThreshold),
		}
		if err := store.ValidateTargetParams(params); err != nil {
			return nil, targetOutput{}, err
		}
		t, err := s.UpdateTarget(ctx, in.TargetID, params)
		if err != nil {
			return nil, targetOutput{}, fmt.Errorf("update target: %w", err)
		}
		return nil, toTargetOutput(t), nil
	}
}

func deleteTargetHandler(s *store.Store) mcp.ToolHandlerFor[deleteTargetInput, deleteTargetOutput] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in deleteTargetInput) (*mcp.CallToolResult, deleteTargetOutput, error) {
		if err := s.DeleteTarget(ctx, in.TargetID); err != nil {
			return nil, deleteTargetOutput{}, fmt.Errorf("delete target: %w", err)
		}
		return nil, deleteTargetOutput{Deleted: true}, nil
	}
}

func toTargetOutput(t checker.Target) targetOutput {
	return targetOutput{
		ID: t.ID, Name: t.Name, URL: t.URL,
		ExpectedStatusMin: t.ExpectedStatusMin, ExpectedStatusMax: t.ExpectedStatusMax,
		IntervalSeconds: t.IntervalSeconds, TimeoutMS: t.TimeoutMS,
		ConsecutiveThreshold: t.ConsecutiveThreshold, CreatedAt: t.CreatedAt,
	}
}

func valueOr(p *int32, fallback int32) int32 {
	if p == nil {
		return fallback
	}
	return *p
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}
