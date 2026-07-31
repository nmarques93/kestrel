package api

import (
	"net/http"
	"strings"
)

// requireAPIKey gates access behind a valid MCP token (see
// store.CreateMCPToken / cmd/mcptoken). Only the MCP endpoint is
// protected — the REST API and status page are intentionally open, per
// the single-operator MVP scope.
func (s *Server) requireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kestrel"`)
			writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header (want: Bearer <token>)")
			return
		}
		valid, err := s.store.AuthenticateMCPToken(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "authenticate: "+err.Error())
			return
		}
		if !valid {
			w.Header().Set("WWW-Authenticate", `Bearer realm="kestrel"`)
			writeError(w, http.StatusUnauthorized, "invalid API token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	return token, token != ""
}
