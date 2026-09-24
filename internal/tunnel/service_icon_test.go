package tunnel

import "testing"

func TestServiceIcons(t *testing.T) {
	for port, want := range map[string]string{"22": "ssh", "3306": "mysql", "5432": "postgresql", "80": "http", "443": "https", "6379": "redis", "27017": "mongodb", "3389": "rdp", "8080": "http", "8443": "https", "9999": "generic"} {
		for _, direction := range []Direction{DirectionLocal, DirectionRemote} {
			cfg := Config{Direction: direction, ForwardAddress: "[::1]:" + port, SSHAddress: "jump:22", LocalListen: "127.0.0.1:3306"}
			if got := cfg.ResolvedServiceIcon(); got != want {
				t.Fatalf("%s/%s: got %s, want %s", direction, port, got, want)
			}
			cfg.ServiceIcon = "redis"
			if cfg.ResolvedServiceIcon() != "redis" {
				t.Fatal("manual icon ignored")
			}
		}
	}
	if (Config{Direction: DirectionDynamic, ForwardAddress: "localhost:22"}).ResolvedServiceIcon() != "socks5" {
		t.Fatal("SOCKS5 should not use stale target")
	}
	if (Config{SSHAddress: "host:22", LocalListen: "localhost:3306"}).ResolvedServiceIcon() != "generic" {
		t.Fatal("detected transport or listener as target")
	}
	if ValidServiceIcon("https://example.com/icon.svg") {
		t.Fatal("external icon accepted")
	}
}
