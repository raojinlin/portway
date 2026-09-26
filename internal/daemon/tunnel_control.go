package daemon

import (
	"fmt"
	"net/http"

	"ssh-tunnel-manager/internal/store"
)

func (s *Server) createTunnel(req TunnelRequest) (TunnelView, int, error) {
	cfg, err := req.toConfig()
	if err != nil {
		return TunnelView{}, http.StatusBadRequest, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.state.Tunnels[cfg.Name]; exists {
		return TunnelView{}, http.StatusConflict, fmt.Errorf("tunnel already exists")
	}
	if err := s.manager.Add(s.tunnelContext(), cfg); err != nil {
		return TunnelView{}, http.StatusInternalServerError, fmt.Errorf("start tunnel: %w", err)
	}
	s.state.Tunnels[cfg.Name] = store.Entry{Config: cfg, Enabled: true}
	if err := s.store.Save(s.state); err != nil {
		delete(s.state.Tunnels, cfg.Name)
		_ = s.manager.Remove(cfg.Name)
		return TunnelView{}, http.StatusInternalServerError, fmt.Errorf("persist state: %w", err)
	}
	v, _ := s.viewLocked(cfg.Name)
	return v, http.StatusCreated, nil
}

func (s *Server) updateTunnel(name string, req TunnelRequest) (TunnelView, int, error) {
	if req.Name != "" && req.Name != name {
		return TunnelView{}, http.StatusBadRequest, fmt.Errorf("tunnel name cannot be changed")
	}
	req.Name = name

	s.mu.Lock()
	defer s.mu.Unlock()
	oldEntry, exists := s.state.Tunnels[name]
	if !exists {
		return TunnelView{}, http.StatusNotFound, fmt.Errorf("not found")
	}
	if req.SSHPassword == "" {
		req.SSHPassword = oldEntry.Config.SSHPassword
	}
	if req.ServiceIcon == "" {
		req.ServiceIcon = oldEntry.Config.ServiceIcon
	}
	cfg, err := req.toConfig()
	if err != nil {
		return TunnelView{}, http.StatusBadRequest, err
	}

	previousConfig := oldEntry.Config
	previousConfig.ServiceIcon = cfg.ServiceIcon
	restart := oldEntry.Enabled && previousConfig != cfg
	if restart {
		_ = s.manager.Remove(name)
		if err := s.manager.Add(s.tunnelContext(), cfg); err != nil {
			_ = s.manager.Add(s.tunnelContext(), oldEntry.Config)
			return TunnelView{}, http.StatusInternalServerError, fmt.Errorf("restart tunnel: %w", err)
		}
	}
	s.state.Tunnels[name] = store.Entry{Config: cfg, Enabled: oldEntry.Enabled}
	if err := s.store.Save(s.state); err != nil {
		s.state.Tunnels[name] = oldEntry
		if restart {
			_ = s.manager.Remove(name)
			_ = s.manager.Add(s.tunnelContext(), oldEntry.Config)
		}
		return TunnelView{}, http.StatusInternalServerError, fmt.Errorf("persist state: %w", err)
	}
	v, _ := s.viewLocked(name)
	return v, http.StatusOK, nil
}

func (s *Server) deleteTunnel(name string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.state.Tunnels[name]
	if !exists {
		return http.StatusNotFound, fmt.Errorf("not found")
	}

	_ = s.manager.Remove(name)
	delete(s.state.Tunnels, name)
	if err := s.store.Save(s.state); err != nil {
		s.state.Tunnels[name] = entry
		if entry.Enabled {
			_ = s.manager.Add(s.tunnelContext(), entry.Config)
		}
		return http.StatusInternalServerError, fmt.Errorf("persist state: %w", err)
	}
	return http.StatusNoContent, nil
}
