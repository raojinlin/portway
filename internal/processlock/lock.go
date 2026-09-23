// Package processlock prevents simultaneous owners of mutable daemon files.
package processlock

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Lock struct {
	file *os.File
	once sync.Once
	err  error
}

func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	// Resolve directory aliases; the sidecar remains stable across atomic file replacements.
	dir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, filepath.Base(path))+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("cannot acquire lock for %s: another desktop app or daemon may be using these files; stop it first: %w", path, err)
	}
	return &Lock{file: f}, nil
}

func (l *Lock) Close() error {
	l.once.Do(func() { l.err = l.file.Close() })
	return l.err
}
