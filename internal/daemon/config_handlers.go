package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
)

var errConfigUnavailable = errors.New("configuration unavailable")

type invalidConfigError struct{ err error }

func (e invalidConfigError) Error() string { return e.err.Error() }

type configView struct {
	Config          FileConfig `json:"config"`
	Effective       FileConfig `json:"effective"`
	Path            string     `json:"path"`
	Exists          bool       `json:"exists"`
	RestartRequired bool       `json:"restart_required"`
	Desktop         bool       `json:"desktop"`
	MCPStatus       MCPStatus  `json:"mcp_status"`
}

func (s *Server) configViewLocked() configView {
	_, err := os.Stat(s.configPath)
	return configView{Config: s.savedConfig, Effective: s.effectiveConfig, Path: s.configPath,
		Exists: err == nil, RestartRequired: s.savedConfig != s.effectiveConfig, Desktop: s.desktop,
		MCPStatus: s.mcpStatus}
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configPath == "" {
		writeError(w, http.StatusServiceUnavailable, "configuration unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.configViewLocked())
}

func (s *Server) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	// This endpoint can change file destinations. Reject cross-origin browser writes.
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
			writeError(w, http.StatusForbidden, "cross-origin configuration writes are not allowed")
			return
		}
	}
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	var config FileConfig
	if err := decoder.Decode(&config); err != nil {
		writeError(w, http.StatusBadRequest, "invalid configuration: "+err.Error())
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeError(w, http.StatusBadRequest, "expected one configuration object")
		return
	}
	view, err := s.saveFileConfig(config)
	if err != nil {
		status := http.StatusInternalServerError
		var invalid invalidConfigError
		if errors.As(err, &invalid) {
			status = http.StatusBadRequest
		} else if errors.Is(err, errConfigUnavailable) {
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) saveFileConfig(config FileConfig) (configView, error) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	return s.saveFileConfigLocked(config)
}

// saveFileConfigLocked persists one complete configuration and hot-applies MCP.
// The caller must hold configMu so read-modify-write callers cannot lose updates.
func (s *Server) saveFileConfigLocked(config FileConfig) (configView, error) {
	if config.MCP.Enabled && config.MCP.Token == "" {
		token, err := newMCPToken()
		if err != nil {
			return configView{}, fmt.Errorf("generate MCP token: %w", err)
		}
		config.MCP.Token = token
	}

	s.mu.Lock()
	if s.configPath == "" {
		s.mu.Unlock()
		return configView{}, errConfigUnavailable
	}
	path, effective, status, applyMCP := s.configPath, s.effectiveConfig, s.mcpStatus, s.applyMCP
	s.mu.Unlock()
	if err := config.validate(path); err != nil {
		return configView{}, invalidConfigError{err: err}
	}
	apply := applyMCP != nil && (config.MCP != effective.MCP || config.MCP.Enabled && !status.Running)
	if apply {
		if err := applyMCP(config.MCP); err != nil {
			return configView{}, fmt.Errorf("apply MCP configuration: %w", err)
		}
	}
	if err := saveConfig(path, config); err != nil {
		message := "save configuration: " + err.Error()
		if apply {
			rollback := effective.MCP
			if !status.Running {
				rollback.Enabled = false
			}
			if rollbackErr := applyMCP(rollback); rollbackErr != nil {
				message = fmt.Sprintf("%s; restore MCP configuration: %v", message, rollbackErr)
			} else {
				s.mu.Lock()
				s.mcpStatus = status
				s.mu.Unlock()
			}
		}
		return configView{}, errors.New(message)
	}
	s.mu.Lock()
	s.savedConfig = config
	if apply {
		s.effectiveConfig.MCP = config.MCP
	}
	view := s.configViewLocked()
	s.mu.Unlock()
	return view, nil
}
