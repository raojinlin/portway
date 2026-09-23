//go:build unix

package tunnel

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// A real SSH server over child-process stdio verifies the proxy transport
// without a listening socket or access to the user's actual SSH servers.
type stdioTestConn struct{}

func (stdioTestConn) Read(b []byte) (int, error)       { return os.Stdin.Read(b) }
func (stdioTestConn) Write(b []byte) (int, error)      { return os.Stdout.Write(b) }
func (stdioTestConn) Close() error                     { return nil }
func (stdioTestConn) LocalAddr() net.Addr              { return proxyAddr("192.0.2.10:2222") }
func (stdioTestConn) RemoteAddr() net.Addr             { return proxyAddr("127.0.0.1:1") }
func (stdioTestConn) SetDeadline(time.Time) error      { return nil }
func (stdioTestConn) SetReadDeadline(time.Time) error  { return nil }
func (stdioTestConn) SetWriteDeadline(time.Time) error { return nil }

func proxyTestSigner() ssh.Signer {
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{42}, ed25519.SeedSize))
	signer, _ := ssh.NewSignerFromKey(key)
	return signer
}

func TestProxyProcess(t *testing.T) {
	mode := os.Getenv("SSH_TUNNEL_PROXY_TEST")
	if mode == "" {
		return
	}
	switch mode {
	case "fail":
		fmt.Fprintln(os.Stderr, "jump authentication failed")
		os.Exit(1)
	case "hang":
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	args := os.Args[len(os.Args)-3:]
	if args[0] != "192.0.2.10" || args[1] != "2222" || args[2] != "alice" {
		fmt.Fprintln(os.Stderr, "incorrect proxy token expansion")
		os.Exit(2)
	}
	server := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if meta.User() != "alice" || string(password) != "test-password" {
			return nil, errors.New("incorrect target authentication")
		}
		return nil, nil
	}}
	server.AddHostKey(proxyTestSigner())
	conn, channels, requests, err := ssh.NewServerConn(stdioTestConn{}, server)
	if err != nil {
		os.Exit(3)
	}
	go func() {
		for ch := range channels {
			_ = ch.Reject(ssh.Prohibited, "test server")
		}
	}()
	for req := range requests {
		_ = req.Reply(req.Type == "probe", []byte("via-proxy"))
	}
	conn.Close()
	os.Exit(0)
}

func proxyTestCommand(t *testing.T, mode string) string {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return "SSH_TUNNEL_PROXY_TEST=" + mode + " '" + strings.ReplaceAll(binary, "'", "'\"'\"'") + "' -test.run=^TestProxyProcess$ -- %h %p %r"
}

func TestDialSSHUsesConfiguredProxyCommand(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	config := "Host pg-web1\n HostName 192.0.2.10\n Port 2222\n User alice\n IdentityAgent none\n ProxyCommand " + proxyTestCommand(t, "server") + "\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	knownPath := filepath.Join(dir, "known_hosts")
	line := knownhosts.Line([]string{"[192.0.2.10]:2222"}, proxyTestSigner().PublicKey()) + "\n"
	if err := os.WriteFile(knownPath, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	inst := &Instance{cfg: Config{
		SSHAddress: "pg-web1", SSHConfigPath: configPath,
		SSHPassword: "test-password", SSHKeyPath: "none", KnownHostsPath: knownPath,
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := inst.dialSSH(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ok, reply, err := client.SendRequest("probe", true, nil)
	if err != nil || !ok || string(reply) != "via-proxy" {
		t.Fatalf("proxy round trip = %v, %q, %v", ok, reply, err)
	}
	client.Close()

	line = knownhosts.Line([]string{"[192.0.2.10]:2222"}, genPublicKey(t)) + "\n"
	if err := os.WriteFile(knownPath, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err = inst.dialSSH(ctx)
	if client != nil {
		client.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "key mismatch") {
		t.Fatalf("target host key must still be verified through proxy: %v", err)
	}
}

func TestProxyCommandFailureAndTimeout(t *testing.T) {
	for _, tc := range []struct{ mode, want string }{
		{"fail", "jump authentication failed"},
		{"hang", "context deadline exceeded"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			cfg := &ssh.ClientConfig{User: "alice", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: time.Second}
			_, err := dialSSHTransport(context.Background(), "192.0.2.10:2222", "pg-web1",
				SSHConfig{ProxyCommand: proxyTestCommand(t, tc.mode)}, cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "ProxyCommand") {
				t.Fatalf("error = %v; want proxy failure containing %q", err, tc.want)
			}
		})
	}
}

func TestProxyCloseReapsProcess(t *testing.T) {
	command, err := expandProxyCommand(proxyTestCommand(t, "hang"), "192.0.2.10:2222", "pg-web1", "alice")
	if err != nil {
		t.Fatal(err)
	}
	p, err := startProxyCommand(context.Background(), command, "192.0.2.10:2222")
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.done:
	default:
		t.Fatal("proxy process was not reaped")
	}
	if p.cmd.ProcessState == nil {
		t.Fatal("missing process exit state")
	}
}

func TestExpandProxyCommand(t *testing.T) {
	got, err := expandProxyCommand("ssh -W %h:%p -q drop # %r %n %%", "10.0.3.179:22", "pg-web1", "alice")
	if err != nil || got != "ssh -W 10.0.3.179:22 -q drop # alice pg-web1 %" {
		t.Fatalf("expanded command = %q, %v", got, err)
	}
	if _, err := expandProxyCommand("ssh -W %h:%p drop", "$(touch bad):22", "test", "alice"); err == nil {
		t.Fatal("shell metacharacters in target must be rejected")
	}
}
