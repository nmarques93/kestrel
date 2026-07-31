//go:build integration

// Run with: make test-integration (requires Docker).
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nmarques93/kestrel/internal/checker"
	"github.com/nmarques93/kestrel/internal/store"
	"github.com/nmarques93/kestrel/internal/testutil"
)

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s := store.New(testutil.NewPool(t))
	server := httptest.NewServer(NewServer(s))
	t.Cleanup(server.Close)
	return server, s
}

func TestTargetsCRUD(t *testing.T) {
	server, _ := newTestServer(t)

	createBody := `{"name":"example","url":"https://example.com","interval_seconds":30}`
	resp, err := http.Post(server.URL+"/api/targets", "application/json", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("POST /api/targets: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/targets status = %d, want 201", resp.StatusCode)
	}
	var created targetResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	resp.Body.Close()

	if created.Name != "example" || created.IntervalSeconds != 30 || created.TimeoutMS != store.DefaultTimeoutMS {
		t.Fatalf("created target = %+v, defaults not applied as expected", created)
	}

	// GET the single target back.
	getResp, err := http.Get(fmt.Sprintf("%s/api/targets/%d", server.URL, created.ID))
	if err != nil {
		t.Fatalf("GET target: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET target status = %d, want 200", getResp.StatusCode)
	}

	// List should include it.
	listResp, err := http.Get(server.URL + "/api/targets")
	if err != nil {
		t.Fatalf("GET /api/targets: %v", err)
	}
	defer listResp.Body.Close()
	var list []targetResponse
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v, want exactly the created target", list)
	}
	if list[0].Up == nil || !*list[0].Up {
		t.Fatal("newly created target with no checks yet should report up=true")
	}

	// Update it.
	updateBody := `{"name":"renamed","url":"https://example.com","interval_seconds":45}`
	req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/targets/%d", server.URL, created.ID), strings.NewReader(updateBody))
	updateResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT target: %v", err)
	}
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT target status = %d, want 200", updateResp.StatusCode)
	}
	var updated targetResponse
	json.NewDecoder(updateResp.Body).Decode(&updated)
	if updated.Name != "renamed" || updated.IntervalSeconds != 45 {
		t.Fatalf("updated target = %+v, want name=renamed interval=45", updated)
	}

	// Delete it.
	delReq, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/targets/%d", server.URL, created.ID), nil)
	delResp, err := http.DefaultClient.Do(delReq)
	if err != nil {
		t.Fatalf("DELETE target: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE target status = %d, want 204", delResp.StatusCode)
	}

	// It's really gone.
	goneResp, err := http.Get(fmt.Sprintf("%s/api/targets/%d", server.URL, created.ID))
	if err != nil {
		t.Fatalf("GET deleted target: %v", err)
	}
	defer goneResp.Body.Close()
	if goneResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted target status = %d, want 404", goneResp.StatusCode)
	}
}

func TestCreateTargetValidation(t *testing.T) {
	server, _ := newTestServer(t)

	cases := []string{
		`{"name":"","url":"https://example.com"}`,                        // empty name
		`{"name":"x","url":"not-a-url"}`,                                 // bad url
		`{"name":"x","url":"ftp://example.com"}`,                         // wrong scheme
		`{"name":"x","url":"https://example.com","interval_seconds":-1}`, // bad interval
	}
	for _, body := range cases {
		resp, err := http.Post(server.URL+"/api/targets", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST /api/targets: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("body %q: status = %d, want 422", body, resp.StatusCode)
		}
	}
}

func TestChecksIncidentsAndUptimeEndpoints(t *testing.T) {
	server, s := newTestServer(t)

	created, err := s.CreateTarget(t.Context(), store.TargetParams{
		Name: "flaky", URL: "https://example.com",
		ExpectedStatusMin: 200, ExpectedStatusMax: 300,
		IntervalSeconds: 60, TimeoutMS: 5000, ConsecutiveThreshold: 2,
	})
	if err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}

	now := time.Now()
	record := func(success bool, offset time.Duration) {
		t.Helper()
		errText := ""
		if !success {
			errText = "boom"
		}
		if err := s.Record(t.Context(), checker.Result{
			TargetID: created.ID, CheckedAt: now.Add(offset), Success: success,
			StatusCode: 200, LatencyMS: 5, Err: errText,
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	record(true, 0)
	record(false, time.Second)
	record(false, 2*time.Second) // trips the incident (threshold 2)

	// /checks
	checksResp, err := http.Get(fmt.Sprintf("%s/api/targets/%d/checks", server.URL, created.ID))
	if err != nil {
		t.Fatalf("GET checks: %v", err)
	}
	defer checksResp.Body.Close()
	var checks []checkResponse
	json.NewDecoder(checksResp.Body).Decode(&checks)
	if len(checks) != 3 {
		t.Fatalf("checks = %d, want 3", len(checks))
	}

	// /incidents (target-scoped)
	incResp, err := http.Get(fmt.Sprintf("%s/api/targets/%d/incidents", server.URL, created.ID))
	if err != nil {
		t.Fatalf("GET target incidents: %v", err)
	}
	defer incResp.Body.Close()
	var incidents []incidentResponse
	json.NewDecoder(incResp.Body).Decode(&incidents)
	if len(incidents) != 1 || incidents[0].ResolvedAt != nil {
		t.Fatalf("incidents = %+v, want exactly one open incident", incidents)
	}

	// /api/incidents (global)
	globalResp, err := http.Get(server.URL + "/api/incidents")
	if err != nil {
		t.Fatalf("GET /api/incidents: %v", err)
	}
	defer globalResp.Body.Close()
	var globalIncidents []incidentResponse
	json.NewDecoder(globalResp.Body).Decode(&globalIncidents)
	if len(globalIncidents) != 1 {
		t.Fatalf("global incidents = %+v, want exactly one", globalIncidents)
	}

	// /uptime
	uptimeResp, err := http.Get(fmt.Sprintf("%s/api/targets/%d/uptime?window_hours=1", server.URL, created.ID))
	if err != nil {
		t.Fatalf("GET uptime: %v", err)
	}
	defer uptimeResp.Body.Close()
	var uptime uptimeResponse
	json.NewDecoder(uptimeResp.Body).Decode(&uptime)
	if uptime.SampleSize != 3 {
		t.Fatalf("uptime sample size = %d, want 3", uptime.SampleSize)
	}
	wantPercent := 100.0 / 3.0
	if diff := uptime.Percent - wantPercent; diff > 0.01 || diff < -0.01 {
		t.Fatalf("uptime percent = %v, want ~%v", uptime.Percent, wantPercent)
	}
}

func TestStatusPagesRender(t *testing.T) {
	server, s := newTestServer(t)

	if _, err := s.CreateTarget(t.Context(), store.TargetParams{
		Name: "example", URL: "https://example.com",
		ExpectedStatusMin: 200, ExpectedStatusMax: 300,
		IntervalSeconds: 60, TimeoutMS: 5000, ConsecutiveThreshold: 3,
	}); err != nil {
		t.Fatalf("CreateTarget: %v", err)
	}

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", resp.StatusCode)
	}
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if !strings.Contains(buf.String(), "example") {
		t.Error("targets page doesn't mention the target's name")
	}

	incResp, err := http.Get(server.URL + "/incidents")
	if err != nil {
		t.Fatalf("GET /incidents: %v", err)
	}
	defer incResp.Body.Close()
	if incResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /incidents status = %d, want 200", incResp.StatusCode)
	}
}
