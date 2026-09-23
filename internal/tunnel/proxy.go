package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"ssh-tunnel-manager/internal/logging"

	"golang.org/x/crypto/ssh"
)

func dialSSHTransport(ctx context.Context, address, alias string, settings SSHConfig, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	dialCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	var conn net.Conn
	var proxy *proxyConn
	var err error
	if settings.ProxyCommand != "" && settings.ProxyCommand != "none" {
		command, expandErr := expandProxyCommand(settings.ProxyCommand, address, alias, cfg.User)
		if expandErr != nil {
			return nil, expandErr
		}
		proxy, err = startProxyCommand(ctx, command, address)
		conn = proxy
	} else if settings.ProxyCommand == "" && settings.ProxyJump != "" && settings.ProxyJump != "none" {
		return nil, errors.New("unresolved ProxyJump: native SSH routing is required")
	} else {
		conn, err = (&net.Dialer{}).DialContext(dialCtx, "tcp", address)
	}
	if err != nil {
		return nil, err
	}

	// ClientConfig.Timeout only applies to ssh.Dial's TCP dial. Closing the
	// transport also bounds SSH handshakes and blocked proxy subprocesses.
	stop := context.AfterFunc(dialCtx, func() { conn.Close() })
	c, channels, requests, err := ssh.NewClientConn(conn, address, cfg)
	stop()
	if err != nil || dialCtx.Err() != nil {
		conn.Close()
		if dialCtx.Err() != nil {
			err = dialCtx.Err()
		}
		if proxy != nil {
			return nil, &proxyTransportError{address: address, cause: err, diagnostic: proxy.diagnostic()}
		}
		return nil, err
	}
	return ssh.NewClient(c, channels, requests), nil
}

type proxyTransportError struct {
	address    string
	cause      error
	diagnostic string
}

func (e *proxyTransportError) Error() string {
	return fmt.Sprintf("ProxyCommand to %s: %v%s", e.address, e.cause, e.diagnostic)
}
func (e *proxyTransportError) Unwrap() error { return e.cause }
func (e *proxyTransportError) SafeLogError() string {
	return fmt.Sprintf("ProxyCommand to %s: %v (proxy stderr omitted)", e.address, e.cause)
}

func expandProxyCommand(command, address, alias, user string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	values := map[byte]string{'h': host, 'p': port, 'r': user, 'n': alias}
	var out strings.Builder
	for n := 0; n < len(command); n++ {
		if command[n] != '%' {
			out.WriteByte(command[n])
			continue
		}
		n++
		if n >= len(command) {
			return "", errors.New("ProxyCommand has a trailing %")
		}
		if command[n] == '%' {
			out.WriteByte('%')
			continue
		}
		value, ok := values[command[n]]
		if !ok {
			return "", fmt.Errorf("unsupported ProxyCommand token %%%c", command[n])
		}
		// Tokens may appear inside shell quotes. Restrict substituted values
		// instead of adding quotes that could change the configured command.
		for _, ch := range value {
			if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-:@[]", ch) {
				return "", fmt.Errorf("unsafe character in ProxyCommand token %%%c", command[n])
			}
		}
		out.WriteString(value)
	}
	return out.String(), nil
}

// stderr is bounded because a long-lived proxy can keep writing diagnostics.
type proxyStderr struct {
	mu   sync.Mutex
	data []byte
}

func (b *proxyStderr) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	const limit = 4096
	b.data = append(b.data, p[max(0, len(p)-limit):]...)
	if len(b.data) > limit {
		b.data = b.data[len(b.data)-limit:]
	}
	return len(p), nil
}

type proxyConn struct {
	reader, writer *os.File
	cmd            *exec.Cmd
	remote         string
	stderr         proxyStderr
	done           chan struct{}
	once           sync.Once
}

func startProxyCommand(ctx context.Context, command, address string) (*proxyConn, error) {
	logging.FromContext(ctx).Debug("external SSH proxy starting", "address", address)
	stdin, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	reader, stdout, err := os.Pipe()
	if err != nil {
		stdin.Close()
		writer.Close()
		return nil, err
	}
	p := &proxyConn{reader: reader, writer: writer, remote: address, done: make(chan struct{})}
	p.cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	configureProxyProcess(p.cmd)
	p.cmd.Stdin, p.cmd.Stdout, p.cmd.Stderr = stdin, stdout, &p.stderr
	p.cmd.WaitDelay = time.Second
	err = p.cmd.Start()
	stdin.Close()
	stdout.Close()
	if err != nil {
		reader.Close()
		writer.Close()
		return nil, fmt.Errorf("start ProxyCommand: %w", err)
	}
	go func() {
		_ = p.cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

func (p *proxyConn) Read(b []byte) (int, error)  { return p.reader.Read(b) }
func (p *proxyConn) Write(b []byte) (int, error) { return p.writer.Write(b) }
func (p *proxyConn) Close() error {
	p.once.Do(func() {
		select {
		case <-p.done:
		default:
			_ = p.cmd.Cancel()
		}
		p.writer.Close()
		p.reader.Close()
		<-p.done
	})
	return nil
}
func (p *proxyConn) diagnostic() string {
	p.stderr.mu.Lock()
	defer p.stderr.mu.Unlock()
	if detail := strings.TrimSpace(string(p.stderr.data)); detail != "" {
		return ": " + detail
	}
	return ""
}

type proxyAddr string

func (a proxyAddr) Network() string       { return "tcp" }
func (a proxyAddr) String() string        { return string(a) }
func (p *proxyConn) LocalAddr() net.Addr  { return proxyAddr("127.0.0.1:0") }
func (p *proxyConn) RemoteAddr() net.Addr { return proxyAddr(p.remote) }
func (p *proxyConn) SetDeadline(t time.Time) error {
	return errors.Join(p.SetReadDeadline(t), p.SetWriteDeadline(t))
}
func (p *proxyConn) SetReadDeadline(t time.Time) error  { return p.reader.SetReadDeadline(t) }
func (p *proxyConn) SetWriteDeadline(t time.Time) error { return p.writer.SetWriteDeadline(t) }
