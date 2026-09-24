package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func logTestServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	return &Server{configPath: filepath.Join(dir, "config.yaml"), effectiveConfig: FileConfig{LogFile: "daemon.log", LogMaxBackups: 2}}
}

func writeTestLog(t *testing.T, s *Server, suffix, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(filepath.Dir(s.configPath), "daemon.log"+suffix), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func getTestLogs(t *testing.T, s *Server, query string) logView {
	t.Helper()
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/logs"+query, nil))
	var result logView
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("log responses may be cached")
	}
	return result
}

func TestLogViewFormatsFiltersAndRotation(t *testing.T) {
	s := logTestServer(t)
	writeTestLog(t, s, ".1", "time=2026-09-24T01:00:00Z level=INFO msg=old tunnel=drop\n")
	writeTestLog(t, s, "", "time=2026-09-24T02:00:00Z level=WARN msg=\"target failed\" tunnel=\"socks proxy\" error=\"timeout\\nretry\"\n"+
		"{\"time\":\"2026-09-24T03:00:00Z\",\"level\":\"ERROR\",\"msg\":\"newest\",\"tunnel\":\"drop\"}\n"+
		"unfinished write")
	result := getTestLogs(t, s, "")
	if len(result.Entries) != 3 || result.Entries[0].Message != "newest" || result.Entries[2].Message != "old" {
		t.Fatal(result)
	}
	if result.Entries[0].ID == "" || result.Entries[0].ID != getTestLogs(t, s, "").Entries[0].ID {
		t.Fatal("unstable log IDs")
	}
	result = getTestLogs(t, s, "?level=warn&tunnel=socks+proxy&q=TIMEOUT")
	if len(result.Entries) != 1 || result.Entries[0].Message != "target failed" || result.Entries[0].Tunnel != "socks proxy" {
		t.Fatal(result)
	}
	result = getTestLogs(t, s, "?limit=1")
	if len(result.Entries) != 1 || !result.Truncated {
		t.Fatal("limit not applied")
	}
}

func TestLogViewUsesEffectivePathAndHandlesDisabledMissing(t *testing.T) {
	s := logTestServer(t)
	s.savedConfig.LogFile = "future.log"
	writeTestLog(t, s, "", "level=INFO msg=current\n")
	result := getTestLogs(t, s, "?path=/etc/passwd&file=../future.log")
	if len(result.Entries) != 1 || result.Entries[0].Message != "current" || filepath.Base(result.Path) != "daemon.log" {
		t.Fatal(result)
	}
	s.effectiveConfig.LogFile = ""
	result = getTestLogs(t, s, "")
	if result.Enabled || result.Entries == nil || len(result.Entries) != 0 {
		t.Fatal(result)
	}
	s.effectiveConfig.LogFile = "missing.log"
	if result = getTestLogs(t, s, ""); !result.Enabled || len(result.Entries) != 0 {
		t.Fatal(result)
	}
}

func TestLogViewRejectsInvalidQueriesAndNonFiles(t *testing.T) {
	s := logTestServer(t)
	for _, query := range []string{"?limit=-1", "?limit=1001", "?limit=bad", "?level=fatal"} {
		w := httptest.NewRecorder()
		s.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/logs"+query, nil))
		if w.Code != 400 {
			t.Fatalf("%s: %d", query, w.Code)
		}
	}
	s.effectiveConfig.LogFile = filepath.Dir(s.configPath)
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/logs", nil))
	if w.Code != 500 {
		t.Fatal("directory accepted as log file")
	}
}

func TestLogViewBoundedReadAndCancellation(t *testing.T) {
	s := logTestServer(t)
	writeTestLog(t, s, "", strings.Repeat("x", logReadBudget+100)+"\nlevel=INFO msg=tail\n")
	result := getTestLogs(t, s, "")
	if len(result.Entries) != 1 || result.Entries[0].Message != "tail" || !result.Truncated {
		t.Fatal("partial first line or read budget mishandled")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := readLogView(ctx, &result, 0, "", "", ""); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestLogViewParsingRawFallback(t *testing.T) {
	for _, value := range []string{"unstructured diagnostic", "{invalid", "msg=\"unterminated", "msg=\"bad\\"} {
		entry := parseLogEntry(value)
		if entry.Raw != value || entry.Message != value {
			t.Fatal(entry)
		}
	}
	entry := parseLogEntry(`time=t level=INFO msg="a \"quote\" and \\ path" tunnel="two words"`)
	if entry.Message != `a "quote" and \ path` || entry.Tunnel != "two words" {
		t.Fatal(fmt.Sprintf("%+v", entry))
	}
}
