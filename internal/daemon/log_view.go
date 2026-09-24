package daemon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const logReadBudget = 2 * 1024 * 1024

type logEntry struct {
	ID      string `json:"id"`
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Tunnel  string `json:"tunnel"`
	Raw     string `json:"raw"`
}

type logView struct {
	Entries   []logEntry `json:"entries"`
	Enabled   bool       `json:"enabled"`
	Path      string     `json:"path"`
	Limit     int        `json:"limit"`
	Truncated bool       `json:"truncated"`
}

func viewLimit(r *http.Request) (int, error) {
	value := r.URL.Query().Get("limit")
	if value == "" {
		return 500, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 1000 {
		return 0, fmt.Errorf("limit must be between 1 and 1000")
	}
	return limit, nil
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	limit, err := viewLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	level := strings.ToUpper(r.URL.Query().Get("level"))
	if level != "" && level != "DEBUG" && level != "INFO" && level != "WARN" && level != "ERROR" {
		writeError(w, http.StatusBadRequest, "invalid log level")
		return
	}
	s.mu.Lock()
	cfg, configFile := s.effectiveConfig, s.configPath
	s.mu.Unlock()
	result := logView{Entries: []logEntry{}, Enabled: cfg.LogFile != "", Limit: limit}
	if !result.Enabled {
		writeJSON(w, http.StatusOK, result)
		return
	}
	// Never accept a file path from the request or the not-yet-applied saved config.
	result.Path, err = configPath(cfg.LogFile, filepath.Dir(configFile))
	if err == nil {
		err = readLogView(r.Context(), &result, cfg.LogMaxBackups, level, r.URL.Query().Get("tunnel"), r.URL.Query().Get("q"))
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read runtime log: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func readLogView(ctx context.Context, result *logView, backups int, level, name, query string) error {
	remaining := int64(logReadBudget)
	query = strings.ToLower(query)
	for n := 0; n <= backups; n++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := result.Path
		if n > 0 {
			path += "." + strconv.Itoa(n)
		}
		data, start, err := logTail(path, remaining)
		if os.IsNotExist(err) {
			// Rotation may briefly move a file between reads.
			continue
		}
		if err != nil {
			return err
		}
		remaining -= int64(len(data))
		if start > 0 {
			result.Truncated = true
		}
		// Drop an unfinished last write and a partial first line at the read boundary.
		end := bytes.LastIndexByte(data, '\n')
		if end >= 0 {
			data = data[:end+1]
		} else {
			data = nil
		}
		if start > 0 {
			if first := bytes.IndexByte(data, '\n'); first >= 0 {
				start += int64(first + 1)
				data = data[first+1:]
			} else {
				data = nil
			}
		}
		for len(data) > 0 {
			end := len(data) - 1
			begin := bytes.LastIndexByte(data[:end], '\n') + 1
			raw := strings.TrimSuffix(string(data[begin:end]), "\r")
			entry := parseLogEntry(raw)
			entry.ID = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%s", path, start+int64(begin), raw))))
			data = data[:begin]
			if raw == "" || (level != "" && entry.Level != level) || (name != "" && entry.Tunnel != name) || !strings.Contains(strings.ToLower(raw), query) {
				continue
			}
			if len(result.Entries) == result.Limit {
				result.Truncated = true
				return nil
			}
			result.Entries = append(result.Entries, entry)
		}
		if remaining <= 0 {
			result.Truncated = true
			break
		}
	}
	return nil
}

func logTail(path string, budget int64) ([]byte, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("log is not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("log is not a regular file")
	}
	start := max(int64(0), info.Size()-budget)
	data := make([]byte, info.Size()-start)
	n, err := f.ReadAt(data, start)
	if err != nil && err != io.EOF {
		return nil, 0, err
	}
	return data[:n], start, nil
}

func parseLogEntry(raw string) logEntry {
	entry := logEntry{Raw: raw, Message: raw}
	var object map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &object) == nil && object != nil {
		_ = json.Unmarshal(object["time"], &entry.Time)
		_ = json.Unmarshal(object["level"], &entry.Level)
		_ = json.Unmarshal(object["msg"], &entry.Message)
		_ = json.Unmarshal(object["tunnel"], &entry.Tunnel)
	} else {
		fields := logTextFields(raw)
		entry.Time, entry.Level, entry.Tunnel = fields["time"], fields["level"], fields["tunnel"]
		if msg, ok := fields["msg"]; ok {
			entry.Message = msg
		}
	}
	entry.Level = strings.ToUpper(entry.Level)
	return entry
}

// TextHandler uses Go-quoted values for whitespace/quotes. Keep parsing bounded
// to one physical line; raw output remains available for unknown formats.
func logTextFields(text string) map[string]string {
	fields := map[string]string{}
	for text != "" {
		text = strings.TrimLeft(text, " \t")
		equal := strings.IndexByte(text, '=')
		if equal <= 0 || strings.ContainsAny(text[:equal], " \t") {
			break
		}
		key := text[:equal]
		text = text[equal+1:]
		end := 0
		if strings.HasPrefix(text, "\"") {
			end = 1
			for end < len(text) {
				if text[end] == '\\' {
					end += 2
					continue
				}
				if text[end] == '"' {
					end++
					break
				}
				end++
			}
			if end > len(text) {
				break
			}
			value, err := strconv.Unquote(text[:end])
			if err != nil {
				break
			}
			fields[key] = value
		} else {
			end = strings.IndexAny(text, " \t")
			if end < 0 {
				end = len(text)
			}
			fields[key] = text[:end]
		}
		text = text[end:]
	}
	return fields
}
