package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ssh-tunnel-manager/internal/logging"
	"ssh-tunnel-manager/internal/store"
	"ssh-tunnel-manager/internal/tunnel"
)

func TestAPILogging(t *testing.T) {
	for _, level := range []string{"info", "debug"} {
		t.Run(level, func(t *testing.T) {
			var out bytes.Buffer
			logger, err := logging.New(&out, level, "json")
			if err != nil {
				t.Fatal(err)
			}
			srv := &Server{
				ctx:     logging.WithLogger(context.Background(), logger),
				store:   store.New(filepath.Join(t.TempDir(), "state.json")),
				state:   store.State{Tunnels: map[string]store.Entry{"test": {Config: tunnel.Config{Name: "test", SSHPassword: "stored-secret"}}}},
				manager: tunnel.NewManager(),
			}
			routes := srv.routes()
			for _, tc := range []struct {
				method, path, body, wantLevel string
				status                        int
			}{
				{http.MethodGet, "/api/tunnels?secret=query-secret", "", "DEBUG", 200},
				{http.MethodPost, "/api/tunnels/test/stop", `{"password":"body-secret"}`, "INFO", 200},
				{http.MethodGet, "/api/tunnels/missing", "", "WARN", 404},
				{http.MethodPost, "/api/tunnels/test/start", "", "ERROR", 500},
			} {
				out.Reset()
				req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
				req.Header.Set("Authorization", "Bearer header-secret")
				rec := httptest.NewRecorder()
				routes.ServeHTTP(rec, req)
				if rec.Code != tc.status {
					t.Fatalf("status = %d, want %d", rec.Code, tc.status)
				}
				if level == "info" && tc.wantLevel == "DEBUG" {
					if out.Len() != 0 {
						t.Fatal("normal polling logged at info")
					}
					continue
				}
				var record map[string]any
				if err := json.Unmarshal(out.Bytes(), &record); err != nil {
					t.Fatalf("invalid log: %s (%v)", out.String(), err)
				}
				if record["level"] != tc.wantLevel || record["method"] != tc.method || record["status"] != float64(tc.status) {
					t.Fatalf("record = %+v", record)
				}
				for _, secret := range []string{"query-secret", "body-secret", "header-secret", "stored-secret"} {
					if strings.Contains(out.String(), secret) {
						t.Fatalf("log leaked %s", secret)
					}
				}
			}
		})
	}
}

func TestDaemonRejectsInvalidLoggingOptions(t *testing.T) {
	if err := Run(context.Background(), Options{LogLevel: "invalid"}); err == nil {
		t.Fatal("expected validation error before startup")
	}
}
