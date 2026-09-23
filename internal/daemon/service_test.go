package daemon

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceLifecycleAndExclusiveOwnership(t *testing.T) {
	_, path := testFileConfig(t)
	s, err := Open(context.Background(), Options{Desktop: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.ConfigDirectory() != filepath.Dir(path) {
		t.Fatal("wrong config directory")
	}
	if duplicate, err := Open(context.Background(), Options{}); err == nil {
		duplicate.Close()
		t.Fatal("CLI and desktop share ownership")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/config", nil))
	var view configView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || w.Code != 200 || !view.Desktop {
		t.Fatalf("desktop config: %s %v", w.Body.String(), err)
	}
	view.Config.LogLevel = "debug"
	body, _ := json.Marshal(view.Config)
	req := httptest.NewRequest("PUT", "/api/config", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("POST", "/api/tunnels/test/start", nil))
	if w.Code != 503 {
		t.Fatalf("closed backend accepted request: %d", w.Code)
	}
	reopened, err := Open(context.Background(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.server.savedConfig.LogLevel != "debug" {
		t.Fatal("desktop config not persisted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServiceStartupFailureReleasesLocks(t *testing.T) {
	c, _ := testFileConfig(t)
	if err := os.MkdirAll(filepath.Dir(c.StatePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.StatePath, []byte("invalid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(context.Background(), Options{}); err == nil {
		s.Close()
		t.Fatal("invalid state accepted")
	}
	if err := os.WriteFile(c.StatePath, []byte(`{"tunnels":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), Options{})
	if err != nil {
		t.Fatalf("startup failure leaked resources: %v", err)
	}
	defer s.Close()
}
