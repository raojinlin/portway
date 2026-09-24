package autostart

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runValue = "Portway Desktop"

type registryRegistration struct{ command string }

func newBackend() (backend, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if strings.ContainsAny(executable, "\"\x00\r\n") {
		return nil, fmt.Errorf("invalid executable path")
	}
	return registryRegistration{command: "\"" + executable + "\" --autostart"}, nil
}

func (r registryRegistration) read() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer k.Close()
	value, _, err := k.GetStringValue(runValue)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(value, "\"") || !strings.HasSuffix(value, "\" --autostart") {
		return "", fmt.Errorf("startup entry is not managed by Portway")
	}
	return value, nil
}

func (r registryRegistration) status() (Status, error) {
	value, err := r.read()
	return Status{Supported: true, Enabled: value != "", NeedsUpdate: value != "" && value != r.command}, err
}

func (r registryRegistration) set(enabled bool) error {
	value, err := r.read()
	if err != nil {
		return err
	}
	if !enabled {
		if value == "" {
			return nil
		}
		k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer k.Close()
		err = k.DeleteValue(runValue)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(runValue, r.command)
}
