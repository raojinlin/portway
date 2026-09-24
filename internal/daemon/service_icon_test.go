package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestServiceIconPersistenceAndMetadataEdit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, err := Open(ctx, Options{Desktop: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	req := TunnelRequest{Name: "database", SSHAddress: "invalid:", SSHConfigPath: filepath.Join(t.TempDir(), "no-config"), LocalListen: "127.0.0.1:15432", ForwardAddress: "localhost:5432"}
	call := func(method, path string, request TunnelRequest, want int) TunnelView {
		t.Helper()
		body, _ := json.Marshal(request)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		var view TunnelView
		if want < 300 {
			if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
				t.Fatal(err)
			}
		}
		return view
	}
	first := call("POST", "/api/tunnels", req, 201)
	if first.ResolvedServiceIcon != "postgresql" {
		t.Fatal("default target detection failed")
	}
	req.ServiceIcon = "mysql"
	updated := call("PUT", "/api/tunnels/database", req, 200)
	if updated.ResolvedServiceIcon != "mysql" || updated.ServiceIcon != "mysql" || !updated.StartedAt.Equal(first.StartedAt) {
		t.Fatal("manual icon not applied, or metadata edit restarted the tunnel")
	}
	req.ServiceIcon = ""
	if call("PUT", "/api/tunnels/database", req, 200).ServiceIcon != "mysql" {
		t.Fatal("legacy client update erased icon preference")
	}
	req.ServiceIcon = "auto"
	if call("PUT", "/api/tunnels/database", req, 200).ResolvedServiceIcon != "postgresql" {
		t.Fatal("automatic mode not restored")
	}
	req.ServiceIcon = "invalid"
	call("PUT", "/api/tunnels/database", req, 400)
	req.ServiceIcon = "redis"
	call("PUT", "/api/tunnels/database", req, 200)
	if err := s.SetTunnelEnabled("database", false); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, Options{Desktop: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if view := reopened.TunnelViews()[0]; view.ServiceIcon != "redis" || view.ResolvedServiceIcon != "redis" {
		t.Fatal("icon preference did not survive restart")
	}
}
