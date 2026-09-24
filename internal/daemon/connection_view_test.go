package daemon

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"ssh-tunnel-manager/internal/store"
	"ssh-tunnel-manager/internal/tunnel"
)

func TestAllConnectionsHistoryAndDeletedTunnels(t *testing.T) {
	now := time.Unix(100, 0)
	s := &Server{state: store.State{Tunnels: map[string]store.Entry{"current": {}}}, manager: tunnel.NewManager(), history: &historyLog{
		lastError: "disk full", rows: map[string][]tunnel.ConnectionHistoryEntry{
			"deleted": {{Connection: tunnel.Connection{ID: "1", State: "failed", Source: "127.0.0.1:1234", Target: "example.com:443", StartedAt: now}, EndedAt: now.Add(time.Second), Error: "timeout"}},
			"current": {{Connection: tunnel.Connection{ID: "1", State: "closed", StartedAt: now}, EndedAt: now.Add(2 * time.Second)}},
		},
	}}
	get := func(query string) connectionsView {
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/connections"+query, nil))
		var result connectionsView
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		return result
	}
	result := get("?kind=history&limit=1")
	if result.Total != 2 || len(result.Connections) != 1 || result.Connections[0].Tunnel != "current" || len(result.Tunnels) != 2 || !result.Persisted || result.PersistenceError != "disk full" {
		t.Fatal(result)
	}
	result = get("?kind=history&tunnel=deleted&state=failed&q=EXAMPLE")
	if result.Total != 1 || result.Connections[0].Error != "timeout" || result.Connections[0].EndedAt == nil {
		t.Fatal(result)
	}
	result = get("?kind=active")
	if result.Total != 0 || result.Connections == nil {
		t.Fatal("history leaked into active connections")
	}
	for _, query := range []string{"", "?kind=history&state=connected", "?kind=active&state=failed", "?kind=other", "?kind=active&limit=1001"} {
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/connections"+query, nil))
		if w.Code != 400 {
			t.Fatalf("invalid query accepted: %s", query)
		}
	}
}
