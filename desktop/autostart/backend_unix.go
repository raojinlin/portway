//go:build !windows

package autostart

import (
	"os"
	"path/filepath"
	"runtime"
)

func newBackend() (backend, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
		executable = resolved
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	r := fileBackend(runtime.GOOS, executable, home, config)
	return r, nil
}
