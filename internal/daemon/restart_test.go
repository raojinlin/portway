package daemon

import (
	"net/http"
	"testing"
)

func TestRestartTunnel(t *testing.T) {
	srv, ts := newTestServer(t)
	var created TunnelView
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/tunnels", TunnelRequest{
		Name: "restart-demo", LocalListen: "127.0.0.1:0", SSHAddress: "127.0.0.1:1",
		SSHUser: "testuser", ForwardAddress: "127.0.0.1:80",
	}, &created)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", resp.StatusCode)
	}

	var restarted TunnelView
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/tunnels/restart-demo/restart", nil, &restarted)
	if resp.StatusCode != http.StatusOK || !restarted.Enabled || !restarted.StartedAt.After(created.StartedAt) {
		t.Fatalf("restart did not replace the running instance: status=%d view=%+v", resp.StatusCode, restarted)
	}
	stored, err := srv.store.Load()
	if err != nil || !stored.Tunnels["restart-demo"].Enabled {
		t.Fatalf("restart changed saved enabled state: %v", err)
	}

	resp = doJSON(t, http.MethodPost, ts.URL+"/api/tunnels/restart-demo/stop", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stop status = %d", resp.StatusCode)
	}
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/tunnels/restart-demo/restart", nil, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stopped tunnel restart status = %d, want 409", resp.StatusCode)
	}
	if _, running := srv.manager.Status("restart-demo"); running {
		t.Fatal("restart started a disabled tunnel")
	}
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/tunnels/missing/restart", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing tunnel restart status = %d, want 404", resp.StatusCode)
	}
}
