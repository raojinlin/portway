package tunnel

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func genPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("ssh.NewPublicKey: %v", err)
	}
	return sshPub
}

type fakeAddr struct{ s string }

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return a.s }

func TestBuildHostKeyCallback_MatchAndMismatch(t *testing.T) {
	// rawHost mirrors what ssh.Dial passes to the HostKeyCallback: the raw
	// "host:port" dial address, not a pre-normalized known_hosts entry.
	// knownhosts normalizes internally (and requires a port to do so).
	rawHost := "host-a:22"
	key1 := genPublicKey(t)
	key2 := genPublicKey(t)

	path := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{knownhosts.Normalize(rawHost)}, key1)
	if err := os.WriteFile(path, []byte(line+"\n"), 0o600); err != nil {
		t.Fatalf("write known_hosts: %v", err)
	}

	cb, err := buildHostKeyCallback(Config{KnownHostsPath: path}, "")
	if err != nil {
		t.Fatalf("buildHostKeyCallback: %v", err)
	}

	if err := cb(rawHost, fakeAddr{rawHost}, key1); err != nil {
		t.Fatalf("expected matching key to be accepted, got: %v", err)
	}
	if err := cb(rawHost, fakeAddr{rawHost}, key2); err == nil {
		t.Fatalf("expected mismatched key to be rejected")
	}
}

func TestBuildHostKeyCallback_MissingFileWithoutTrust(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	_, err := buildHostKeyCallback(Config{KnownHostsPath: path}, "")
	if err == nil {
		t.Fatalf("expected error for missing known_hosts without TrustNewHostKey")
	}
}

func TestBuildHostKeyCallback_InsecureSkipAcceptsMismatch(t *testing.T) {
	rawHost := "host-a:22"
	cb, err := buildHostKeyCallback(Config{
		KnownHostsPath:           filepath.Join(t.TempDir(), "missing-known-hosts"),
		InsecureSkipHostKeyCheck: true,
	}, "")
	if err != nil {
		t.Fatalf("buildHostKeyCallback: %v", err)
	}
	if err := cb(rawHost, fakeAddr{rawHost}, genPublicKey(t)); err != nil {
		t.Fatalf("insecure callback rejected host key: %v", err)
	}
}

func TestBuildHostKeyCallback_TrustOnFirstUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	rawHost := "host-b:22"
	key1 := genPublicKey(t)
	key2 := genPublicKey(t)

	cb, err := buildHostKeyCallback(Config{KnownHostsPath: path, TrustNewHostKey: true}, "")
	if err != nil {
		t.Fatalf("buildHostKeyCallback: %v", err)
	}

	if err := cb(rawHost, fakeAddr{rawHost}, key1); err != nil {
		t.Fatalf("expected unknown host to be trusted on first use, got: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read known_hosts after TOFU: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected known_hosts to be populated after TOFU")
	}

	// A rebuilt callback (simulating the next connection) must now enforce
	// the recorded key, even though TrustNewHostKey is still true.
	cb2, err := buildHostKeyCallback(Config{KnownHostsPath: path, TrustNewHostKey: true}, "")
	if err != nil {
		t.Fatalf("buildHostKeyCallback (2nd): %v", err)
	}
	if err := cb2(rawHost, fakeAddr{rawHost}, key1); err != nil {
		t.Fatalf("expected previously trusted key to still match: %v", err)
	}
	if err := cb2(rawHost, fakeAddr{rawHost}, key2); err == nil {
		t.Fatalf("expected key change on a known host to be rejected even with TrustNewHostKey")
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir available")
	}
	got, err := expandHome("~/known_hosts")
	if err != nil {
		t.Fatalf("expandHome: %v", err)
	}
	want := filepath.Join(home, "known_hosts")
	if got != want {
		t.Fatalf("expandHome = %q, want %q", got, want)
	}
}

var _ net.Addr = fakeAddr{}
