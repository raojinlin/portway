package tunnel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type stalledKeepAliveClient struct {
	closed chan struct{}
	once   sync.Once
}

func (c *stalledKeepAliveClient) SendRequest(string, bool, []byte) (bool, []byte, error) {
	<-c.closed
	return false, nil, net.ErrClosed
}

func (c *stalledKeepAliveClient) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func TestKeepAliveTimeoutClosesStaleSSHConnection(t *testing.T) {
	client := &stalledKeepAliveClient{closed: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		keepAlive(context.Background(), client, time.Millisecond, 5*time.Millisecond, "sleep-wake", "")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("keepalive timeout did not close stale SSH connection")
	}
	select {
	case <-client.closed:
	default:
		t.Fatal("stale SSH connection remained open")
	}
}

func TestValidateConfig(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "missing name",
			cfg:     Config{Direction: DirectionLocal, LocalListen: "127.0.0.1:0", SSHAddress: "h:22", ForwardAddress: "t:80"},
			wantErr: true,
		},
		{
			name:    "local ok",
			cfg:     Config{Name: "a", Direction: DirectionLocal, LocalListen: "127.0.0.1:0", SSHAddress: "h:22", ForwardAddress: "t:80"},
			wantErr: false,
		},
		{
			name:    "local missing forward",
			cfg:     Config{Name: "a", Direction: DirectionLocal, LocalListen: "127.0.0.1:0", SSHAddress: "h:22"},
			wantErr: true,
		},
		{
			name:    "empty direction defaults to local, same rules",
			cfg:     Config{Name: "a", LocalListen: "127.0.0.1:0", SSHAddress: "h:22", ForwardAddress: "t:80"},
			wantErr: false,
		},
		{
			name:    "empty direction missing forward",
			cfg:     Config{Name: "a", LocalListen: "127.0.0.1:0", SSHAddress: "h:22"},
			wantErr: true,
		},
		{
			name:    "remote ok",
			cfg:     Config{Name: "a", Direction: DirectionRemote, SSHAddress: "h:22", RemoteListen: "0.0.0.0:9000", ForwardAddress: "t:80"},
			wantErr: false,
		},
		{
			name:    "remote missing remote listen",
			cfg:     Config{Name: "a", Direction: DirectionRemote, SSHAddress: "h:22", ForwardAddress: "t:80"},
			wantErr: true,
		},
		{
			name:    "dynamic ok, no forward address needed",
			cfg:     Config{Name: "a", Direction: DirectionDynamic, LocalListen: "127.0.0.1:0", SSHAddress: "h:22"},
			wantErr: false,
		},
		{
			name:    "dynamic missing local listen",
			cfg:     Config{Name: "a", Direction: DirectionDynamic, SSHAddress: "h:22"},
			wantErr: true,
		},
		{
			name:    "unknown direction",
			cfg:     Config{Name: "a", Direction: "bogus", SSHAddress: "h:22"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateConfig(tc.cfg)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSSHConfigIdentityIsResolvedBeforeAuth(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_ed25519")
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate private key: %v", err)
	}
	pemBlock, err := ssh.MarshalPrivateKey(privateKey, "test key")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	configPath := filepath.Join(dir, "config")
	config := "Host drop\n  HostName 10.0.1.6\n  Port 2222\n  User testuser\n  IdentityFile " + keyPath + "\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write ssh config: %v", err)
	}

	host, user, resolvedKey, err := resolveSSHSettings(Config{
		SSHAddress:    "drop",
		SSHConfigPath: configPath,
	})
	if err != nil {
		t.Fatalf("resolveSSHSettings: %v", err)
	}
	if host != "10.0.1.6:2222" || user != "testuser" || resolvedKey != keyPath {
		t.Fatalf("resolved settings = (%q, %q, %q)", host, user, resolvedKey)
	}
	auth, closeAuth, err := buildSSHAuth(context.Background(), Config{SSHKeyPath: resolvedKey}, SSHConfig{IdentityAgent: "none"})
	defer closeAuth()
	if err != nil {
		t.Fatalf("buildSSHAuth using IdentityFile: %v", err)
	}
	if len(auth) != 1 {
		t.Fatalf("got %d auth methods, want 1", len(auth))
	}
}
