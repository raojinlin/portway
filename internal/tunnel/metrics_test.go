package tunnel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func awaitMetrics(t *testing.T, inst *Instance, in, out int64, active int32) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		s := inst.Status()
		if s.BytesIn == in && s.BytesOut == out && s.ActiveConns == active {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("metrics = %+v; want in=%d out=%d active=%d", s, in, out, active)
		case <-tick.C:
		}
	}
}

func TestLiveMetricsAcrossConnections(t *testing.T) {
	for _, direction := range []Direction{DirectionLocal, DirectionRemote, DirectionDynamic} {
		t.Run(string(direction), func(t *testing.T) {
			inst := &Instance{cfg: Config{Direction: direction, ForwardAddress: "service.internal:8080"}}
			inst.setStatus(Status{Name: "metrics", State: "running"})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var handlers sync.WaitGroup
			var transfers sync.WaitGroup
			const connections = 3
			upload := bytes.Repeat([]byte("a"), 701)
			download := bytes.Repeat([]byte("b"), 1303)
			for n := 0; n < connections; n++ {
				client, src := net.Pipe()
				dst, target := net.Pipe()
				for _, conn := range []net.Conn{client, src, dst, target} {
					conn := conn
					t.Cleanup(func() { conn.Close() })
					_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				}
				handlers.Add(1)
				go func() {
					defer handlers.Done()
					inst.handleConn(ctx, src, func(_ net.Conn, setTarget func(string)) (net.Conn, error) {
						if direction == DirectionDynamic {
							setTarget("service.internal:8080")
						}
						return dst, nil
					})
				}()
				transfers.Add(1)
				go func() {
					defer transfers.Done()
					writeDone := make(chan error, 1)
					go func() {
						_, err := client.Write(upload)
						writeDone <- err
					}()
					got := make([]byte, len(upload))
					if _, err := io.ReadFull(target, got); err != nil || !bytes.Equal(got, upload) {
						t.Errorf("upload forwarding failed: %v", err)
					}
					if err := <-writeDone; err != nil {
						t.Errorf("upload write: %v", err)
					}
					go func() {
						_, err := target.Write(download)
						writeDone <- err
					}()
					got = make([]byte, len(download))
					if _, err := io.ReadFull(client, got); err != nil || !bytes.Equal(got, download) {
						t.Errorf("download forwarding failed: %v", err)
					}
					if err := <-writeDone; err != nil {
						t.Errorf("download write: %v", err)
					}
				}()
			}
			transfers.Wait()
			in, out := int64(connections*len(download)), int64(connections*len(upload))
			if direction == DirectionRemote {
				in, out = out, in
			}
			// Connections remain open: counters must not wait for io.Copy to end.
			awaitMetrics(t, inst, in, out, connections)
			rows := awaitConnections(t, inst, connections, in, out)
			ids := make(map[string]bool)
			for _, row := range rows {
				if row.ID == "" || ids[row.ID] || row.Source != "pipe" || row.Target != "service.internal:8080" || row.State != "connected" || row.StartedAt.IsZero() || row.AgeSeconds < 0 {
					t.Fatalf("invalid connection snapshot: %+v", row)
				}
				ids[row.ID] = true
				if row.BytesIn != in/connections || row.BytesOut != out/connections {
					t.Fatalf("per-connection bytes = %d/%d, want %d/%d", row.BytesIn, row.BytesOut, in/connections, out/connections)
				}
			}
			rows[0].Target = "mutated snapshot"
			if inst.Connections()[0].Target != "service.internal:8080" {
				t.Fatal("snapshot mutated the live record")
			}
			// Reconnection/state updates must not reset or overwrite accumulated traffic.
			inst.setStatus(Status{Name: "metrics", State: "starting"})
			awaitMetrics(t, inst, in, out, connections)
			cancel()
			handlers.Wait()
			awaitMetrics(t, inst, in, out, 0)
			awaitConnections(t, inst, 0, 0, 0)
			if direction == DirectionDynamic {
				rows := inst.ConnectionHistory()
				if len(rows) != connections {
					t.Fatalf("completed history = %+v", rows)
				}
				for _, row := range rows {
					if row.BytesIn != in/connections || row.BytesOut != out/connections || row.State != "closed" || row.Error != "" {
						t.Fatalf("completed traffic = %+v", row)
					}
				}
			}
		})
	}
}

type partialWriter struct{}

func (partialWriter) Write([]byte) (int, error) { return 3, io.ErrClosedPipe }

func TestMetricsCountOnlyWrittenBytes(t *testing.T) {
	var count, connectionCount atomic.Int64
	n, err := (countingWriter{partialWriter{}, &count, &connectionCount}).Write([]byte("abcdef"))
	if n != 3 || err != io.ErrClosedPipe || count.Load() != 3 || connectionCount.Load() != 3 {
		t.Fatalf("write = %d, %v; count = %d", n, err, count.Load())
	}
}

func awaitConnections(t *testing.T, inst *Instance, count int, in, out int64) []Connection {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		rows := inst.Connections()
		var gotIn, gotOut int64
		for _, row := range rows {
			gotIn += row.BytesIn
			gotOut += row.BytesOut
		}
		if len(rows) == count && gotIn == in && gotOut == out {
			return rows
		}
		if time.Now().After(deadline) {
			t.Fatalf("connections = %+v; want count=%d in=%d out=%d", rows, count, in, out)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestConnectionDialFailureCleanup(t *testing.T) {
	inst := &Instance{cfg: Config{Direction: DirectionDynamic}}
	inst.setStatus(Status{Name: "dynamic", State: "running"})
	_, src := pipeConn(t)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer close(done)
		inst.handleConn(ctx, src, func(_ net.Conn, setTarget func(string)) (net.Conn, error) {
			setTarget("example.com:443")
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil, errors.New("test dial failure")
		})
	}()
	<-entered
	rows := awaitConnections(t, inst, 1, 0, 0)
	if rows[0].Target != "example.com:443" || rows[0].State != "connecting" || inst.Status().ActiveConns != 1 {
		t.Fatalf("dialing connection = %+v", rows)
	}
	cancel()
	<-done
	awaitConnections(t, inst, 0, 0, 0)
	awaitMetrics(t, inst, 0, 0, 0)
	if status := inst.Status(); status.State != "running" || status.LastError != "" {
		t.Fatalf("target failure changed tunnel health: %+v", status)
	}
}

func TestManagerConnections(t *testing.T) {
	m := NewManager()
	if rows := m.Connections("missing"); rows == nil || len(rows) != 0 {
		t.Fatalf("missing tunnel connections = %+v", rows)
	}
	inst := &Instance{cfg: Config{ForwardAddress: "localhost:80"}}
	m.tunnels["test"] = inst
	_, src := pipeConn(t)
	_, untrack := inst.trackConnection(src)
	defer untrack()
	if rows := m.Connections("test"); len(rows) != 1 || rows[0].Target != "localhost:80" {
		t.Fatalf("manager connections = %+v", rows)
	}
}

func TestDynamicConnectionRecordsRequestedTarget(t *testing.T) {
	inst := &Instance{cfg: Config{Direction: DirectionDynamic}}
	inst.setStatus(Status{Name: "socks", State: "running"})
	client, src := pipeConn(t)
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	_ = src.SetDeadline(time.Now().Add(3 * time.Second))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		inst.handleConn(ctx, src, func(src net.Conn, setTarget func(string)) (net.Conn, error) {
			return inst.dialSocks5Target(func(network, address string) (net.Conn, error) {
				rows := inst.Connections()
				if network != "tcp" || address != "example.com:443" || len(rows) != 1 || rows[0].Target != address || rows[0].State != "connecting" {
					t.Errorf("SOCKS5 target not retained before dialing: %s %s %+v", network, address, rows)
				}
				return nil, errors.New("test target unavailable")
			}, src, setTarget)
		})
	}()
	rows := awaitConnections(t, inst, 1, 0, 0)
	if rows[0].Target != "" || rows[0].State != "connecting" {
		t.Fatalf("before SOCKS5 negotiation: %+v", rows)
	}
	domain := "example.com"
	req := []byte{socks5Version, socks5CmdConnect, 0, socks5AtypDomain, byte(len(domain))}
	req = append(req, []byte(domain)...)
	req = append(req, 0x01, 0xbb)
	runClient(t, client, []byte{socks5AuthNone}, req)
	<-done
	awaitConnections(t, inst, 0, 0, 0)
}

func TestClosedConnectionRemoved(t *testing.T) {
	inst := &Instance{}
	inst.setStatus(Status{Name: "closed", State: "running"})
	client, src := pipeConn(t)
	target, dst := pipeConn(t)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		defer close(done)
		inst.handleConn(ctx, src, func(net.Conn, func(string)) (net.Conn, error) { return dst, nil })
	}()
	awaitConnections(t, inst, 1, 0, 0)
	client.Close()
	target.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not exit after connection closed")
	}
	awaitConnections(t, inst, 0, 0, 0)
	awaitMetrics(t, inst, 0, 0, 0)
}

func TestTargetFailureDoesNotChangeTunnelHealth(t *testing.T) {
	for _, direction := range []Direction{DirectionLocal, DirectionRemote, DirectionDynamic} {
		for _, state := range []string{"running", "error"} {
			t.Run(string(direction)+"/"+state, func(t *testing.T) {
				inst := &Instance{cfg: Config{Direction: direction}}
				initial := Status{Name: "health", State: state, StartedAt: time.Now()}
				if state == "error" {
					initial.LastError = "ssh transport failure"
				}
				inst.setStatus(initial)
				_, src := pipeConn(t)
				inst.handleConn(context.Background(), src, func(net.Conn, func(string)) (net.Conn, error) {
					return nil, errors.New("socks5 dial 157.240.7.20:443: ssh: rejected: connect failed (Connection timed out)")
				})
				if got := inst.Status(); got != initial {
					t.Fatalf("request failure changed tunnel status: got %+v; want %+v", got, initial)
				}
				if rows := inst.Connections(); len(rows) != 0 {
					t.Fatalf("failed request not removed: %+v", rows)
				}
			})
		}
	}
}
