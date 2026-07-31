// Package mcpserver exposes Kestrel's data over MCP: read tools for
// status/history/uptime and write tools for managing targets. Every
// handler is a thin call into internal/store — the same methods the REST
// API uses — so an agent and a human operator see identical behavior.
package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nmarques93/kestrel/internal/store"
)

// New builds an MCP server with every Kestrel tool registered against s.
func New(s *store.Store) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "kestrel", Version: "0.1.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_targets",
		Description: "List every monitored target with its current up/down status, last check time, and configuration.",
	}, listTargetsHandler(s))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_checks",
		Description: "List recent check results for one target, newest first.",
	}, listChecksHandler(s))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_incidents",
		Description: "List incidents (DOWN periods with start/end time and cause). Pass target_id to scope to one target, or omit it for the timeline across every target.",
	}, listIncidentsHandler(s))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_uptime",
		Description: "Get the uptime percentage for one target over a recent time window.",
	}, getUptimeHandler(s))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_target",
		Description: "Register a new HTTP(S) target to monitor.",
	}, createTargetHandler(s))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_target",
		Description: "Replace an existing target's configuration (name, URL, check interval, timeout, expected status range, flap-prevention threshold).",
	}, updateTargetHandler(s))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_target",
		Description: "Stop monitoring a target and delete its check/incident history.",
	}, deleteTargetHandler(s))

	return server
}
