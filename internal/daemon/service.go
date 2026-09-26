package daemon

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"ssh-tunnel-manager/internal/logging"
	"ssh-tunnel-manager/internal/processlock"
	"ssh-tunnel-manager/internal/store"
	"ssh-tunnel-manager/internal/tunnel"
)

// Service is the common backend for the HTTP daemon and the in-process desktop app.
// It does not open a TCP port. Its owner must call Close before exiting.
type Service struct {
	server    *Server
	handler   http.Handler
	logger    *slog.Logger
	cancel    context.CancelFunc
	resources []io.Closer
	requests  sync.RWMutex
	closing   atomic.Bool
	closeOnce sync.Once
	closeErr  error
	mcpMu     sync.Mutex
	mcp       *mcpRuntime
}

func Open(ctx context.Context, opts Options) (_ *Service, err error) {
	saved, effective, path, err := loadConfig(opts)
	if err != nil {
		return nil, err
	}
	return openWithConfig(ctx, opts, saved, effective, path)
}

func openWithConfig(ctx context.Context, opts Options, saved, effective FileConfig, path string) (_ *Service, err error) {
	resolve := func(value string) (string, error) { return configPath(value, filepath.Dir(path)) }
	statePath, err := resolve(effective.StatePath)
	if err != nil {
		return nil, err
	}
	historyPath, err := resolve(effective.ConnectionLog)
	if err != nil {
		return nil, err
	}
	var logPath string
	if effective.LogFile != "" {
		logPath, err = resolve(effective.LogFile)
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{cancel: cancel}
	defer func() {
		if err != nil {
			cancel()
			for n := len(s.resources) - 1; n >= 0; n-- {
				s.resources[n].Close()
			}
		}
	}()
	for _, target := range []string{path, statePath, historyPath, logPath} {
		if target == "" {
			continue
		}
		lock, lockErr := processlock.Acquire(target)
		if lockErr != nil {
			return nil, lockErr
		}
		s.resources = append(s.resources, lock)
	}
	var output io.Writer = os.Stderr
	if logPath != "" {
		file, openErr := logging.OpenRotating(logPath, int64(effective.LogMaxSizeMB)*1024*1024, effective.LogMaxBackups)
		if openErr != nil {
			return nil, openErr
		}
		s.resources = append(s.resources, file)
		output = logging.Mirror(os.Stderr, file)
	}
	s.logger, err = logging.New(output, effective.LogLevel, effective.LogFormat)
	if err != nil {
		return nil, err
	}
	ctx = logging.WithLogger(ctx, s.logger)
	history, err := openHistory(historyPath, int64(effective.LogMaxSizeMB)*1024*1024, effective.LogMaxBackups, s.logger)
	if err != nil {
		return nil, err
	}
	s.resources = append(s.resources, history.writer)
	ctx = tunnel.WithHistoryRecorder(ctx, history.record)
	st := store.New(statePath)
	loaded, err := st.Load()
	if err != nil {
		return nil, err
	}
	s.server = &Server{ctx: ctx, store: st, state: loaded, manager: tunnel.NewManager(), configPath: path,
		savedConfig: saved, effectiveConfig: effective, history: history, desktop: opts.Desktop}
	s.server.applyMCP = s.applyMCP
	s.handler = s.server.routes()
	s.logger.Info("backend starting", "state_file", statePath, "config_file", path, "connection_log", historyPath)
	enabled := 0
	for name, entry := range loaded.Tunnels {
		if !entry.Enabled {
			continue
		}
		enabled++
		if err := s.server.manager.Add(ctx, entry.Config); err != nil {
			s.logger.Error("tunnel restore failed", "tunnel", name, "error", logging.SafeError(err, entry.Config.SSHPassword))
		}
	}
	s.logger.Info("daemon state loaded", "tunnels", len(loaded.Tunnels), "enabled", enabled)
	if startErr := s.applyMCP(effective.MCP); startErr != nil {
		s.logger.Error("MCP server start failed", "address", effective.MCP.Addr, "error", startErr)
	}
	return s, nil
}

func (s *Service) setMCPStatus(status MCPStatus) {
	s.server.mu.Lock()
	s.server.mcpStatus = status
	s.server.mu.Unlock()
}

func (s *Service) applyMCP(config MCPConfig) error {
	s.mcpMu.Lock()
	defer s.mcpMu.Unlock()

	status := MCPStatus{Enabled: config.Enabled, Addr: config.Addr, URL: "http://" + config.Addr + "/mcp", Access: config.Access, Auth: config.Auth}
	current := s.mcp
	if !config.Enabled {
		s.mcp = nil
		s.setMCPStatus(status)
		if current != nil {
			if err := current.Close(); err != nil {
				s.logger.Warn("MCP server shutdown incomplete", "error", err)
			}
			s.logger.Info("MCP server stopped")
		}
		return nil
	}
	if current != nil && current.addr == config.Addr {
		current.update(s.server, config, s.logger)
		status.Running = true
		s.setMCPStatus(status)
		s.logger.Info("MCP server configuration updated", "address", config.Addr, "access", config.Access, "auth", config.Auth)
		return nil
	}

	next, err := startMCP(s.server, config, s.logger)
	if err != nil {
		if current == nil {
			status.Error = err.Error()
			s.setMCPStatus(status)
		}
		return err
	}
	s.mcp = next
	status.Running = true
	s.setMCPStatus(status)
	s.logger.Info("MCP server listening", "address", config.Addr, "access", config.Access, "auth", config.Auth)
	if current != nil {
		if err := current.Close(); err != nil {
			s.logger.Warn("previous MCP server shutdown incomplete", "error", err)
		}
	}
	return nil
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.requests.RLock()
	defer s.requests.RUnlock()
	if s.closing.Load() {
		writeError(w, http.StatusServiceUnavailable, "backend is stopping")
		return
	}
	s.handler.ServeHTTP(w, r)
}

func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		s.closing.Store(true)
		s.cancel()
		s.mcpMu.Lock()
		mcpServer := s.mcp
		s.mcp = nil
		s.mcpMu.Unlock()
		if mcpServer != nil {
			s.closeErr = errors.Join(s.closeErr, mcpServer.Close())
		}
		s.requests.Lock()
		defer s.requests.Unlock()
		s.server.mu.Lock()
		s.server.manager.StopAll()
		s.server.mu.Unlock()
		s.logger.Info("backend stopped")
		for n := len(s.resources) - 1; n >= 0; n-- {
			s.closeErr = errors.Join(s.closeErr, s.resources[n].Close())
		}
	})
	return s.closeErr
}

func (s *Service) ConfigDirectory() string { return filepath.Dir(s.server.configPath) }

// MCPStatus returns the non-secret listener state used by the desktop menu.
func (s *Service) MCPStatus() MCPStatus {
	s.server.mu.Lock()
	defer s.server.mu.Unlock()
	return s.server.mcpStatus
}

// SetMCPEnabled persists and hot-applies only the MCP enabled state.
func (s *Service) SetMCPEnabled(enabled bool) error {
	s.server.configMu.Lock()
	defer s.server.configMu.Unlock()
	s.server.mu.Lock()
	config := s.server.savedConfig
	s.server.mu.Unlock()
	config.MCP.Enabled = enabled
	_, err := s.server.saveFileConfigLocked(config)
	return err
}

// Logger shares the backend's configured log sinks with its desktop host.
// It must not be used after Close.
func (s *Service) Logger() *slog.Logger { return s.logger }

// TunnelViews returns the same non-secret, sorted snapshot as the HTTP API,
// including saved lines that are currently stopped.
func (s *Service) TunnelViews() []TunnelView {
	s.requests.RLock()
	defer s.requests.RUnlock()
	if s.closing.Load() {
		return []TunnelView{}
	}
	return s.server.tunnelViews()
}

// SetTunnelEnabled shares the HTTP controls and persistence without opening a port.
func (s *Service) SetTunnelEnabled(name string, enabled bool) error {
	s.requests.RLock()
	defer s.requests.RUnlock()
	if s.closing.Load() {
		return errors.New("backend is stopping")
	}
	_, _, err := s.server.setTunnelEnabled(name, enabled)
	if err != nil {
		s.logger.Warn("desktop tunnel action failed", "tunnel", name, "enabled", enabled, "error", logging.SafeError(err))
	} else {
		s.logger.Info("desktop tunnel action applied", "tunnel", name, "enabled", enabled)
	}
	return err
}

func (s *Service) Summary() (running, failed int) {
	for _, status := range s.server.manager.List() {
		if status.State == "running" {
			running++
		}
		if status.State == "error" {
			failed++
		}
	}
	return
}
