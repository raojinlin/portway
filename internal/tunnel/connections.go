package tunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"ssh-tunnel-manager/internal/logging"
)

// Connection is a snapshot of an accepted forwarding connection, not the SSH transport.
type Connection struct {
	ID         string    `json:"id"`
	Source     string    `json:"source"`
	Target     string    `json:"target"`
	State      string    `json:"state"`
	StartedAt  time.Time `json:"started_at"`
	AgeSeconds int64     `json:"age_seconds"`
	BytesIn    int64     `json:"bytes_in"`
	BytesOut   int64     `json:"bytes_out"`
}

// ConnectionHistoryLimit bounds the recent SOCKS5 history shown by the API.
const ConnectionHistoryLimit = 500

type historyRecorderKey struct{}

// WithHistoryRecorder attaches a daemon-owned sink that outlives individual tunnels.
func WithHistoryRecorder(ctx context.Context, record func(string, ConnectionHistoryEntry)) context.Context {
	return context.WithValue(ctx, historyRecorderKey{}, record)
}

type ConnectionHistoryEntry struct {
	Connection
	EndedAt time.Time `json:"ended_at"`
	Error   string    `json:"error"`
}

type liveConnection struct {
	mu       sync.Mutex
	info     Connection
	bytesIn  atomic.Int64
	bytesOut atomic.Int64
	failure  string
}

func (c *liveConnection) snapshot() (Connection, string) {
	c.mu.Lock()
	row, failure := c.info, c.failure
	c.mu.Unlock()
	row.BytesIn, row.BytesOut = c.bytesIn.Load(), c.bytesOut.Load()
	return row, failure
}

func (c *liveConnection) fail(err error) {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure == "" {
		c.failure = err.Error()
	}
}

func (c *liveConnection) setTarget(target string) {
	c.mu.Lock()
	c.info.Target = target
	c.mu.Unlock()
}

func (c *liveConnection) connected() {
	c.mu.Lock()
	c.info.State = "connected"
	c.mu.Unlock()
}

func (i *Instance) trackConnection(src net.Conn) (*liveConnection, func()) {
	i.connectionsMu.Lock()
	defer i.connectionsMu.Unlock()
	i.nextConnectionID++
	id := strconv.FormatUint(i.nextConnectionID, 10)
	target := i.cfg.ForwardAddress
	if i.cfg.direction() == DirectionDynamic {
		target = ""
	}
	c := &liveConnection{info: Connection{
		ID: id, Source: src.RemoteAddr().String(), Target: target,
		State: "connecting", StartedAt: time.Now(),
	}}
	if i.connections == nil {
		i.connections = make(map[string]*liveConnection)
	}
	i.connections[id] = c
	return c, func() {
		i.connectionsMu.Lock()
		if _, ok := i.connections[id]; !ok {
			i.connectionsMu.Unlock()
			return
		}
		delete(i.connections, id)
		if i.cfg.direction() != DirectionDynamic {
			i.connectionsMu.Unlock()
			return
		}
		c.mu.Lock()
		row := ConnectionHistoryEntry{Connection: c.info, EndedAt: time.Now(), Error: c.failure}
		c.mu.Unlock()
		row.State = "closed"
		if row.Error != "" {
			row.State = "failed"
		}
		row.AgeSeconds = int64(row.EndedAt.Sub(row.StartedAt) / time.Second)
		row.BytesIn, row.BytesOut = c.bytesIn.Load(), c.bytesOut.Load()
		if row.Error != "" {
			row.Error = logging.SafeError(errors.New(row.Error), i.cfg.SSHPassword)
		}
		if len(i.connectionHistory) < ConnectionHistoryLimit {
			i.connectionHistory = append(i.connectionHistory, row)
		} else {
			i.connectionHistory[i.historyNext] = row
		}
		i.historyNext = (i.historyNext + 1) % ConnectionHistoryLimit
		i.connectionsMu.Unlock()
		if i.recordHistory != nil {
			i.recordHistory(i.cfg.Name, row)
		}
	}
}

// ConnectionHistory returns newest-first, detached snapshots of completed SOCKS5 requests.
func (i *Instance) ConnectionHistory() []ConnectionHistoryEntry {
	i.connectionsMu.RLock()
	defer i.connectionsMu.RUnlock()
	rows := make([]ConnectionHistoryEntry, 0, len(i.connectionHistory))
	for n := len(i.connectionHistory) - 1; n >= 0; n-- {
		index := n
		if len(i.connectionHistory) == ConnectionHistoryLimit {
			index = (i.historyNext + n) % ConnectionHistoryLimit
		}
		rows = append(rows, i.connectionHistory[index])
	}
	return rows
}

// Connections returns detached snapshots of active connections in acceptance order.
func (i *Instance) Connections() []Connection {
	i.connectionsMu.RLock()
	defer i.connectionsMu.RUnlock()
	rows := make([]Connection, 0, len(i.connections))
	for _, c := range i.connections {
		c.mu.Lock()
		row := c.info
		c.mu.Unlock()
		row.AgeSeconds = int64(time.Since(row.StartedAt) / time.Second)
		row.BytesIn, row.BytesOut = c.bytesIn.Load(), c.bytesOut.Load()
		rows = append(rows, row)
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].StartedAt.Before(rows[b].StartedAt) })
	return rows
}
