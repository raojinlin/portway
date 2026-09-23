package daemon

import (
	"encoding/json"
	"net/http"
	"sort"

	"ssh-tunnel-manager/internal/store"
	"ssh-tunnel-manager/internal/tunnel"
)

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", s.handleGetConfig)
	mux.HandleFunc("PUT /api/config", s.handleSaveConfig)

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
	if req.Name != "" && req.Name != name {
		writeError(w, http.StatusBadRequest, "tunnel name cannot be changed")
		return
	}
	req.Name = name

	s.mu.Lock()
	defer s.mu.Unlock()

	oldEntry, exists := s.state.Tunnels[name]
	if !exists {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	// Passwords are never returned to the browser. An empty password on edit
	// therefore means "keep the stored password" rather than erase it.
	if req.SSHPassword == "" {
		req.SSHPassword = oldEntry.Config.SSHPassword
	}
	cfg, err := req.toConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if oldEntry.Enabled {
		_ = s.manager.Remove(name)
		if err := s.manager.Add(s.tunnelContext(), cfg); err != nil {
			_ = s.manager.Add(s.tunnelContext(), oldEntry.Config)
			writeError(w, http.StatusInternalServerError, "restart tunnel: "+err.Error())
			return
		}
	}

	s.state.Tunnels[name] = store.Entry{Config: cfg, Enabled: oldEntry.Enabled}
	if err := s.store.Save(s.state); err != nil {
		s.state.Tunnels[name] = oldEntry
		if oldEntry.Enabled {
			_ = s.manager.Remove(name)
			_ = s.manager.Add(s.tunnelContext(), oldEntry.Config)
		}
		writeError(w, http.StatusInternalServerError, "persist state: "+err.Error())
		return
	}

	v, _ := s.viewLocked(name)
	writeJSON(w, http.StatusOK, v)
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
	cfg, err := req.toConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.state.Tunnels[cfg.Name]; exists {
		writeError(w, http.StatusConflict, "tunnel already exists")
		return
	}

	if err := s.manager.Add(s.tunnelContext(), cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "start tunnel: "+err.Error())
		return
	}

	s.state.Tunnels[cfg.Name] = store.Entry{Config: cfg, Enabled: true}
	if err := s.store.Save(s.state); err != nil {
		writeError(w, http.StatusInternalServerError, "persist state: "+err.Error())
		return
	}

	v, _ := s.viewLocked(cfg.Name)
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.state.Tunnels[name]; !exists {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	_ = s.manager.Remove(name) // may already be stopped; ignore "not found"
	delete(s.state.Tunnels, name)
	if err := s.store.Save(s.state); err != nil {
		writeError(w, http.StatusInternalServerError, "persist state: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.state.Tunnels[name]
	if !exists {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	_ = s.manager.Remove(name)
	entry.Enabled = false
	s.state.Tunnels[name] = entry
	if err := s.store.Save(s.state); err != nil {
		writeError(w, http.StatusInternalServerError, "persist state: "+err.Error())
		return
	}

	v, _ := s.viewLocked(name)
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.state.Tunnels[name]
	if !exists {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	if err := s.manager.Add(s.tunnelContext(), entry.Config); err != nil {
		writeError(w, http.StatusInternalServerError, "start tunnel: "+err.Error())
		return
	}

	entry.Enabled = true
	s.state.Tunnels[name] = entry
	if err := s.store.Save(s.state); err != nil {
		writeError(w, http.StatusInternalServerError, "persist state: "+err.Error())
		return
	}

	v, _ := s.viewLocked(name)
	writeJSON(w, http.StatusOK, v)
}
