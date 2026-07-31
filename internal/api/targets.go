package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/nmarques93/kestrel/internal/checker"
	"github.com/nmarques93/kestrel/internal/store"
)

// Defaults mirror the targets table's column defaults (migrations/00001).
const (
	defaultExpectedStatusMin    int32 = 200
	defaultExpectedStatusMax    int32 = 300
	defaultIntervalSeconds      int32 = 60
	defaultTimeoutMS            int32 = 5000
	defaultConsecutiveThreshold int32 = 3
)

type targetResponse struct {
	ID                   int64      `json:"id"`
	Name                 string     `json:"name"`
	URL                  string     `json:"url"`
	ExpectedStatusMin    int32      `json:"expected_status_min"`
	ExpectedStatusMax    int32      `json:"expected_status_max"`
	IntervalSeconds      int32      `json:"interval_seconds"`
	TimeoutMS            int32      `json:"timeout_ms"`
	ConsecutiveThreshold int32      `json:"consecutive_threshold"`
	CreatedAt            time.Time  `json:"created_at"`
	Up                   *bool      `json:"up,omitempty"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
}

func targetToResponse(t checker.Target) targetResponse {
	return targetResponse{
		ID:                   t.ID,
		Name:                 t.Name,
		URL:                  t.URL,
		ExpectedStatusMin:    t.ExpectedStatusMin,
		ExpectedStatusMax:    t.ExpectedStatusMax,
		IntervalSeconds:      t.IntervalSeconds,
		TimeoutMS:            t.TimeoutMS,
		ConsecutiveThreshold: t.ConsecutiveThreshold,
		CreatedAt:            t.CreatedAt,
	}
}

func targetStatusToResponse(ts store.TargetStatus) targetResponse {
	resp := targetToResponse(ts.Target)
	up := ts.Up
	resp.Up = &up
	resp.LastCheckedAt = ts.LastCheckedAt
	return resp
}

// targetRequest is the create/update request body. Pointer fields are
// optional on create (defaulted below) and simply overwrite the existing
// value on update if present... in practice PUT is a full replace, so we
// apply the same defaulting for any field the caller omits.
type targetRequest struct {
	Name                 string `json:"name"`
	URL                  string `json:"url"`
	ExpectedStatusMin    *int32 `json:"expected_status_min"`
	ExpectedStatusMax    *int32 `json:"expected_status_max"`
	IntervalSeconds      *int32 `json:"interval_seconds"`
	TimeoutMS            *int32 `json:"timeout_ms"`
	ConsecutiveThreshold *int32 `json:"consecutive_threshold"`
}

func (req targetRequest) toParams() (store.TargetParams, error) {
	p := store.TargetParams{
		Name:                 req.Name,
		URL:                  req.URL,
		ExpectedStatusMin:    valueOr(req.ExpectedStatusMin, defaultExpectedStatusMin),
		ExpectedStatusMax:    valueOr(req.ExpectedStatusMax, defaultExpectedStatusMax),
		IntervalSeconds:      valueOr(req.IntervalSeconds, defaultIntervalSeconds),
		TimeoutMS:            valueOr(req.TimeoutMS, defaultTimeoutMS),
		ConsecutiveThreshold: valueOr(req.ConsecutiveThreshold, defaultConsecutiveThreshold),
	}
	return p, validateTargetParams(p)
}

func valueOr(p *int32, fallback int32) int32 {
	if p == nil {
		return fallback
	}
	return *p
}

func validateTargetParams(p store.TargetParams) error {
	if p.Name == "" {
		return errors.New("name is required")
	}
	u, err := url.Parse(p.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("url must be a valid http:// or https:// URL")
	}
	if p.ExpectedStatusMin < 100 || p.ExpectedStatusMax > 599 || p.ExpectedStatusMin >= p.ExpectedStatusMax {
		return errors.New("expected_status_min must be less than expected_status_max, within 100-599")
	}
	if p.IntervalSeconds <= 0 {
		return errors.New("interval_seconds must be positive")
	}
	if p.TimeoutMS <= 0 {
		return errors.New("timeout_ms must be positive")
	}
	if p.ConsecutiveThreshold <= 0 {
		return errors.New("consecutive_threshold must be positive")
	}
	return nil
}

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListTargets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list targets: "+err.Error())
		return
	}
	resp := make([]targetResponse, len(targets))
	for i, t := range targets {
		resp[i] = targetStatusToResponse(t)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	var req targetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	params, err := req.toParams()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	t, err := s.store.CreateTarget(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create target: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, targetToResponse(t))
}

func (s *Server) handleGetTarget(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	t, err := s.store.GetTarget(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "get target: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, targetToResponse(t))
}

func (s *Server) handleUpdateTarget(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req targetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	params, err := req.toParams()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	t, err := s.store.UpdateTarget(r.Context(), id, params)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "target not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update target: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, targetToResponse(t))
}

func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.DeleteTarget(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "target not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "delete target: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type checkResponse struct {
	ID         int64     `json:"id"`
	CheckedAt  time.Time `json:"checked_at"`
	Success    bool      `json:"success"`
	StatusCode *int32    `json:"status_code,omitempty"`
	LatencyMS  int64     `json:"latency_ms"`
	Err        *string   `json:"error,omitempty"`
}

func (s *Server) handleListChecks(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit := limitParam(r, 100, 1000)

	checks, err := s.store.ListChecks(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list checks: "+err.Error())
		return
	}
	resp := make([]checkResponse, len(checks))
	for i, c := range checks {
		resp[i] = checkResponse{
			ID: c.ID, CheckedAt: c.CheckedAt, Success: c.Success,
			StatusCode: c.StatusCode, LatencyMS: c.LatencyMS, Err: c.Err,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type uptimeResponse struct {
	WindowHours int     `json:"window_hours"`
	Percent     float64 `json:"percent"`
	SampleSize  int     `json:"sample_size"`
}

func (s *Server) handleUptime(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	windowHours := 24
	if raw := r.URL.Query().Get("window_hours"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			writeError(w, http.StatusBadRequest, "window_hours must be a positive integer")
			return
		}
		windowHours = v
	}

	percent, sampleSize, err := s.store.Uptime(r.Context(), id, time.Now().Add(-time.Duration(windowHours)*time.Hour))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "uptime: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, uptimeResponse{WindowHours: windowHours, Percent: percent, SampleSize: sampleSize})
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q", r.PathValue("id"))
	}
	return id, nil
}

func limitParam(r *http.Request, def, max int) int {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}
