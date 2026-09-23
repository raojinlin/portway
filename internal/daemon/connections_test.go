package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ssh-tunnel-manager/internal/store"
	"ssh-tunnel-manager/internal/tunnel"
)

func TestConnectionsEndpoint(t *testing.T) {
	srv := &Server{
		state:   store.State{Tunnels: map[string]store.Entry{"stopped tunnel": {}}},
		manager: tunnel.NewManager(),
	}
	for _, tc := range []struct {
		name   string
		path   string
		status int
	}{
		{"stopped", "/api/tunnels/stopped%20tunnel/connections", http.StatusOK},
		{"missing", "/api/tunnels/missing/connections", http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.status {
				t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
			}
			if tc.status == http.StatusOK {
				var rows []tunnel.Connection
				if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || rows == nil || len(rows) != 0 {
					t.Fatalf("expected empty JSON array, got %s (%v)", rec.Body.String(), err)
				}
			}
		})
	}
}

func TestConnectionHistoryEndpoint(t *testing.T) {
	srv := &Server{
		state:   store.State{Tunnels: map[string]store.Entry{"socks proxy": {Config: tunnel.Config{Direction: tunnel.DirectionDynamic}}}},
		manager: tunnel.NewManager(),
	}
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tunnels/socks%20proxy/connections/history", nil))
	var result struct {
		Connections []tunnel.ConnectionHistoryEntry `json:"connections"`
		Limit       int                             `json:"limit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || rec.Code != http.StatusOK || result.Connections == nil || len(result.Connections) != 0 || result.Limit != tunnel.ConnectionHistoryLimit {
		t.Fatalf("history response = %d %s, err = %v", rec.Code, rec.Body.String(), err)
	}
	rec = httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/tunnels/missing/connections/history", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing history status = %d", rec.Code)
	}
}
