package daemon

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"ssh-tunnel-manager/internal/tunnel"
)

type connectionRecord struct {
	Tunnel string `json:"tunnel"`
	tunnel.Connection
	EndedAt *time.Time `json:"ended_at,omitempty"`
	Error   string     `json:"error,omitempty"`
}

type connectionsView struct {
	Connections      []connectionRecord `json:"connections"`
	Tunnels          []string           `json:"tunnels"`
	Total            int                `json:"total"`
	Limit            int                `json:"limit"`
	Persisted        bool               `json:"persisted"`
	PersistenceError string             `json:"persistence_error,omitempty"`
}

func (s *Server) handleAllConnections(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	limit, err := viewLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	kind, state := r.URL.Query().Get("kind"), r.URL.Query().Get("state")
	if kind != "active" && kind != "history" {
		writeError(w, http.StatusBadRequest, "kind must be active or history")
		return
	}
	if state != "" && ((kind == "active" && state != "connecting" && state != "connected") || (kind == "history" && state != "closed" && state != "failed")) {
		writeError(w, http.StatusBadRequest, "invalid connection state")
		return
	}
	result := s.connectionsView(kind == "history", r.URL.Query().Get("tunnel"), state, r.URL.Query().Get("q"), limit)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) connectionsView(history bool, name, state, query string, limit int) connectionsView {
	s.mu.Lock()
	result := connectionsView{Connections: []connectionRecord{}, Tunnels: []string{}, Limit: limit, Persisted: s.history != nil}
	query = strings.ToLower(query)
	names := map[string]bool{}
	appendRow := func(row connectionRecord) {
		names[row.Tunnel] = true
		if name != "" && row.Tunnel != name || state != "" && row.State != state {
			return
		}
		if !strings.Contains(strings.ToLower(strings.Join([]string{row.Tunnel, row.Source, row.Target, row.Error, row.ID}, " ")), query) {
			return
		}
		result.Connections = append(result.Connections, row)
	}
	appendHistory := func(name string, row tunnel.ConnectionHistoryEntry) {
		ended := row.EndedAt
		appendRow(connectionRecord{Tunnel: name, Connection: row.Connection, EndedAt: &ended, Error: row.Error})
	}
	for name := range s.state.Tunnels {
		names[name] = true
	}
	if history && s.history != nil {
		// Include retained history even if its tunnel has since been deleted.
		s.history.mu.Lock()
		for name, rows := range s.history.rows {
			for _, row := range rows {
				appendHistory(name, row)
			}
		}
		result.PersistenceError = s.history.lastError
		s.history.mu.Unlock()
	} else {
		for name := range s.state.Tunnels {
			if history {
				for _, row := range s.manager.ConnectionHistory(name) {
					appendHistory(name, row)
				}
			} else {
				for _, row := range s.manager.Connections(name) {
					appendRow(connectionRecord{Tunnel: name, Connection: row})
				}
			}
		}
	}
	s.mu.Unlock()
	sort.Slice(result.Connections, func(i, j int) bool {
		a, b := result.Connections[i], result.Connections[j]
		ta, tb := a.StartedAt, b.StartedAt
		if history {
			ta, tb = *a.EndedAt, *b.EndedAt
		}
		if !ta.Equal(tb) {
			return ta.After(tb)
		}
		if a.Tunnel != b.Tunnel {
			return a.Tunnel < b.Tunnel
		}
		return a.ID < b.ID
	})
	result.Total = len(result.Connections)
	if len(result.Connections) > limit {
		result.Connections = result.Connections[:limit]
	}
	for name := range names {
		result.Tunnels = append(result.Tunnels, name)
	}
	sort.Strings(result.Tunnels)
	return result
}
