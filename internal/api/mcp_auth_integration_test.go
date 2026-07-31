//go:build integration

// Run with: make test-integration (requires Docker).
package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// authRoundTripper injects a fixed Authorization header into every request,
// standing in for how a real MCP client would authenticate.
type authRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (rt authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+rt.token)
	return rt.base.RoundTrip(req)
}

func TestMCPEndpointRejectsMissingOrInvalidToken(t *testing.T) {
	server, _ := newTestServer(t)

	resp, err := http.Post(server.URL+"/mcp", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST /mcp without Authorization: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/mcp", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp with bogus token: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Errorf("bogus token: status = %d, want 401", resp2.StatusCode)
	}
}

func TestMCPEndpointAcceptsValidTokenAndServesTools(t *testing.T) {
	server, s := newTestServer(t)

	token, err := s.CreateMCPToken(t.Context())
	if err != nil {
		t.Fatalf("CreateMCPToken: %v", err)
	}

	httpClient := &http.Client{Transport: authRoundTripper{token: token, base: http.DefaultTransport}}
	transport := &mcp.StreamableClientTransport{
		Endpoint: server.URL + "/mcp", HTTPClient: httpClient,
		// The test only needs request/response; skip the persistent SSE
		// stream so session.Close() doesn't have to tear down a long-lived
		// GET connection.
		DisableStandaloneSSE: true,
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("client.Connect (with valid token): %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_targets"})
	if err != nil {
		t.Fatalf("CallTool(list_targets): %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool(list_targets) returned a tool error: %+v", result.Content)
	}
}
