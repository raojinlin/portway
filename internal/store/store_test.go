package store

import (
	"path/filepath"
	"testing"

	"ssh-tunnel-manager/internal/tunnel"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "tunnels.json")
	s := New(path)

	st, err := s.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if len(st.Tunnels) != 0 {
		t.Fatalf("expected empty state, got %d entries", len(st.Tunnels))
	}

	st.Tunnels["demo"] = Entry{
		Config: tunnel.Config{
			Name:           "demo",
			LocalListen:    "127.0.0.1:8080",
			SSHAddress:     "host:22",
			SSHUser:        "alice",
			ForwardAddress: "10.0.0.1:80",
		},
		Enabled: true,
	}
	if err := s.Save(st); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	got, ok := reloaded.Tunnels["demo"]
	if !ok {
		t.Fatalf("expected 'demo' entry after reload")
	}
	if got.Config.SSHUser != "alice" || !got.Enabled {
		t.Fatalf("unexpected entry after reload: %+v", got)
	}
}
