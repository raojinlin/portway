// Package autostart manages only Portway's current-user login registration.
package autostart

import (
	"errors"
	"sync"
)

var registrationMu sync.Mutex

type Status struct {
	Supported   bool   `json:"supported"`
	Enabled     bool   `json:"enabled"`
	NeedsUpdate bool   `json:"needs_update"`
	Reason      string `json:"reason,omitempty"`
}

type backend interface {
	status() (Status, error)
	set(bool) error
}

func Get() (Status, error) {
	registrationMu.Lock()
	defer registrationMu.Unlock()
	b, err := newBackend()
	if err != nil {
		return Status{}, err
	}
	return b.status()
}

func Set(enabled bool) (Status, error) {
	registrationMu.Lock()
	defer registrationMu.Unlock()
	b, err := newBackend()
	if err != nil {
		return Status{}, err
	}
	if err = b.set(enabled); err != nil {
		return Status{}, err
	}
	return b.status()
}

var errUnsupported = errors.New("autostart is unavailable for this application location or platform")

func LaunchHidden(args []string, hasTray bool) bool {
	if !hasTray {
		return false
	}
	for _, arg := range args {
		if arg == "--autostart" {
			return true
		}
	}
	return false
}
