package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ssh-tunnel-manager/internal/store"
	"ssh-tunnel-manager/internal/tunnel"
)

type shortHistoryWriter struct {
	bytes.Buffer
	failed bool
}

func (w *shortHistoryWriter) Close() error { return nil }
func (w *shortHistoryWriter) Write(data []byte) (int, error) {
	if !w.failed {
		w.failed = true
		w.Buffer.Write(data[:5])
		return 5, io.ErrShortWrite
	}
	return w.Buffer.Write(data)
}

func TestPersistentHistoryRecoversAfterPartialWrite(t *testing.T) {
	w := &shortHistoryWriter{}
	h := &historyLog{writer: w, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), rows: make(map[string][]tunnel.ConnectionHistoryEntry)}
	h.record("socks", testHistoryEntry(1))
	h.record("socks", testHistoryEntry(2))
	lines := bytes.Split(bytes.TrimSpace(w.Bytes()), []byte{'\n'})
	if len(lines) != 2 {
		t.Fatalf("missing record separator: %q", w.String())
	}
	var record historyRecord
	if err := json.Unmarshal(lines[1], &record); err != nil || record.ID != "2" {
		t.Fatalf("next record corrupted: %s %v", lines[1], err)
	}
}

func testHistoryEntry(n int) tunnel.ConnectionHistoryEntry {
	start := time.Unix(int64(n), 0).UTC()
	return tunnel.ConnectionHistoryEntry{Connection: tunnel.Connection{
		ID: fmt.Sprint(n), Source: "127.0.0.1:50000", Target: "example.com:443", State: "closed",
		StartedAt: start, AgeSeconds: 1, BytesIn: int64(n), BytesOut: int64(n * 2),
	}, EndedAt: start.Add(time.Second)}
}

func TestPersistentHistoryRestartAndStoppedEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.jsonl")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h, err := openHistory(path, 1024*1024, 2, logger)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 510; n++ {
		h.record("socks", testHistoryEntry(n))
	}
	failed := testHistoryEntry(1000)
	failed.State, failed.Error = "failed", "socks5 dial: timeout"
	h.record("other", failed)
	h.writer.Close()
	h, err = openHistory(path, 1024*1024, 2, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer h.writer.Close()
	rows, writeErr := h.list("socks")
	if writeErr != "" || len(rows) != 500 || rows[0].ID != "509" || rows[499].ID != "10" {
		t.Fatalf("restored rows: %d %s", len(rows), writeErr)
	}
	rows[0].Target = "changed"
	rows, _ = h.list("socks")
	if rows[0].Target != "example.com:443" {
		t.Fatal("snapshot alias")
	}
	other, _ := h.list("other")
	if len(other) != 1 || other[0] != failed {
		t.Fatalf("failed record: %+v", other)
	}
	srv := &Server{history: h, manager: tunnel.NewManager(), state: store.State{Tunnels: map[string]store.Entry{"socks": {}}}}
	w := httptest.NewRecorder()
	srv.routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/tunnels/socks/connections/history", nil))
	var result struct {
		Connections []tunnel.ConnectionHistoryEntry `json:"connections"`
		Persisted   bool                            `json:"persisted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || !result.Persisted || len(result.Connections) != 500 {
		t.Fatalf("stopped endpoint: %s %v", w.Body.String(), err)
	}
}

func TestPersistentHistoryRotationAndPartialRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.jsonl")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h, err := openHistory(path, 400, 2, logger)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 5; n++ {
		h.record("socks", testHistoryEntry(n))
	}
	h.writer.Close()
	h, err = openHistory(path, 400, 2, logger)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := h.list("socks")
	if len(rows) != 3 || rows[0].ID != "4" || rows[2].ID != "2" {
		t.Fatalf("rotated rows: %+v", rows)
	}
	h.writer.Close()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"tunnel":`)
	f.Close()
	h, err = openHistory(path, 1024*1024, 2, logger)
	if err != nil {
		t.Fatal(err)
	}
	h.record("socks", testHistoryEntry(5))
	h.writer.Close()
	h, err = openHistory(path, 1024*1024, 2, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer h.writer.Close()
	rows, _ = h.list("socks")
	if len(rows) != 4 || rows[0].ID != "5" {
		t.Fatalf("partial record lost next append: %+v", rows)
	}
}

func TestPersistentHistoryConcurrentAndWriteFailure(t *testing.T) {
	h, err := openHistory(filepath.Join(t.TempDir(), "connections.jsonl"), 1024*1024, 2, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for n := 0; n < 40; n++ {
		wg.Add(1)
		go func(n int) { defer wg.Done(); h.record("socks", testHistoryEntry(n)); h.list("socks") }(n)
	}
	wg.Wait()
	rows, writeErr := h.list("socks")
	if len(rows) != 40 || rows[0].ID != "39" || writeErr != "" {
		t.Fatalf("concurrent rows: %+v %s", rows, writeErr)
	}
	h.writer.Close()
	h.record("socks", testHistoryEntry(50))
	rows, writeErr = h.list("socks")
	if len(rows) != 41 || writeErr == "" {
		t.Fatal("write failure not surfaced or in-memory history lost")
	}
}
