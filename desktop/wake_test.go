package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ssh-tunnel-manager/internal/daemon"
)

func TestWakeRestartsOnlyEnabledTunnels(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a := &application{ready: make(chan struct{})}
	a.startup(t.Context())
	defer closeTestBackend(a)
	if a.problem != nil {
		t.Fatal(a.problem)
	}
	body, err := json.Marshal(daemon.TunnelRequest{
		Name: "wake-demo", LocalListen: "127.0.0.1:0", SSHAddress: "127.0.0.1:1",
		SSHUser: "testuser", ForwardAddress: "127.0.0.1:80",
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	a.service.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/tunnels", bytes.NewReader(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	before := a.service.TunnelViews()[0].StartedAt
	a.restartAfterWake()
	after := a.service.TunnelViews()[0]
	if !after.Enabled || !after.StartedAt.After(before) {
		t.Fatalf("wake did not restart enabled tunnel: %+v", after)
	}
	if err := a.changeTunnel("wake-demo", false); err != nil {
		t.Fatal(err)
	}
	a.restartAfterWake()
	if a.service.TunnelViews()[0].Enabled {
		t.Fatal("wake restarted a stopped tunnel")
	}
}
