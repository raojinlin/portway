// Package daemon runs the long-lived process that owns all tunnels: it
// loads/persists tunnel definitions via internal/store, keeps them running
// via internal/tunnel.Manager, and exposes both a JSON API and the embedded
// web UI over HTTP so the CLI and browser share one source of truth.
package daemon

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"ssh-tunnel-manager/internal/logging"
	"ssh-tunnel-manager/internal/store"
	"ssh-tunnel-manager/internal/tunnel"
)

// DefaultAddr is the default bind address for the daemon HTTP server.
const DefaultAddr = "127.0.0.1:7777"

// Options configures a daemon run.
type Options struct {
	Desktop    bool   // in-process desktop backend; HTTP listen settings are not used
	ConfigPath string // optional YAML file; defaults to ~/.config/ssh-tunnel-manager/config.yaml
	Addr       string // defaults to DefaultAddr
	StatePath  string // defaults to store.DefaultPath()
	LogLevel   string // debug, info (default), warn, error
	LogFormat  string // text (default), json
}

// Server ties together the persisted tunnel definitions and the running
// Manager. All mutating handlers hold mu while touching state, keeping the
// in-memory state and the on-disk file consistent.
type Server struct {
	ctx             context.Context
	mu              sync.Mutex
	store           *store.Store
	state           store.State
	manager         *tunnel.Manager
	configPath      string
	savedConfig     FileConfig
	effectiveConfig FileConfig
	history         *historyLog
	desktop         bool
}

// Run starts the daemon and blocks until ctx is cancelled, then shuts down
// gracefully (stops all tunnels, closes the HTTP server).
func Run(ctx context.Context, opts Options) error {
	saved, configFile, path, err := loadConfig(opts)
	if err != nil {
		return err
	}
	addr := configFile.Addr
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	service, err := openWithConfig(ctx, opts, saved, configFile, path)
	if err != nil {
		return err
	}
	defer service.Close()
	logger := service.logger

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           service,
		ReadHeaderTimeout: 5 * time.Second,
	}
	defer httpServer.Close()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("daemon listening", "address", listener.Addr().String())
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		logger.Error("daemon HTTP server failed", "error", logging.SafeError(err))
		return err
	}

	logger.Info("daemon stopping", "reason", ctx.Err())

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = httpServer.Shutdown(shutdownCtx)
	if err != nil {
		logger.Error("daemon shutdown failed", "error", logging.SafeError(err))
		return err
	}
	logger.Info("daemon stopped")
	return nil
}

func (s *Server) tunnelContext() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}
