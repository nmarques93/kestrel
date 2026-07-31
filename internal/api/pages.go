package api

import (
	"fmt"
	"net/http"
	"time"
)

type targetView struct {
	Name            string
	URL             string
	Up              bool
	LastCheckedAt   *time.Time
	HasUptimeSample bool
	UptimePercent   float64
}

type targetsPageData struct {
	Title   string
	Targets []targetView
}

func (s *Server) handleTargetsPage(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.ListTargets(r.Context())
	if err != nil {
		http.Error(w, "list targets: "+err.Error(), http.StatusInternalServerError)
		return
	}

	since := time.Now().Add(-24 * time.Hour)
	views := make([]targetView, len(targets))
	for i, t := range targets {
		percent, sampleSize, err := s.store.Uptime(r.Context(), t.ID, since)
		if err != nil {
			http.Error(w, "uptime: "+err.Error(), http.StatusInternalServerError)
			return
		}
		views[i] = targetView{
			Name: t.Name, URL: t.URL, Up: t.Up, LastCheckedAt: t.LastCheckedAt,
			HasUptimeSample: sampleSize > 0, UptimePercent: percent,
		}
	}

	s.render(w, "targets.html", targetsPageData{Title: "Targets", Targets: views})
}

type incidentView struct {
	TargetName string
	StartedAt  time.Time
	ResolvedAt *time.Time
	Duration   string
	Cause      *string
}

type incidentsPageData struct {
	Title     string
	Incidents []incidentView
}

func (s *Server) handleIncidentsPage(w http.ResponseWriter, r *http.Request) {
	incidents, err := s.store.ListIncidents(r.Context(), nil, 200)
	if err != nil {
		http.Error(w, "list incidents: "+err.Error(), http.StatusInternalServerError)
		return
	}

	views := make([]incidentView, len(incidents))
	for i, inc := range incidents {
		end := time.Now()
		if inc.ResolvedAt != nil {
			end = *inc.ResolvedAt
		}
		views[i] = incidentView{
			TargetName: inc.TargetName, StartedAt: inc.StartedAt, ResolvedAt: inc.ResolvedAt,
			Duration: formatDuration(end.Sub(inc.StartedAt)), Cause: inc.Cause,
		}
	}

	s.render(w, "incidents.html", incidentsPageData{Title: "Incidents", Incidents: views})
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
