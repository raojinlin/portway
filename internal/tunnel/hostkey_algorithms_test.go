package tunnel

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestTrustedHostKeyAlgorithms(t *testing.T) {
	for _, tc := range []struct {
		name, host, lookup string
		hashed             bool
	}{
		{"plain", "10.0.1.6:2222", "10.0.1.6:2222", false},
		{"hashed", "10.0.1.6:2222", "10.0.1.6:2222", true},
		{"IPv6", "[2001:db8::1]:2200", "[2001:db8::1]:2200", true},
		{"wildcard", "*.example.test:22", "one.example.test:22", false},
		{"different port", "10.0.1.6:2222", "10.0.1.6:22", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "known_hosts")
			host := knownhosts.Normalize(tc.host)
			if tc.hashed {
				host = knownhosts.HashHostname(host)
			}
			data := knownhosts.Line([]string{host}, genPublicKey(t)) + "\n"
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			cfg := Config{KnownHostsPath: path}
			algorithms, err := hostKeyAlgorithms(cfg, "alias", tc.lookup, "+ssh-rsa")
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "different port" {
				defaults, _, _ := configuredHostKeyAlgorithms("+ssh-rsa")
				if !reflect.DeepEqual(algorithms, defaults) {
					t.Fatalf("wrong-port key affected order: %v", algorithms)
				}
			} else if algorithms[0] != ssh.KeyAlgoED25519 {
				t.Fatalf("ED25519 not preferred: %v", algorithms)
			}
			if !slices.Contains(algorithms, ssh.KeyAlgoRSA) {
				t.Fatal("explicit +ssh-rsa lost")
			}
			algorithms, err = hostKeyAlgorithms(cfg, "alias", tc.lookup, "-ssh-ed25519")
			if err != nil || slices.Contains(algorithms, ssh.KeyAlgoED25519) {
				t.Fatalf("excluded key reintroduced: %v %v", algorithms, err)
			}
			algorithms, err = hostKeyAlgorithms(cfg, "alias", tc.lookup, "ecdsa-sha2-nistp256,ssh-ed25519")
			if err != nil || algorithms[0] != ssh.KeyAlgoECDSA256 {
				t.Fatalf("explicit order not honored: %v %v", algorithms, err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != data {
				t.Fatal("algorithm discovery modified known_hosts")
			}
		})
	}
}

func TestRSAHostKeyUsesSHA2Algorithms(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(path, []byte(knownhosts.Line([]string{"host"}, pub)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	algorithms, err := hostKeyAlgorithms(Config{KnownHostsPath: path}, "host", "host:22", "")
	if err != nil {
		t.Fatal(err)
	}
	if algorithms[0] != ssh.KeyAlgoRSASHA256 || algorithms[1] != ssh.KeyAlgoRSASHA512 {
		t.Fatalf("RSA key must prefer SHA2 signatures: %v", algorithms)
	}
	if slices.Contains(algorithms, ssh.KeyAlgoRSA) {
		t.Fatal("legacy SHA1 algorithm enabled without explicit configuration")
	}
}

func TestConfiguredHostKeyAlgorithms(t *testing.T) {
	for _, tc := range []struct {
		spec, first  string
		prefer, fail bool
	}{
		{"ssh-ed25519,ecdsa-sha2-nistp256", ssh.KeyAlgoED25519, false, false},
		{"^ssh-ed25519", ssh.KeyAlgoED25519, false, false},
		{"+ssh-rsa", "", true, false},
		{"-ecdsa-sha2-*", "", true, false},
		{"-unknown", "", true, false},
		{"unknown", "", false, true},
		{"+unknown", "", false, true},
		{"+", "", false, true},
		{"-*", "", false, true},
		{"ssh-ed25519,", "", false, true},
	} {
		algorithms, prefer, err := configuredHostKeyAlgorithms(tc.spec)
		if tc.fail {
			if err == nil {
				t.Errorf("accepted invalid %q", tc.spec)
			}
			continue
		}
		if err != nil || prefer != tc.prefer || (tc.first != "" && algorithms[0] != tc.first) {
			t.Errorf("%s => %v %v %v", tc.spec, algorithms, prefer, err)
		}
		seen := make(map[string]bool)
		for _, algorithm := range algorithms {
			if seen[algorithm] {
				t.Errorf("duplicate %s", algorithm)
			}
			seen[algorithm] = true
		}
	}
}

func TestCertificateAuthorityAndRevokedAlgorithms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	key := genPublicKey(t)
	data := "@cert-authority " + knownhosts.Line([]string{"host"}, key) + "\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	algorithms, err := hostKeyAlgorithms(Config{KnownHostsPath: path}, "host", "host:22", "")
	if err != nil || !strings.Contains(algorithms[0], "-cert-v01@openssh.com") {
		t.Fatalf("CA lost certificate preference: %v %v", algorithms, err)
	}
	data = "@revoked " + knownhosts.Line([]string{"host"}, key) + "\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	algorithms, err = hostKeyAlgorithms(Config{KnownHostsPath: path}, "host", "host:22", "")
	if err != nil || !reflect.DeepEqual(algorithms, ssh.SupportedAlgorithms().HostKeys) {
		t.Fatalf("revoked entry preferred: %v %v", algorithms, err)
	}
	callback, err := buildHostKeyCallback(Config{KnownHostsPath: path, TrustNewHostKey: true}, "host")
	if err != nil {
		t.Fatal(err)
	}
	err = callback("host:22", fakeAddr{"host:22"}, key)
	var revoked *knownhosts.RevokedError
	if !errors.As(err, &revoked) || !strings.Contains(err.Error(), path+":1") {
		t.Fatalf("revoked key accepted/lost diagnostic: %v", err)
	}
}

func TestHostKeyMismatchDiagnostics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	trusted, changed := genPublicKey(t), genPublicKey(t)
	data := "# comment\n" + knownhosts.Line([]string{"[host]:2222"}, trusted) + "\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tofu := range []bool{false, true} {
		callback, err := buildHostKeyCallback(Config{KnownHostsPath: path, TrustNewHostKey: tofu}, "host")
		if err != nil {
			t.Fatal(err)
		}
		err = callback("host:2222", fakeAddr{"host:2222"}, changed)
		var mismatch *knownhosts.KeyError
		if !errors.As(err, &mismatch) || len(mismatch.Want) != 1 {
			t.Fatalf("key mismatch type lost: %v", err)
		}
		for _, want := range []string{"key mismatch", "host:2222", "ssh-ed25519", ssh.FingerprintSHA256(changed), ssh.FingerprintSHA256(trusted), path + ":2"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("missing %s in %v", want, err)
			}
		}
	}
	got, _ := os.ReadFile(path)
	if string(got) != data {
		t.Fatal("changed key replaced trusted key")
	}
}
