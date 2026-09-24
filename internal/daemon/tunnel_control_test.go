package daemon

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ssh-tunnel-manager/internal/store"
	"ssh-tunnel-manager/internal/tunnel"
)

func TestServiceTunnelControls(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Exercise lifecycle and persistence without making SSH connections.
	s, err := Open(ctx, Options{Desktop: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	name := "menu-line"
	s.server.state.Tunnels[name] = store.Entry{Config: tunnel.Config{
		Name: name, Direction: tunnel.DirectionDynamic, LocalListen: "127.0.0.1:1080", SSHAddress: "invalid:",
		SSHConfigPath: filepath.Join(t.TempDir(), "no-ssh-config"),
	}}
	for _, enabled := range []bool{true, true, false, false, true, false} {
		before, exists := s.server.manager.Status(name)
		if err := s.SetTunnelEnabled(name, enabled); err != nil {
			t.Fatal(err)
		}
		view := s.TunnelViews()[0]
		if view.Enabled != enabled {
			t.Fatalf("enabled=%v, want %v", view.Enabled, enabled)
		}
		after, active := s.server.manager.Status(name)
		if active != enabled || (enabled && exists && before.StartedAt != after.StartedAt) {
			t.Fatal("start/stop is not idempotent")
		}
		saved, err := s.server.store.Load()
		if err != nil || saved.Tunnels[name].Enabled != enabled {
			t.Fatalf("enabled state not persisted: %v", err)
		}
	}
	for _, action := range []string{"start", "start", "stop", "stop"} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("POST", "/api/tunnels/"+name+"/"+action, nil))
		if w.Code != 200 {
			t.Fatalf("HTTP %s: %d %s", action, w.Code, w.Body.String())
		}
	}
	if err := s.SetTunnelEnabled("missing", true); err == nil {
		t.Fatal("missing tunnel accepted start")
	}
	s.server.state.Tunnels["invalid"] = store.Entry{Config: tunnel.Config{Name: "invalid"}}
	if err := s.SetTunnelEnabled("invalid", true); err == nil || !strings.Contains(err.Error(), "start tunnel") {
		t.Fatalf("missing validation error: %v", err)
	}
	s.server.store = store.New(t.TempDir()) // Cannot replace a directory with the state file.
	if err := s.SetTunnelEnabled(name, false); err == nil || !strings.Contains(err.Error(), "persist state") {
		t.Fatalf("missing persistence error: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTunnelEnabled(name, true); err == nil {
		t.Fatal("closed service accepted start")
	}
}
