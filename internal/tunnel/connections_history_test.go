package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHistoryRecorderOnceAndRedaction(t *testing.T) {
	inst := &Instance{cfg: Config{Name: "socks", Direction: DirectionDynamic, SSHPassword: "private-password"}}
	var recorded []ConnectionHistoryEntry
	inst.recordHistory = func(name string, row ConnectionHistoryEntry) {
		if name != "socks" {
			t.Errorf("name = %s", name)
		}
		inst.Connections() // The sink must run outside the instance lock.
		recorded = append(recorded, row)
	}
	_, src := pipeConn(t)
	c, finish := inst.trackConnection(src)
	c.fail(errors.New("failure private-password"))
	finish()
	finish()
	if len(recorded) != 1 || strings.Contains(recorded[0].Error, "private-password") || !strings.Contains(recorded[0].Error, "[REDACTED]") {
		t.Fatalf("recorded = %+v", recorded)
	}
}

type historyTestListener struct {
	conn   net.Conn
	closed chan struct{}
	once   sync.Once
}

func (l *historyTestListener) Accept() (net.Conn, error) {
	if l.conn != nil {
		c := l.conn
		l.conn = nil
		return c, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}
func (l *historyTestListener) Close() error   { l.once.Do(func() { close(l.closed) }); return nil }
func (l *historyTestListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }

func TestServeConnectionsDrainsHistoryOnStop(t *testing.T) {
	_, src := pipeConn(t)
	listener := &historyTestListener{conn: src, closed: make(chan struct{})}
	inst := &Instance{cfg: Config{Name: "socks", Direction: DirectionDynamic}}
	dialing, recording, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	inst.recordHistory = func(_ string, row ConnectionHistoryEntry) {
		if row.EndedAt.IsZero() {
			t.Error("record not finalized")
		}
		close(recording)
		<-release
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- inst.serveConnections(ctx, listener, func(conn net.Conn, _ func(string)) (net.Conn, error) {
			close(dialing)
			var data [1]byte
			_, err := conn.Read(data[:])
			return nil, err
		}, func() {})
	}()
	select {
	case <-dialing:
	case <-time.After(3 * time.Second):
		t.Fatal("did not accept")
	}
	cancel()
	select {
	case <-recording:
	case <-time.After(3 * time.Second):
		t.Fatal("did not finalize")
	}
	select {
	case <-done:
		t.Fatal("returned before history was written")
	default:
	}
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not drain")
	}
}

func TestSOCKS5FailureHistory(t *testing.T) {
	inst := &Instance{cfg: Config{Direction: DirectionDynamic}}
	initial := Status{Name: "socks", State: "running"}
	inst.setStatus(initial)
	_, src := pipeConn(t)
	wantErr := "socks5 dial 157.240.7.20:443: ssh: rejected: connect failed (Connection timed out)"
	inst.handleConn(context.Background(), src, func(_ net.Conn, setTarget func(string)) (net.Conn, error) {
		setTarget("157.240.7.20:443")
		if len(inst.ConnectionHistory()) != 0 {
			t.Error("active connection appeared in history")
		}
		return nil, errors.New(wantErr)
	})
	rows := inst.ConnectionHistory()
	if len(rows) != 1 {
		t.Fatalf("history = %+v", rows)
	}
	row := rows[0]
	if row.Target != "157.240.7.20:443" || row.State != "failed" || row.Error != wantErr || row.EndedAt.Before(row.StartedAt) || row.AgeSeconds < 0 {
		t.Fatalf("failed connection = %+v", row)
	}
	if got := inst.Status(); got != initial {
		t.Fatalf("request failure changed tunnel health: %+v", got)
	}
	if len(inst.Connections()) != 0 {
		t.Fatal("failed connection still active")
	}
}

func TestConnectionHistoryBoundAndOrder(t *testing.T) {
	inst := &Instance{cfg: Config{Direction: DirectionDynamic}}
	_, src := pipeConn(t)
	const total = ConnectionHistoryLimit*2 + 7
	for n := 1; n <= total; n++ {
		c, finish := inst.trackConnection(src)
		c.setTarget(fmt.Sprintf("target-%d:443", n))
		c.info.StartedAt = time.Now().Add(-2 * time.Second)
		c.bytesIn.Store(int64(n))
		c.bytesOut.Store(int64(n * 2))
		c.connected()
		finish()
		finish() // Finalization is idempotent.
	}
	rows := inst.ConnectionHistory()
	if len(rows) != ConnectionHistoryLimit {
		t.Fatalf("history length = %d", len(rows))
	}
	for n, row := range rows {
		want := total - n
		if row.ID != fmt.Sprint(want) || row.Target != fmt.Sprintf("target-%d:443", want) || row.BytesIn != int64(want) || row.BytesOut != int64(want*2) || row.State != "closed" || row.Error != "" || row.AgeSeconds < 2 {
			t.Fatalf("row %d = %+v", n, row)
		}
	}
	age := rows[0].AgeSeconds
	rows[0].Target = "modified"
	rows[0].AgeSeconds = 100000
	if got := inst.ConnectionHistory()[0]; got.Target == "modified" || got.AgeSeconds != age {
		t.Fatalf("history not a fixed detached snapshot: %+v", got)
	}
}

func TestHistoryOnlyForDynamic(t *testing.T) {
	for _, direction := range []Direction{DirectionLocal, DirectionRemote} {
		inst := &Instance{cfg: Config{Direction: direction}}
		_, src := pipeConn(t)
		_, finish := inst.trackConnection(src)
		finish()
		if rows := inst.ConnectionHistory(); rows == nil || len(rows) != 0 {
			t.Fatalf("%s history = %+v", direction, rows)
		}
	}
}

func TestConcurrentConnectionHistory(t *testing.T) {
	inst := &Instance{cfg: Config{Direction: DirectionDynamic}}
	_, src := pipeConn(t)
	var wg sync.WaitGroup
	for n := 0; n < 40; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, finish := inst.trackConnection(src)
			c.setTarget("example.com:443")
			c.fail(errors.New("target unavailable"))
			inst.Connections()
			inst.ConnectionHistory()
			finish()
		}()
	}
	wg.Wait()
	if len(inst.ConnectionHistory()) != 40 || len(inst.Connections()) != 0 {
		t.Fatal("concurrent completion lost records")
	}
	m := NewManager()
	m.tunnels["test"] = inst
	if len(m.ConnectionHistory("test")) != 40 || m.ConnectionHistory("missing") == nil {
		t.Fatal("manager history mismatch")
	}
}
