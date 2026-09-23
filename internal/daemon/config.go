package daemon

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"ssh-tunnel-manager/internal/logging"
	"ssh-tunnel-manager/internal/store"
)

// FileConfig contains daemon settings only; tunnel definitions stay in tunnels.json.
type FileConfig struct {
	Addr          string `json:"addr" yaml:"addr"`
	StatePath     string `json:"state_file" yaml:"state_file"`
	LogLevel      string `json:"log_level" yaml:"log_level"`
	LogFormat     string `json:"log_format" yaml:"log_format"`
	LogFile       string `json:"log_file" yaml:"log_file"`
	ConnectionLog string `json:"connection_log" yaml:"connection_log"`
	LogMaxSizeMB  int    `json:"log_max_size_mb" yaml:"log_max_size_mb"`
	LogMaxBackups int    `json:"log_max_backups" yaml:"log_max_backups"`
}

func defaultFileConfig() (FileConfig, string, error) {
	statePath, err := store.DefaultPath()
	if err != nil {
		return FileConfig{}, "", err
	}
	dir := filepath.Dir(statePath)
	return FileConfig{
		Addr: DefaultAddr, StatePath: statePath, LogLevel: "info", LogFormat: "text",
		LogFile:       filepath.Join(dir, "logs", "daemon.log"),
		ConnectionLog: filepath.Join(dir, "logs", "connections.jsonl"),
		LogMaxSizeMB:  20, LogMaxBackups: 5,
	}, filepath.Join(dir, "config.yaml"), nil
}

func configPath(path, base string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return filepath.Abs(path)
}

func (c FileConfig) validate(path string) error {
	_, port, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return fmt.Errorf("addr must be host:port: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 0 || n > 65535 {
		return fmt.Errorf("addr port must be between 0 and 65535")
	}
	if _, err := logging.New(io.Discard, c.LogLevel, c.LogFormat); err != nil {
		return err
	}
	if c.StatePath == "" || c.ConnectionLog == "" {
		return fmt.Errorf("state_file and connection_log are required")
	}
	if c.LogMaxSizeMB < 1 || c.LogMaxSizeMB > 1024 {
		return fmt.Errorf("log_max_size_mb must be between 1 and 1024")
	}
	if c.LogMaxBackups < 1 || c.LogMaxBackups > 20 {
		return fmt.Errorf("log_max_backups must be between 1 and 20")
	}
	// Include archive names so rotation can never overwrite another managed file.
	paths := map[string]bool{}
	add := func(value string) error {
		resolved, err := configPath(value, filepath.Dir(path))
		if err != nil {
			return err
		}
		if paths[resolved] {
			return fmt.Errorf("configuration, state and log paths must not overlap: %s", value)
		}
		paths[resolved] = true
		return nil
	}
	for _, value := range []string{path, c.StatePath} {
		if err := add(value); err != nil {
			return err
		}
	}
	for _, value := range []string{c.LogFile, c.ConnectionLog} {
		if value == "" {
			continue
		}
		if err := add(value); err != nil {
			return err
		}
		for n := 1; n <= c.LogMaxBackups; n++ {
			if err := add(fmt.Sprintf("%s.%d", value, n)); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadConfig(opts Options) (saved, effective FileConfig, path string, err error) {
	saved, path, err = defaultFileConfig()
	if err != nil {
		return
	}
	if opts.ConfigPath != "" {
		path = opts.ConfigPath
	}
	path, err = configPath(path, ".")
	if err != nil {
		return
	}
	var data []byte
	data, err = os.ReadFile(path)
	if os.IsNotExist(err) && opts.ConfigPath == "" {
		err = nil
	} else if err != nil {
		return
	}
	if len(data) > 0 {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err = decoder.Decode(&saved); err != nil && err != io.EOF {
			return
		}
		err = nil
		var extra any
		if decodeErr := decoder.Decode(&extra); decodeErr != io.EOF {
			err = fmt.Errorf("configuration must contain exactly one YAML document")
			return
		}
	}
	if err = saved.validate(path); err != nil {
		return
	}
	effective = saved
	if opts.Addr != "" {
		effective.Addr = opts.Addr
	}
	if opts.StatePath != "" {
		// Preserve the CLI's existing working-directory-relative path semantics.
		effective.StatePath, err = configPath(opts.StatePath, ".")
		if err != nil {
			return
		}
	}
	if opts.LogLevel != "" {
		effective.LogLevel = opts.LogLevel
	}
	if opts.LogFormat != "" {
		effective.LogFormat = opts.LogFormat
	}
	err = effective.validate(path)
	return
}

// Write a complete replacement with private permissions; a failed write leaves the old file intact.
func saveConfig(path string, config FileConfig) error {
	if err := config.validate(path); err != nil {
		return err
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
