package tunnel

import (
	"context"
	"errors"
	"sync"
)

// Manager tracks multiple tunnels by name.
type Manager struct {
	mu      sync.RWMutex
	tunnels map[string]*Instance
}

func NewManager() *Manager {
	return &Manager{tunnels: make(map[string]*Instance)}
}

// Add starts a new tunnel with the given config.
func (m *Manager) Add(ctx context.Context, cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.tunnels[cfg.Name]; exists {
		return errors.New("tunnel already exists")
	}
	inst, err := Start(ctx, cfg)
	if err != nil {
		return err
	}
	m.tunnels[cfg.Name] = inst
	return nil
}

// Remove stops and deletes a tunnel.
func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	inst, ok := m.tunnels[name]
	if !ok {
		m.mu.Unlock()
		return errors.New("not found")
	}
	delete(m.tunnels, name)
	m.mu.Unlock()
	inst.Stop()
	return nil
}

// List returns current statuses.
func (m *Manager) List() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Status, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		out = append(out, t.Status())
	}
	return out
}

// Status returns status for one tunnel.
func (m *Manager) Status(name string) (Status, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.tunnels[name]
	if !ok {
		return Status{}, false
	}
	return inst.Status(), true
}

// Connections returns active forwarding connections, or an empty list when stopped.
func (m *Manager) Connections(name string) []Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if inst, ok := m.tunnels[name]; ok {
		return inst.Connections()
	}
	return []Connection{}
}

// ConnectionHistory returns this instance's completed SOCKS5 requests.
func (m *Manager) ConnectionHistory(name string) []ConnectionHistoryEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if inst, ok := m.tunnels[name]; ok {
		return inst.ConnectionHistory()
	}
	return []ConnectionHistoryEntry{}
}

// StopAll stops everything.
func (m *Manager) StopAll() {
	m.mu.Lock()
	insts := make([]*Instance, 0, len(m.tunnels))
	for _, t := range m.tunnels {
		insts = append(insts, t)
	}
	m.tunnels = make(map[string]*Instance)
	m.mu.Unlock()

	for _, t := range insts {
		t.Stop()
	}
}
