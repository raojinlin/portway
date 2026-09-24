package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/user"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"ssh-tunnel-manager/internal/logging"

	"golang.org/x/crypto/ssh"
)

// Direction selects which of the three SSH port-forwarding modes a tunnel
// implements.
type Direction string

const (
	// DirectionLocal forwards a local listen port through the SSH server to
	// a fixed remote target (ssh -L). This is the zero value, so tunnels
	// persisted before Direction existed keep working unchanged.
	DirectionLocal Direction = "local"
	// DirectionRemote asks the SSH server to listen on RemoteListen and
	// forwards each incoming connection to a fixed target reachable from
	// this host (ssh -R).
	DirectionRemote Direction = "remote"
	// DirectionDynamic runs a local SOCKS5 proxy on LocalListen; the target
	// for each connection is supplied by the SOCKS5 client (ssh -D).
	DirectionDynamic Direction = "dynamic"
)

// Config describes a single tunnel.
type Config struct {
	ServiceIcon    string // Empty/auto detects the forwarded service; otherwise a built-in icon ID.
	Name           string
	Direction      Direction // local (default) | remote | dynamic
	LocalListen    string    // local bind address; used by local and dynamic
	SSHAddress     string    // e.g. host:22 (may be logical host from ssh config)
	SSHUser        string
	SSHKeyPath     string // path to private key (PEM)
	SSHPassword    string // optional password; prefer key
	SSHConfigPath  string // optional path to ssh config (default ~/.ssh/config)
	RemoteListen   string // address the SSH server should bind; remote only
	ForwardAddress string // fixed target host:port; used by local and remote
	KeepAlive      time.Duration
	ReconnectDelay time.Duration

	KnownHostsPath           string // optional override; defaults to ssh config's UserKnownHostsFile or ~/.ssh/known_hosts
	TrustNewHostKey          bool   // trust-on-first-use for hosts not yet in known_hosts; mismatches are still rejected
	InsecureSkipHostKeyCheck bool   // disables host identity verification; unsafe, for controlled environments only
}

// direction returns cfg.Direction, defaulting empty (zero value / configs
// persisted before this field existed) to DirectionLocal.
func (c Config) direction() Direction {
	if c.Direction == "" {
		return DirectionLocal
	}
	return c.Direction
}

// ValidateConfig checks that cfg has the fields required for its Direction.
// It's used both by Start (in-process) and by the daemon's HTTP API, so the
// required-field rules for each direction live in exactly one place.
func ValidateConfig(cfg Config) error {
	if !ValidServiceIcon(cfg.ServiceIcon) {
		return fmt.Errorf("invalid service_icon %q", cfg.ServiceIcon)
	}
	if cfg.Name == "" {
		return errors.New("name is required")
	}
	switch cfg.direction() {
	case DirectionRemote:
		if cfg.RemoteListen == "" || cfg.SSHAddress == "" || cfg.ForwardAddress == "" {
			return errors.New("remote listen, ssh address, and forward address are required for remote forwarding")
		}
	case DirectionDynamic:
		if cfg.LocalListen == "" || cfg.SSHAddress == "" {
			return errors.New("local listen and ssh address are required for dynamic forwarding")
		}
	case DirectionLocal:
		if cfg.LocalListen == "" || cfg.SSHAddress == "" || cfg.ForwardAddress == "" {
			return errors.New("local listen, ssh address, and forward address are required")
		}
	default:
		return fmt.Errorf("unknown direction %q", cfg.Direction)
	}
	return nil
}

// Status captures runtime stats for a tunnel.
type Status struct {
	Name        string
	State       string // starting, running, stopping, stopped, error
	LastError   string
	BytesIn     int64 // payload received by this host during this instance's lifetime
	BytesOut    int64 // payload sent by this host during this instance's lifetime
	ActiveConns int32 // accepted forwarding connections, including target dialing
	StartedAt   time.Time
}

// Instance represents a running tunnel.
type Instance struct {
	cfg    Config
	status atomic.Value // holds Status

	cancel context.CancelFunc
	done   chan struct{}

	connectionsMu     sync.RWMutex
	connections       map[string]*liveConnection
	connectionHistory []ConnectionHistoryEntry
	historyNext       int
	nextConnectionID  uint64
	recordHistory     func(string, ConnectionHistoryEntry)
	bytesIn           atomic.Int64
	bytesOut          atomic.Int64

	mu       sync.Mutex
	listener net.Listener
	client   *ssh.Client
}

// Start launches a tunnel and returns the running instance.
func Start(ctx context.Context, cfg Config) (*Instance, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	// SSHUser falls back to ssh config's User, then the daemon's local user.

	cctx, cancel := context.WithCancel(ctx)
	inst := &Instance{
		cfg:    cfg,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	inst.recordHistory, _ = ctx.Value(historyRecorderKey{}).(func(string, ConnectionHistoryEntry))
	inst.setStatus(Status{ // initial
		Name:      cfg.Name,
		State:     "starting",
		StartedAt: time.Now(),
	})

	go inst.run(cctx)
	return inst, nil
}

// Stop requests shutdown and waits for completion.
func (i *Instance) Stop() {
	i.cancel()
	<-i.done
}

// Status returns the latest status snapshot.
func (i *Instance) Status() Status {
	v := i.status.Load()
	if v == nil {
		return Status{}
	}
	s := v.(Status)
	s.BytesIn = i.bytesIn.Load()
	s.BytesOut = i.bytesOut.Load()
	i.connectionsMu.RLock()
	s.ActiveConns = int32(len(i.connections))
	i.connectionsMu.RUnlock()
	return s
}

func (i *Instance) setStatus(s Status) {
	i.status.Store(s)
}

func (i *Instance) run(ctx context.Context) {
	defer close(i.done)
	logger := logging.FromContext(ctx).With("tunnel", i.cfg.Name, "direction", i.cfg.direction())
	logger.Info("tunnel starting", "ssh", i.cfg.SSHAddress, "local_listen", i.cfg.LocalListen, "remote_listen", i.cfg.RemoteListen, "target", i.cfg.ForwardAddress)
	defer func() {
		status := i.Status()
		logger.Info("tunnel stopped", "bytes_in", status.BytesIn, "bytes_out", status.BytesOut, "uptime", time.Since(status.StartedAt))
	}()
	for attempt := 1; ; attempt++ {
		logger.Info("tunnel connecting", "attempt", attempt)
		if err := i.connectAndServe(ctx); err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				i.setStatus(Status{
					Name:      i.cfg.Name,
					State:     "stopped",
					LastError: "",
					StartedAt: i.Status().StartedAt,
				})
				return
			}
			// record error and retry after delay
			prev := i.Status()
			i.setStatus(Status{
				Name:      i.cfg.Name,
				State:     "error",
				LastError: err.Error(),
				StartedAt: prev.StartedAt,
			})
			delay := i.cfg.ReconnectDelay
			if delay == 0 {
				delay = 3 * time.Second
			}
			logger.Warn("tunnel connection failed; retrying", "attempt", attempt, "retry_in", delay, "error", logging.SafeError(err, i.cfg.SSHPassword))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
		} else {
			return
		}
	}
}

// dstDialer produces the destination connection for one accepted src
// connection. Local/remote forwarding ignore src and dial a fixed target;
// dynamic (SOCKS5) forwarding reads the target from src itself.
type dstDialer func(src net.Conn, setTarget func(string)) (net.Conn, error)

func (i *Instance) connectAndServe(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	client, err := i.dialSSH(ctx)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	i.mu.Lock()
	i.client = client
	i.mu.Unlock()

	ln, dial, err := i.setupDirection(ctx, client)
	if err != nil {
		client.Close()
		return err
	}
	i.mu.Lock()
	i.listener = ln
	i.mu.Unlock()

	i.setStatus(Status{
		Name:      i.cfg.Name,
		State:     "running",
		StartedAt: i.Status().StartedAt,
	})
	logging.FromContext(ctx).Info("tunnel listening", "tunnel", i.cfg.Name, "direction", i.cfg.direction(), "listen", ln.Addr().String())

	return i.serveConnections(ctx, ln, dial, func() { cancel(); client.Close() })
}

// Drain connection finalizers before Stop returns, so the daemon can safely close its history log.
func (i *Instance) serveConnections(ctx context.Context, ln net.Listener, dial dstDialer, closeTransport func()) error {
	ctx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	acceptDone := make(chan struct{})
	var connections sync.WaitGroup
	go func() {
		defer close(acceptDone)
		for {
			conn, aErr := ln.Accept()
			if aErr != nil {
				errCh <- aErr
				return
			}
			connections.Add(1)
			go func() {
				defer connections.Done()
				i.handleConn(ctx, conn, dial)
			}()
		}
	}()
	defer func() {
		cancel()
		ln.Close()
		closeTransport()
		<-acceptDone
		connections.Wait()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// setupDirection opens the listener for cfg.direction() and returns the
// matching dstDialer: local dials the fixed forward target through the SSH
// tunnel; remote asks the server to listen and dials the fixed target
// directly from this host; dynamic listens locally as a SOCKS5 proxy and
// dials whatever target each client requests, through the tunnel.
func (i *Instance) setupDirection(ctx context.Context, client *ssh.Client) (net.Listener, dstDialer, error) {
	switch i.cfg.direction() {
	case DirectionRemote:
		ln, err := client.Listen("tcp", i.cfg.RemoteListen)
		if err != nil {
			return nil, nil, fmt.Errorf("remote listen %s: %w", i.cfg.RemoteListen, err)
		}
		return ln, func(net.Conn, func(string)) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", i.cfg.ForwardAddress)
		}, nil
	case DirectionDynamic:
		ln, err := net.Listen("tcp", i.cfg.LocalListen)
		if err != nil {
			return nil, nil, fmt.Errorf("listen %s: %w", i.cfg.LocalListen, err)
		}
		return ln, func(src net.Conn, setTarget func(string)) (net.Conn, error) {
			return i.dialSocks5Target(client.Dial, src, setTarget)
		}, nil
	default: // DirectionLocal
		ln, err := net.Listen("tcp", i.cfg.LocalListen)
		if err != nil {
			return nil, nil, fmt.Errorf("listen %s: %w", i.cfg.LocalListen, err)
		}
		return ln, func(net.Conn, func(string)) (net.Conn, error) {
			return client.Dial("tcp", i.cfg.ForwardAddress)
		}, nil
	}
}

// dialSocks5Target performs the SOCKS5 handshake on src, dials the requested
// target through the SSH tunnel, and replies to the SOCKS5 client with the
// outcome.
func (i *Instance) dialSocks5Target(dial func(string, string) (net.Conn, error), src net.Conn, setTarget func(string)) (net.Conn, error) {
	target, err := socks5ReadRequest(src)
	if err != nil {
		return nil, fmt.Errorf("socks5 handshake: %w", err)
	}
	// SSH channel addresses are placeholders; retain the client's requested target.
	setTarget(target)
	dst, err := dial("tcp", target)
	if err != nil {
		_ = socks5WriteReply(src, socks5ReplyForDialError(err), "")
		return nil, fmt.Errorf("socks5 dial %s: %w", target, err)
	}
	if err := socks5WriteReply(src, socks5RepSucceeded, dst.LocalAddr().String()); err != nil {
		dst.Close()
		return nil, fmt.Errorf("socks5 reply: %w", err)
	}
	return dst, nil
}

func (i *Instance) dialSSH(ctx context.Context) (*ssh.Client, error) {
	client, err := dialNativeSSH(ctx, i.cfg, (&net.Dialer{}).DialContext)
	if err != nil {
		return nil, err
	}
	if i.cfg.KeepAlive > 0 {
		go keepAlive(ctx, client, i.cfg.KeepAlive, i.cfg.Name, i.cfg.SSHPassword)
	}
	return client, nil
}

// resolveSSHSettings applies OpenSSH config before authentication is built.
// In particular, IdentityFile is itself an authentication source, so checking
// for auth before resolving the host alias would incorrectly reject it.
func resolveSSHSettings(cfg Config) (host, user, keyPath string, err error) {
	alias, _, err := splitSSHAddress(cfg.SSHAddress)
	if err != nil {
		return "", "", "", err
	}
	settings, _ := LoadSSHConfig(alias, cfg.SSHConfigPath)
	return resolveSSHSettingsWithConfig(cfg, settings)
}

func resolveSSHSettingsWithConfig(cfg Config, sshCfg SSHConfig) (host, username, keyPath string, err error) {
	host, port, err := splitSSHAddress(cfg.SSHAddress)
	if err != nil {
		return "", "", "", err
	}
	username = cfg.SSHUser
	keyPath = cfg.SSHKeyPath

	if sshCfg.Host != "" {
		host = sshCfg.Host
	}
	if port == "" {
		port = sshCfg.Port
	}
	if username == "" {
		username = sshCfg.User
	}
	if keyPath == "" {
		keyPath = sshCfg.Identity
	}
	if username == "" {
		localUser, userErr := user.Current()
		if userErr != nil {
			return "", "", "", fmt.Errorf("resolve local SSH user: %w", userErr)
		}
		username = localUser.Username
	}
	if port == "" {
		port = "22"
	}
	portNumber, portErr := strconv.Atoi(port)
	if portErr != nil || portNumber < 1 || portNumber > 65535 {
		return "", "", "", fmt.Errorf("invalid ssh port %q: must be between 1 and 65535", port)
	}
	host = net.JoinHostPort(host, port)

	if keyPath != "" {
		keyPath, err = expandHome(keyPath)
		if err != nil {
			return "", "", "", fmt.Errorf("expand ssh key path: %w", err)
		}
	}
	return host, username, keyPath, nil
}

func (i *Instance) handleConn(ctx context.Context, src net.Conn, dial dstDialer) {
	connection, untrack := i.trackConnection(src)
	defer untrack()
	initial, _ := connection.snapshot()
	logger := logging.FromContext(ctx).With("tunnel", i.cfg.Name, "connection", initial.ID, "direction", i.cfg.direction(), "source", initial.Source)
	logger.Debug("forward connection accepted", "target", initial.Target)
	defer func() {
		row, failure := connection.snapshot()
		fields := []any{"target", row.Target, "elapsed", time.Since(row.StartedAt), "bytes_in", row.BytesIn, "bytes_out", row.BytesOut}
		if failure != "" && ctx.Err() == nil {
			logger.Warn("forward connection failed", append(fields, "error", logging.SafeError(errors.New(failure), i.cfg.SSHPassword))...)
		} else {
			logger.Debug("forward connection closed", fields...)
		}
	}()
	defer src.Close()
	stopClose := context.AfterFunc(ctx, func() { src.Close() })
	defer stopClose()

	dst, err := dial(src, connection.setTarget)
	if err != nil {
		if ctx.Err() == nil {
			connection.fail(err)
		}
		// A target failure belongs to this request, not the shared SSH tunnel.
		// SOCKS5 already reports the failure to its client in dialSocks5Target.
		// Only the transport/listener lifecycle may change tunnel health.
		return
	}
	defer dst.Close()
	connection.connected()
	connected, _ := connection.snapshot()
	logger.Debug("forward target connected", "target", connected.Target, "elapsed", time.Since(connected.StartedAt))

	// Count each successful write, so long-lived and concurrent connections
	// contribute immediately without overwriting each other's status snapshots.
	toTarget, toClient := &i.bytesOut, &i.bytesIn
	connToTarget, connToClient := &connection.bytesOut, &connection.bytesIn
	if i.cfg.direction() == DirectionRemote {
		toTarget, toClient = &i.bytesIn, &i.bytesOut
		connToTarget, connToClient = &connection.bytesIn, &connection.bytesOut
	}
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, err := io.Copy(countingWriter{dst, toTarget, connToTarget}, src)
		if ctx.Err() == nil {
			connection.fail(err)
		}
	}()
	go func() {
		defer wg.Done()
		_, err := io.Copy(countingWriter{src, toClient, connToClient}, dst)
		if ctx.Err() == nil {
			connection.fail(err)
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
	src.Close()
	dst.Close()
	wg.Wait()
}

type countingWriter struct {
	dst             io.Writer
	bytes           *atomic.Int64
	connectionBytes *atomic.Int64
}

func (w countingWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	w.bytes.Add(int64(n))
	if w.connectionBytes != nil {
		w.connectionBytes.Add(int64(n))
	}
	return n, err
}

func keepAlive(ctx context.Context, c *ssh.Client, interval time.Duration, name, password string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
		if err != nil {
			if ctx.Err() == nil {
				logging.FromContext(ctx).Warn("SSH keepalive failed", "tunnel", name, "error", logging.SafeError(err, password))
			}
			return
		}
	}
}
