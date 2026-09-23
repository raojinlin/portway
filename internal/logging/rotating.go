package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RotatingFile appends complete writes and retains numbered archives (.1 is newest).
type RotatingFile struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	backups  int
	file     *os.File
	size     int64
	closed   bool
}

func OpenRotating(path string, maxBytes int64, backups int) (*RotatingFile, error) {
	if maxBytes <= 0 || backups < 1 {
		return nil, fmt.Errorf("invalid log rotation limits")
	}
	r := &RotatingFile{path: path, maxBytes: maxBytes, backups: backups}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *RotatingFile) open() error {
	if info, err := os.Lstat(r.path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("log path is not a regular file: %s", r.path)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	r.file, r.size = f, info.Size()
	return nil
}

func (r *RotatingFile) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, os.ErrClosed
	}
	if r.file == nil {
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	if r.size > 0 && r.size+int64(len(data)) > r.maxBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.file.Write(data)
	r.size += int64(n)
	return n, err
}

func (r *RotatingFile) rotate() error {
	if err := r.file.Close(); err != nil {
		r.file = nil
		return err
	}
	r.file = nil
	for n := r.backups; n >= 1; n-- {
		source := r.path
		if n > 1 {
			source = fmt.Sprintf("%s.%d", r.path, n-1)
		}
		if err := os.Rename(source, fmt.Sprintf("%s.%d", r.path, n)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return r.open()
}

func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}
