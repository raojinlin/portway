package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"ssh-tunnel-manager/internal/tunnel"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handleSaveConfig)
	mux.HandleFunc("GET /api/logs", s.handleLogs)
	mux.HandleFunc("GET /api/connections", s.handleAllConnections)
	mux.HandleFunc("GET /api/skills/portway/download", s.handleDownloadSkill)

	mux.HandleFunc("GET /api/tunnels", s.handleList)
	mux.HandleFunc("POST /api/tunnels", s.handleCreate)
	mux.HandleFunc("GET /api/tunnels/{name}", s.handleGet)
	mux.HandleFunc("GET /api/tunnels/{name}/connections", s.handleConnections)
	mux.HandleFunc("GET /api/tunnels/{name}/connections/history", s.handleConnectionHistory)
	mux.HandleFunc("PUT /api/tunnels/{name}", s.handleUpdate)
	mux.HandleFunc("DELETE /api/tunnels/{name}", s.handleDelete)
	mux.HandleFunc("POST /api/tunnels/{name}/start", s.handleStart)
	mux.HandleFunc("POST /api/tunnels/{name}/stop", s.handleStop)

	mux.Handle("/", webUIHandler())

	return s.logRequests(mux)
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req TunnelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	v, status, err := s.updateTunnel(name, req)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, status, v)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// viewLocked builds a TunnelView for name from current state; caller must
// hold s.mu.
func (s *Server) viewLocked(name string) (TunnelView, bool) {
	entry, ok := s.state.Tunnels[name]
	if !ok {
		return TunnelView{}, false
	}
	status, running := s.manager.Status(name)
	if !running {
		status.Name = name
		status.State = "stopped"
	}
	return newTunnelView(entry.Config, entry.Enabled, status), true
}

func (s *Server) tunnelViews() []TunnelView {
	s.mu.Lock()
	defer s.mu.Unlock()

	names := make([]string, 0, len(s.state.Tunnels))
	for name := range s.state.Tunnels {
		names = append(names, name)
	}
	sort.Strings(names)

	views := make([]TunnelView, 0, len(names))
	for _, name := range names {
		v, _ := s.viewLocked(name)
		views = append(views, v)
	}
	return views
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.tunnelViews())
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.viewLocked(name)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.Tunnels[name]; !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, s.manager.Connections(name))
}

func (s *Server) handleConnectionHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.state.Tunnels[name]; !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	rows := s.manager.ConnectionHistory(name)
	var persistenceError string
	if s.history != nil {
		rows, persistenceError = s.history.list(name)
	}
	writeJSON(w, http.StatusOK, struct {
		Connections      []tunnel.ConnectionHistoryEntry `json:"connections"`
		Limit            int                             `json:"limit"`
		Persisted        bool                            `json:"persisted"`
		PersistenceError string                          `json:"persistence_error,omitempty"`
	}{rows, tunnel.ConnectionHistoryLimit, s.history != nil, persistenceError})
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req TunnelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	v, status, err := s.createTunnel(req)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, status, v)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	status, err := s.deleteTunnel(r.PathValue("name"))
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	w.WriteHeader(status)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.handleSetEnabled(w, r, false)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	s.handleSetEnabled(w, r, true)
}

func (s *Server) handleSetEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	v, status, err := s.setTunnelEnabled(r.PathValue("name"), enabled)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) setTunnelEnabled(name string, enabled bool) (TunnelView, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.state.Tunnels[name]
	if !exists {
		return TunnelView{}, http.StatusNotFound, fmt.Errorf("tunnel %q not found", name)
	}
	if enabled {
		// A stale menu click must not restart an already active instance.
		if _, running := s.manager.Status(name); !running {
			if err := s.manager.Add(s.tunnelContext(), entry.Config); err != nil {
				return TunnelView{}, http.StatusInternalServerError, fmt.Errorf("start tunnel: %w", err)
			}
		}
	} else {
		_ = s.manager.Remove(name)
	}
	entry.Enabled = enabled
	s.state.Tunnels[name] = entry
	if err := s.store.Save(s.state); err != nil {
		return TunnelView{}, http.StatusInternalServerError, fmt.Errorf("persist state: %w", err)
	}
	v, _ := s.viewLocked(name)
	return v, http.StatusOK, nil
}
