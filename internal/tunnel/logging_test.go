package tunnel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ssh-tunnel-manager/internal/logging"
)

type logBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	events chan string
}

func (b *logBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	n, err := b.buffer.Write(data)
	b.mu.Unlock()
	if b.events != nil {
		select {
		case b.events <- string(data):
		default:
		}
	}
	return n, err
}
func (b *logBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buffer.String() }

func TestForwardFailureLoggingDoesNotChangeHealth(t *testing.T) {
	var out logBuffer
	logger, err := logging.New(&out, "debug", "json")
	if err != nil {
		t.Fatal(err)
	}
	ctx := logging.WithLogger(context.Background(), logger)
	inst := &Instance{cfg: Config{Name: "socks-log", Direction: DirectionDynamic, SSHPassword: "secret-password"}}
	inst.setStatus(Status{Name: "socks-log", State: "running"})
	_, src := pipeConn(t)
	inst.handleConn(ctx, src, func(_ net.Conn, target func(string)) (net.Conn, error) {
		target("example.com:443")
		return nil, errors.New("target timeout secret-password")
	})
	got := out.String()
	for _, want := range []string{"forward connection accepted", "forward connection failed", `"level":"WARN"`, `"tunnel":"socks-log"`, `"connection":"1"`, `"target":"example.com:443"`, `"bytes_in":0`, "[REDACTED]"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "secret-password") {
		t.Fatal("password leaked")
	}
	if inst.Status().State != "running" || inst.Status().LastError != "" {
		t.Fatal("request failure changed health")
	}
}

func TestTunnelRetryLogging(t *testing.T) {
	out := &logBuffer{events: make(chan string, 100)}
	logger, err := logging.New(out, "debug", "text")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("Host test\n User test\n IdentityFile none\n IdentityAgent none\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := logging.WithLogger(context.Background(), logger)
	inst, err := Start(ctx, Config{Name: "retry-log", Direction: DirectionDynamic, LocalListen: "127.0.0.1:0", SSHAddress: "test", SSHConfigPath: path, SSHPassword: "hidden-password", KnownHostsPath: filepath.Join(dir, "missing"), ReconnectDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer inst.Stop()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	waiting := true
	for waiting {
		select {
		case event := <-out.events:
			if strings.Contains(event, "retrying") && strings.Contains(event, "attempt=2") {
				waiting = false
			}
		case <-deadline.C:
			t.Fatal("retry logs not emitted")
		}
	}
	inst.Stop()
	got := out.String()
	for _, want := range []string{"tunnel starting", "tunnel connecting", "SSH authentication prepared", "retry_in=", "host key verification", "tunnel stopped"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "hidden-password") {
		t.Fatal("password leaked in retry logs")
	}
}

func TestProxyStderrExcludedFromLogs(t *testing.T) {
	err := fmt.Errorf("SSH host target: %w", &proxyTransportError{address: "host:22", cause: errors.New("handshake failed"), diagnostic: ": private-proxy-token"})
	if !strings.Contains(err.Error(), "private-proxy-token") {
		t.Fatal("UI diagnostic changed")
	}
	safe := logging.SafeError(err)
	if strings.Contains(safe, "private-proxy-token") || !strings.Contains(safe, "stderr omitted") {
		t.Fatalf("unsafe proxy log: %s", safe)
	}
}
