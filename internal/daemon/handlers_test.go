package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ssh-tunnel-manager/internal/store"
	"ssh-tunnel-manager/internal/tunnel"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	st := store.New(filepath.Join(t.TempDir(), "state.json"))
	loaded, err := st.Load()
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	srv := &Server{store: st, state: loaded, manager: tunnel.NewManager()}
	ts := httptest.NewServer(srv.routes())
	t.Cleanup(func() {
		ts.Close()
		srv.manager.StopAll()
	})
	return srv, ts
}

func doJSON(t *testing.T, method, url string, body any, out any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	if out != nil && resp.StatusCode < 300 {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return resp
}

func TestDaemonLifecycle(t *testing.T) {
	_, ts := newTestServer(t)

	createReq := TunnelRequest{
		Name:           "demo",
		LocalListen:    "127.0.0.1:0",
		SSHAddress:     "127.0.0.1:1",
		SSHUser:        "testuser",
		ForwardAddress: "10.0.0.1:80",
	}
	var created TunnelView
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/tunnels", createReq, &created)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", resp.StatusCode)
	}
	if created.Name != "demo" || !created.Enabled {
		t.Fatalf("unexpected created view: %+v", created)
	}

	updateReq := createReq
	updateReq.ForwardAddress = "10.0.0.2:443"
	updateReq.SSHKeyPath = "/tmp/test-key"
	var updated TunnelView
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/tunnels/demo", updateReq, &updated)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d", resp.StatusCode)
	}
	if updated.ForwardAddress != "10.0.0.2:443" || updated.SSHKeyPath != "/tmp/test-key" {
		t.Fatalf("unexpected updated view: %+v", updated)
	}

	// Duplicate name should conflict.
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/tunnels", createReq, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate create status = %d, want 409", resp.StatusCode)
	}

	var list []TunnelView
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/tunnels", nil, &list)
	if resp.StatusCode != http.StatusOK || len(list) != 1 {
		t.Fatalf("list status=%d len=%d", resp.StatusCode, len(list))
	}

	var stopped TunnelView
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/tunnels/demo/stop", nil, &stopped)
	if resp.StatusCode != http.StatusOK || stopped.Enabled {
		t.Fatalf("stop status=%d enabled=%v", resp.StatusCode, stopped.Enabled)
	}

	var started TunnelView
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/tunnels/demo/start", nil, &started)
	if resp.StatusCode != http.StatusOK || !started.Enabled {
		t.Fatalf("start status=%d enabled=%v", resp.StatusCode, started.Enabled)
	}

	resp = doJSON(t, http.MethodDelete, ts.URL+"/api/tunnels/demo", nil, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", resp.StatusCode)
	}

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/tunnels/demo", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", resp.StatusCode)
	}
}

func TestDaemonCreateValidation(t *testing.T) {
	_, ts := newTestServer(t)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/tunnels", TunnelRequest{}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty create status = %d, want 400", resp.StatusCode)
	}
}

func TestDaemonCreateRemoteDirection(t *testing.T) {
	_, ts := newTestServer(t)

	// Missing remote_listen should be rejected for direction=remote.
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/tunnels", TunnelRequest{
		Name:           "remote-missing",
		Direction:      "remote",
		SSHAddress:     "127.0.0.1:1",
		SSHUser:        "testuser",
		ForwardAddress: "10.0.0.1:80",
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("remote without remote_listen status = %d, want 400", resp.StatusCode)
	}

	var created TunnelView
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/tunnels", TunnelRequest{
		Name:           "remote-ok",
		Direction:      "remote",
		SSHAddress:     "127.0.0.1:1",
		SSHUser:        "testuser",
		RemoteListen:   "0.0.0.0:9000",
		ForwardAddress: "127.0.0.1:3000",
	}, &created)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("remote create status = %d", resp.StatusCode)
	}
	if created.Direction != "remote" || created.RemoteListen != "0.0.0.0:9000" {
		t.Fatalf("unexpected view: %+v", created)
	}
}

func TestDaemonCreateDynamicDirection(t *testing.T) {
	_, ts := newTestServer(t)

	// dynamic direction needs no forward_address.
	var created TunnelView
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/tunnels", TunnelRequest{
		Name:        "socks",
		Direction:   "dynamic",
		LocalListen: "127.0.0.1:0",
		SSHAddress:  "127.0.0.1:1",
		SSHUser:     "testuser",
	}, &created)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("dynamic create status = %d", resp.StatusCode)
	}
	if created.Direction != "dynamic" {
		t.Fatalf("unexpected direction: %+v", created)
	}

	// dynamic without local_listen should be rejected.
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/tunnels", TunnelRequest{
		Name:       "socks-missing",
		Direction:  "dynamic",
		SSHAddress: "127.0.0.1:1",
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("dynamic without local_listen status = %d, want 400", resp.StatusCode)
	}
}
