package api

import (
	"net/http"
	"time"

	"github.com/nmarques93/kestrel/internal/store"
)

type incidentResponse struct {
	ID              int64      `json:"id"`
	TargetID        int64      `json:"target_id"`
	TargetName      string     `json:"target_name"`
	StartedAt       time.Time  `json:"started_at"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	Cause           *string    `json:"cause,omitempty"`
	DurationSeconds *float64   `json:"duration_seconds,omitempty"`
}

func (s *Server) handleListIncidents(w http.ResponseWriter, r *http.Request) {
	limit := limitParam(r, 100, 1000)

	incidents, err := s.store.ListIncidents(r.Context(), nil, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list incidents: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, incidentsToResponse(incidents))
}

func incidentsToResponse(incidents []store.Incident) []incidentResponse {
	resp := make([]incidentResponse, len(incidents))
	for i, inc := range incidents {
		resp[i] = incidentResponse{
			ID: inc.ID, TargetID: inc.TargetID, TargetName: inc.TargetName,
			StartedAt: inc.StartedAt, ResolvedAt: inc.ResolvedAt, Cause: inc.Cause,
			DurationSeconds: inc.DurationSeconds(),
		}
	}
	return resp
}

func (s *Server) handleListTargetIncidents(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit := limitParam(r, 100, 1000)

	incidents, err := s.store.ListIncidents(r.Context(), &id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list incidents: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, incidentsToResponse(incidents))
}
