package daemon

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testFileConfig(t *testing.T) (FileConfig, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	c, path, err := defaultFileConfig()
	if err != nil {
		t.Fatal(err)
	}
	return c, path
}

func TestYAMLConfigDefaultsAndOverrides(t *testing.T) {
	want, path := testFileConfig(t)
	saved, effective, gotPath, err := loadConfig(Options{})
	if err != nil || saved != want || effective != want || gotPath != path {
		t.Fatalf("defaults: %+v %+v %s %v", saved, effective, gotPath, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("reading defaults created a file")
	}
	if _, _, _, err := loadConfig(Options{ConfigPath: path}); !os.IsNotExist(err) {
		t.Fatalf("explicit missing config: %v", err)
	}
	want.LogFormat, want.LogLevel = "json", "warn"
	want.ConnectionLog = "logs/requests.jsonl"
	if err := saveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	saved, effective, _, err = loadConfig(Options{ConfigPath: path, Addr: "127.0.0.1:8888", LogLevel: "debug"})
	if err != nil || saved != want || effective.LogFormat != "json" || effective.LogLevel != "debug" || effective.Addr != "127.0.0.1:8888" {
		t.Fatalf("overrides: %+v %+v %v", saved, effective, err)
	}
	resolved, err := configPath(saved.ConnectionLog, filepath.Dir(path))
	if err != nil || resolved != filepath.Join(filepath.Dir(path), "logs", "requests.jsonl") {
		t.Fatalf("relative path: %s %v", resolved, err)
	}
	_, cliConfig, _, err := loadConfig(Options{ConfigPath: path, StatePath: "cli-state.json"})
	wantCLIPath, _ := filepath.Abs("cli-state.json")
	if err != nil || cliConfig.StatePath != wantCLIPath {
		t.Fatalf("CLI state path: %s %v", cliConfig.StatePath, err)
	}
	info, err := os.Stat(path)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		t.Fatalf("config permissions: %v %v", info, err)
	}
}

func TestYAMLConfigRejectsInvalid(t *testing.T) {
	c, path := testFileConfig(t)
	if err := saveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		"log_level: invalid\n", "log_format: xml\n", "addr: no-port\n", "addr: localhost:65536\n",
		"connection_log: ''\n", "log_max_size_mb: 0\n", "log_max_backups: 0\n", "log_max_backups: 21\n",
		"unexpected_field: value\n", "addr: localhost:1\naddr: localhost:2\n", "{}\n---\n{}\n", "[invalid\n",
		"state_file: same\nconnection_log: same\n", "log_file: same\nconnection_log: same.1\n", "connection_log: config.yaml\n",
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := loadConfig(Options{ConfigPath: path}); err == nil {
			t.Errorf("accepted invalid config: %q", data)
		}
	}
}

func TestConfigEndpointSaveAndFailures(t *testing.T) {
	c, path := testFileConfig(t)
	srv := &Server{configPath: path, savedConfig: c, effectiveConfig: c}
	request := func(method, body, contentType, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/config", strings.NewReader(body))
		r.Header.Set("Content-Type", contentType)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		srv.routes().ServeHTTP(w, r)
		return w
	}
	w := request("GET", "", "", "")
	var view configView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || w.Code != 200 || view.Exists || view.Path != path {
		t.Fatalf("GET: %s %v", w.Body.String(), err)
	}
	next := c
	next.LogLevel = "debug"
	data, _ := json.Marshal(next)
	w = request("PUT", string(data), "application/json", "")
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || w.Code != 200 || !view.Exists || !view.RestartRequired || view.Config != next || view.Effective != c {
		t.Fatalf("save: %s %v", w.Body.String(), err)
	}
	loaded, effective, _, err := loadConfig(Options{ConfigPath: path})
	if err != nil || loaded != next || effective != next {
		t.Fatalf("reload: %+v %v", loaded, err)
	}
	before, _ := os.ReadFile(path)
	for _, tc := range []struct {
		body, contentType, origin string
		status                    int
	}{
		{"{}", "application/json", "", 400},
		{string(data) + "{}", "application/json", "", 400},
		{`{"unexpected":true}`, "application/json", "", 400},
		{string(data), "text/plain", "", 415},
		{string(data), "application/json", "https://evil.example", 403},
		{strings.Repeat(" ", 64*1024) + string(data), "application/json", "", 400},
	} {
		w := request("PUT", tc.body, tc.contentType, tc.origin)
		if w.Code != tc.status {
			t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body.String())
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) || srv.savedConfig != next {
			t.Fatal("invalid save changed config")
		}
	}
	// Force a rename failure without relying on permission bits (tests may run as root).
	srv.configPath = filepath.Join(t.TempDir(), "directory.yaml")
	if err := os.Mkdir(srv.configPath, 0o700); err != nil {
		t.Fatal(err)
	}
	w = request("PUT", string(data), "application/json", "")
	if w.Code != 500 || srv.savedConfig != next || srv.effectiveConfig != c {
		t.Fatalf("failed save mutated settings: %s", w.Body.String())
	}
}
