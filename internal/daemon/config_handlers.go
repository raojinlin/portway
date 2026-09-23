package daemon

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
)

type configView struct {
	Config          FileConfig `json:"config"`
	Effective       FileConfig `json:"effective"`
	Path            string     `json:"path"`
	Exists          bool       `json:"exists"`
	RestartRequired bool       `json:"restart_required"`
	Desktop         bool       `json:"desktop"`
}

func (s *Server) configViewLocked() configView {
	_, err := os.Stat(s.configPath)
	return configView{Config: s.savedConfig, Effective: s.effectiveConfig, Path: s.configPath,
		Exists: err == nil, RestartRequired: s.savedConfig != s.effectiveConfig, Desktop: s.desktop}
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configPath == "" {
		writeError(w, http.StatusServiceUnavailable, "configuration unavailable")
		return
	}
	if err := config.validate(s.configPath); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := saveConfig(s.configPath, config); err != nil {
		writeError(w, http.StatusInternalServerError, "save configuration: "+err.Error())
		return
	}
	s.savedConfig = config
	writeJSON(w, http.StatusOK, s.configViewLocked())
}
