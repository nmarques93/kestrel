// Package api exposes the HTTP API and the server-rendered status page.
// Every handler here is a thin wrapper over internal/store — the same
// methods the checker engine and (later) the MCP server use — so the
// business logic isn't duplicated across layers.
package api

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nmarques93/kestrel/internal/mcpserver"
	"github.com/nmarques93/kestrel/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

// Server holds the shared dependencies for every handler and implements
// http.Handler so it can be handed straight to http.Server.
type Server struct {
	store *store.Store
	// Each page gets its own layout.html + content template combined into
	// one *template.Template, keyed by content file name. They can't share
	// a single parsed set because every content file defines a block named
	// "content" — combining them all would let the last one parsed win for
	// every page.
	pages     map[string]*template.Template
	mux       *http.ServeMux
	mcpServer *mcp.Server
}

func NewServer(s *store.Store) *Server {
	srv := &Server{
		store: s,
		pages: map[string]*template.Template{
			"targets.html":   template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/targets.html")),
			"incidents.html": template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/incidents.html")),
		},
		mcpServer: mcpserver.New(s),
	}
	srv.mux = srv.routes()
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	lw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
	s.mux.ServeHTTP(lw, r)
	log.Printf("%s %s %d %s", r.Method, r.URL.Path, lw.status, time.Since(start))
}

func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/targets", s.handleListTargets)
	mux.HandleFunc("POST /api/targets", s.handleCreateTarget)
	mux.HandleFunc("GET /api/targets/{id}", s.handleGetTarget)
	mux.HandleFunc("PUT /api/targets/{id}", s.handleUpdateTarget)
	mux.HandleFunc("DELETE /api/targets/{id}", s.handleDeleteTarget)
	mux.HandleFunc("GET /api/targets/{id}/checks", s.handleListChecks)
	mux.HandleFunc("GET /api/targets/{id}/incidents", s.handleListTargetIncidents)
	mux.HandleFunc("GET /api/targets/{id}/uptime", s.handleUptime)
	mux.HandleFunc("GET /api/incidents", s.handleListIncidents)

	mux.HandleFunc("GET /{$}", s.handleTargetsPage)
	mux.HandleFunc("GET /incidents", s.handleIncidentsPage)

	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.mcpServer }, nil)
	mux.Handle("/mcp", s.requireAPIKey(mcpHandler))

	return mux
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	tmpl, ok := s.pages[name]
	if !ok {
		http.Error(w, "unknown page template "+name, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
