package tunnel

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveSSHAddress(t *testing.T) {
	cases := []struct {
		name, address, config, want string
		wantErr                     bool
	}{
		{name: "alias without port", address: "pg-web1", config: "Host pg-web1\n HostName 10.0.3.179\n", want: "10.0.3.179:22"},
		{name: "alias configured port", address: "pg-web1", config: "Host pg-web1\n HostName 10.0.3.179\n Port 2222\n", want: "10.0.3.179:2222"},
		{name: "explicit port wins", address: "pg-web1:2200", config: "Host pg-web1\n HostName 10.0.3.179\n Port 2222\n", want: "10.0.3.179:2200"},
		{name: "wildcard port", address: "10.0.3.179", config: "Host *\n Port 2222\n", want: "10.0.3.179:2222"},
		{name: "explicit port beats wildcard", address: "10.0.3.179:2200", config: "Host *\n Port 2222\n", want: "10.0.3.179:2200"},
		{name: "no config IPv4", address: "10.0.3.179", want: "10.0.3.179:22"},
		{name: "no config hostname", address: "example.test", want: "example.test:22"},
		{name: "no config explicit port", address: "example.test:2222", want: "example.test:2222"},
		{name: "IPv6", address: "2001:db8::1", want: "[2001:db8::1]:22"},
		{name: "IPv6 brackets", address: "[2001:db8::1]", want: "[2001:db8::1]:22"},
		{name: "IPv6 explicit port", address: "[2001:db8::1]:2222", want: "[2001:db8::1]:2222"},
		{name: "IPv6 alias", address: "pg-web1", config: "Host pg-web1\n HostName 2001:db8::1\n", want: "[2001:db8::1]:22"},
		{name: "empty port", address: "example.test:", wantErr: true},
		{name: "invalid port", address: "example.test:abc", wantErr: true},
		{name: "out of range port", address: "example.test:65536", wantErr: true},
		{name: "invalid configured port", address: "pg-web1", config: "Host pg-web1\n Port 0\n", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			if tc.config != "" {
				if err := os.WriteFile(path, []byte(tc.config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, _, _, err := resolveSSHSettings(Config{SSHAddress: tc.address, SSHConfigPath: path})
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected invalid address error")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("resolved address = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestSSHAuthenticationAndJumpSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	config := "Host app\n User alice\n IdentityFile ~/.ssh/first\n IdentityFile ~/.ssh/second\n IdentityAgent ~/agent.sock\n IdentitiesOnly yes\n ProxyJump drop,alice@inner:2222\n HostkeyAlgorithms +ssh-rsa\nHost *\n IdentityFile ~/.ssh/common\n"
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadSSHConfig("app", path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(settings.Identities, []string{"~/.ssh/first", "~/.ssh/second", "~/.ssh/common"}) || !settings.IdentitiesOnly || settings.IdentityAgent != "~/agent.sock" || settings.ProxyJump != "drop,alice@inner:2222" || settings.HostKeyAlgorithms != "+ssh-rsa" {
		t.Fatalf("SSH settings = %+v", settings)
	}
}

func TestAliasWithPortRetainsSSHSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	config := "Host pg-web1\n HostName 10.0.3.179\n User alice\n IdentityFile /keys/test-key\n UserKnownHostsFile /keys/known_hosts\n"
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{SSHAddress: "pg-web1:2200", SSHConfigPath: path}
	host, user, key, err := resolveSSHSettings(cfg)
	if err != nil || host != "10.0.3.179:2200" || user != "alice" || key != "/keys/test-key" {
		t.Fatalf("unexpected settings: %q, %q, %q, %v", host, user, key, err)
	}
	knownHosts, err := resolveKnownHostsPath(cfg, cfg.SSHAddress)
	if err != nil || knownHosts != "/keys/known_hosts" {
		t.Fatalf("known_hosts = %q, %v", knownHosts, err)
	}
}
