package daemon

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"ssh-tunnel-manager/internal/store"
	"ssh-tunnel-manager/internal/tunnel"
)

func TestServiceTunnelViewsIncludeStoppedAndMatchAPI(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	service, err := Open(context.Background(), Options{Desktop: true})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.server.mu.Lock()
	for _, name := range []string{"z-last", "a-first"} {
		service.server.state.Tunnels[name] = store.Entry{Config: tunnel.Config{Name: name, SSHAddress: "jump", SSHPassword: "not-for-menu"}}
	}
	service.server.mu.Unlock()
	views := service.TunnelViews()
	if len(views) != 2 || views[0].Name != "a-first" || views[1].Name != "z-last" || views[0].State != "stopped" {
		t.Fatalf("wrong stopped snapshot: %+v", views)
	}
	w := httptest.NewRecorder()
	service.ServeHTTP(w, httptest.NewRequest("GET", "/api/tunnels", nil))
	var wire []TunnelView
	if err := json.Unmarshal(w.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(views, wire) {
		t.Fatal("tray and HTTP API disagree")
	}
	views[0].Name = "mutated"
	if service.TunnelViews()[0].Name != "a-first" {
		t.Fatal("snapshot aliases server state")
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	if len(service.TunnelViews()) != 0 {
		t.Fatal("closed service returned stale lines")
	}
}
