// Package store persists tunnel definitions across daemon restarts.
package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"ssh-tunnel-manager/internal/tunnel"
)

// Entry is a stored tunnel definition plus whether it should be running.
type Entry struct {
	Config  tunnel.Config `json:"config"`
	Enabled bool          `json:"enabled"`
}

// State is the full persisted set of tunnel definitions, keyed by name.
type State struct {
	Tunnels map[string]Entry `json:"tunnels"`
}

// Store reads/writes a State to a JSON file on disk.
type Store struct {
	path string
}

// New returns a Store backed by path.
func New(path string) *Store {
	return &Store{path: path}
}

// DefaultPath returns the default state file location, honoring
// XDG_CONFIG_HOME, falling back to ~/.config.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "ssh-tunnel-manager", "tunnels.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ssh-tunnel-manager", "tunnels.json"), nil
}

// Load reads the state file. A missing file is not an error; it yields an
// empty State.
func (s *Store) Load() (State, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return State{Tunnels: map[string]Entry{}}, nil
	}
	if err != nil {
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, err
	}
	if st.Tunnels == nil {
		st.Tunnels = map[string]Entry{}
	}
	return st, nil
}

// Save writes the state file atomically (write to a temp file, then rename).
func (s *Store) Save(st State) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
