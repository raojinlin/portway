package tunnel

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func testPrivateKey(t *testing.T, path, passphrase string) (ed25519.PrivateKey, ssh.Signer) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var block *pem.Block
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(key, "test")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(key, "test", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
			t.Fatal(err)
		}
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return key, signer
}

// OS pipes provide buffered full-duplex I/O without opening any listening socket.
type testPipeConn struct{ reader, writer *os.File }

func (c *testPipeConn) Read(p []byte) (int, error)  { return c.reader.Read(p) }
func (c *testPipeConn) Write(p []byte) (int, error) { return c.writer.Write(p) }
func (c *testPipeConn) Close() error                { c.reader.Close(); return c.writer.Close() }
func (c *testPipeConn) LocalAddr() net.Addr         { return fakeAddr{"127.0.0.1:10000"} }
func (c *testPipeConn) RemoteAddr() net.Addr        { return fakeAddr{"127.0.0.1:22"} }
func (c *testPipeConn) SetDeadline(d time.Time) error {
	return errors.Join(c.SetReadDeadline(d), c.SetWriteDeadline(d))
}
func (c *testPipeConn) SetReadDeadline(d time.Time) error  { return c.reader.SetReadDeadline(d) }
func (c *testPipeConn) SetWriteDeadline(d time.Time) error { return c.writer.SetWriteDeadline(d) }

func bufferedTestPipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ar, bw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	br, aw, err := os.Pipe()
	if err != nil {
		ar.Close()
		bw.Close()
		t.Fatal(err)
	}
	a, b := &testPipeConn{ar, aw}, &testPipeConn{br, bw}
	t.Cleanup(func() { a.Close(); b.Close() })
	a.SetDeadline(time.Now().Add(5 * time.Second))
	b.SetDeadline(time.Now().Add(5 * time.Second))
	return a, b
}

func testAuthentication(t *testing.T, methods []ssh.AuthMethod, accepted ssh.PublicKey, wantSuccess bool) {
	t.Helper()
	client, server := bufferedTestPipe(t)
	_, hostKey := testPrivateKey(t, "", "")
	config := &ssh.ServerConfig{PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if bytes.Equal(accepted.Marshal(), key.Marshal()) {
			return nil, nil
		}
		return nil, errors.New("unexpected public key")
	}}
	config.AddHostKey(hostKey)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer server.Close()
		conn, channels, requests, err := ssh.NewServerConn(server, config)
		if err != nil {
			return
		}
		go ssh.DiscardRequests(requests)
		for ch := range channels {
			ch.Reject(ssh.Prohibited, "test")
		}
		conn.Close()
	}()
	conn, _, _, err := ssh.NewClientConn(client, "test:22", &ssh.ClientConfig{User: "tester", Auth: methods, HostKeyCallback: ssh.InsecureIgnoreHostKey()})
	if (err == nil) != wantSuccess {
		t.Errorf("authentication error = %v, want success=%v", err, wantSuccess)
	}
	if conn != nil {
		conn.Close()
	}
	client.Close()
	<-done
}

func noAgentDial(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("agent unavailable")
}

func TestDefaultSSHKeysAndExplicitOverride(t *testing.T) {
	home := t.TempDir()
	_, defaultKey := testPrivateKey(t, filepath.Join(home, ".ssh", "id_rsa"), "")
	explicitPath := filepath.Join(home, ".ssh", "custom")
	_, explicitKey := testPrivateKey(t, explicitPath, "")
	for _, tc := range []struct {
		name, path string
		key        ssh.PublicKey
	}{
		{"default", "", defaultKey.PublicKey()}, {"explicit", explicitPath, explicitKey.PublicKey()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			methods, cleanup, err := loadSSHAuth(context.Background(), Config{SSHKeyPath: tc.path}, SSHConfig{IdentityAgent: "none"}, home, noAgentDial)
			defer cleanup()
			if err != nil {
				t.Fatal(err)
			}
			testAuthentication(t, methods, tc.key, true)
			if tc.path != "" {
				testAuthentication(t, methods, defaultKey.PublicKey(), false)
			}
		})
	}
}

func TestMultipleIdentityFiles(t *testing.T) {
	home := t.TempDir()
	first, second := filepath.Join(home, "one"), filepath.Join(home, "two")
	testPrivateKey(t, first, "")
	_, key := testPrivateKey(t, second, "")
	methods, cleanup, err := loadSSHAuth(context.Background(), Config{}, SSHConfig{Identities: []string{first, second}, IdentityAgent: "none"}, home, noAgentDial)
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	testAuthentication(t, methods, key.PublicKey(), true)
}

func TestSSHAgentAuthenticationAndFiltering(t *testing.T) {
	for _, mode := range []string{"agent-only", "encrypted-selected", "public-selected", "identities-only-rejects-other"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			keyPath := filepath.Join(home, "encrypted")
			privateKey, key := testPrivateKey(t, keyPath, "secret")
			keyring := agent.NewKeyring()
			if err := keyring.Add(agent.AddedKey{PrivateKey: privateKey}); err != nil {
				t.Fatal(err)
			}
			settings := SSHConfig{IdentityAgent: "test-agent", Identities: []string{"none"}}
			switch mode {
			case "encrypted-selected":
				settings.Identities, settings.IdentitiesOnly = []string{keyPath}, true
			case "public-selected":
				publicPath := filepath.Join(home, "public")
				if err := os.WriteFile(publicPath, ssh.MarshalAuthorizedKey(key.PublicKey()), 0600); err != nil {
					t.Fatal(err)
				}
				settings.Identities, settings.IdentitiesOnly = []string{publicPath}, true
			case "identities-only-rejects-other":
				settings.IdentitiesOnly = true
			}
			a, b := net.Pipe()
			done := make(chan struct{})
			go func() { defer close(done); defer b.Close(); agent.ServeAgent(keyring, b) }()
			t.Cleanup(func() { a.Close(); b.Close() })
			methods, cleanup, err := loadSSHAuth(context.Background(), Config{}, settings, home, func(_ context.Context, network, address string) (net.Conn, error) {
				if network != "unix" || address != "test-agent" {
					t.Errorf("agent dial = %s %s", network, address)
				}
				return a, nil
			})
			defer cleanup()
			if mode == "identities-only-rejects-other" {
				if err == nil {
					t.Fatal("unconfigured agent key was accepted")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				testAuthentication(t, methods, key.PublicKey(), true)
			}
			cleanup()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("agent connection leaked")
			}
		})
	}
}

func TestAuthErrorsAndAgentFallback(t *testing.T) {
	home := t.TempDir()
	encryptedPath := filepath.Join(home, "encrypted")
	testPrivateKey(t, encryptedPath, "secret")
	for _, tc := range []struct{ name, path, password, want string }{
		{"no keys", "", "", "no usable SSH authentication"},
		{"encrypted", encryptedPath, "", "encrypted key"},
		{"missing", filepath.Join(home, "missing"), "", "read key"},
		{"password fallback", "", "password", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			methods, cleanup, err := loadSSHAuth(context.Background(), Config{SSHKeyPath: tc.path, SSHPassword: tc.password}, SSHConfig{IdentityAgent: "unavailable"}, home, noAgentDial)
			defer cleanup()
			if tc.want == "" {
				if err != nil || len(methods) != 1 {
					t.Fatalf("fallback = %v %v", methods, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestAgentCancellation(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, cleanup, err := loadSSHAuth(ctx, Config{}, SSHConfig{IdentityAgent: "blocked"}, t.TempDir(), func(context.Context, string, string) (net.Conn, error) { return a, nil })
	defer cleanup()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked agent error = %v", err)
	}
}

func TestUnresponsiveAgentDoesNotBlockPassword(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	methods, cleanup, err := loadSSHAuth(ctx, Config{SSHPassword: "test"}, SSHConfig{IdentityAgent: "blocked"}, t.TempDir(), func(context.Context, string, string) (net.Conn, error) { return a, nil })
	defer cleanup()
	if err != nil || len(methods) != 1 {
		t.Fatalf("password fallback = %v %v", methods, err)
	}
}
