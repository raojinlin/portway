package daemon

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"sync"

	"ssh-tunnel-manager/internal/logging"
	"ssh-tunnel-manager/internal/tunnel"
)

type historyRecord struct {
	Tunnel string `json:"tunnel"`
	tunnel.ConnectionHistoryEntry
}

// historyLog owns the persisted history across tunnel stop/start and daemon restart.
type historyLog struct {
	mu             sync.Mutex
	writer         io.WriteCloser
	logger         *slog.Logger
	rows           map[string][]tunnel.ConnectionHistoryEntry
	lastError      string
	needsSeparator bool
}

func openHistory(path string, maxBytes int64, backups int, logger *slog.Logger) (*historyLog, error) {
	h := &historyLog{logger: logger, rows: make(map[string][]tunnel.ConnectionHistoryEntry)}
	// Replay oldest to newest; only the last 500 entries per tunnel are cached.
	for n := backups; n >= 0; n-- {
		file := path
		if n > 0 {
			file = fmt.Sprintf("%s.%d", path, n)
		}
		if err := h.replay(file); err != nil {
			return nil, err
		}
	}
	writer, err := logging.OpenRotating(path, maxBytes, backups)
	if err != nil {
		return nil, err
	}
	h.writer = writer
	// A hard shutdown may leave a partial last record. Keep the next record separate.
	f, err := os.Open(path)
	if err != nil {
		writer.Close()
		return nil, err
	}
	info, err := f.Stat()
	needsSeparator := false
	if err == nil && info.Size() > 0 {
		var last [1]byte
		_, err = f.ReadAt(last[:], info.Size()-1)
		needsSeparator = err == nil && last[0] != '\n'
	}
	f.Close()
	// Close the read handle before Write can rotate this file (required on Windows).
	if err == nil && needsSeparator {
		_, err = writer.Write([]byte{'\n'})
	}
	if err != nil {
		writer.Close()
		return nil, err
	}
	return h, nil
}

func (h *historyLog) replay(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	skipped := 0
	for scanner.Scan() {
		var record historyRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil || record.Tunnel == "" || record.EndedAt.IsZero() {
			skipped++
			continue
		}
		h.append(record.Tunnel, record.ConnectionHistoryEntry)
	}
	if skipped > 0 {
		h.logger.Warn("invalid connection history records skipped", "file", path, "records", skipped)
	}
	return scanner.Err()
}

func (h *historyLog) append(name string, row tunnel.ConnectionHistoryEntry) {
	rows := h.rows[name]
	// Concurrent finalizers may acquire the log lock out of end-time order.
	index := sort.Search(len(rows), func(n int) bool { return rows[n].EndedAt.After(row.EndedAt) })
	rows = append(rows, tunnel.ConnectionHistoryEntry{})
	copy(rows[index+1:], rows[index:])
	rows[index] = row
	if len(rows) > tunnel.ConnectionHistoryLimit {
		copy(rows, rows[len(rows)-tunnel.ConnectionHistoryLimit:])
		rows = rows[:tunnel.ConnectionHistoryLimit]
	}
	h.rows[name] = rows
}

func (h *historyLog) record(name string, row tunnel.ConnectionHistoryEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()
	data, err := json.Marshal(historyRecord{Tunnel: name, ConnectionHistoryEntry: row})
	if err == nil && h.needsSeparator {
		_, err = h.writer.Write([]byte{'\n'})
		if err == nil {
			h.needsSeparator = false
		}
	}
	if err == nil {
		data = append(data, '\n')
		var written int
		written, err = h.writer.Write(data)
		if err == nil && written != len(data) {
			err = io.ErrShortWrite
		}
		if err != nil {
			h.needsSeparator = true
		}
	}
	if err != nil {
		h.lastError = logging.SafeError(err)
		h.logger.Error("connection history write failed", "tunnel", name, "error", h.lastError)
	}
	// Keep the UI usable on disk errors, but surface that persistence has failed.
	h.append(name, row)
}

func (h *historyLog) list(name string) ([]tunnel.ConnectionHistoryEntry, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rows := append([]tunnel.ConnectionHistoryEntry{}, h.rows[name]...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].EndedAt.After(rows[j].EndedAt) })
	return rows, h.lastError
}
