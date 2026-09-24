package main

import (
	"runtime"
	"testing"
	"time"

	"ssh-tunnel-manager/internal/daemon"
	"ssh-tunnel-manager/internal/tunnel"
)

func TestTrayProxyCommands(t *testing.T) {
	for listen, want := range map[string]string{
		"127.0.0.1:1080": "socks5h://127.0.0.1:1080", "0.0.0.0:1080": "socks5h://127.0.0.1:1080",
		":1080": "socks5h://127.0.0.1:1080", "[::]:1080": "socks5h://[::1]:1080", "proxy.local:8080": "socks5h://proxy.local:8080",
	} {
		url, command := trayProxy(listen)
		wantCommand := "export all_proxy='" + want + "' http_proxy='" + want + "' https_proxy='" + want + "'"
		if runtime.GOOS == "windows" {
			wantCommand = "$env:all_proxy='" + want + "'; $env:http_proxy=$env:all_proxy; $env:https_proxy=$env:all_proxy"
		}
		if url != want || command != wantCommand {
			t.Fatalf("%s: %q %q", listen, url, command)
		}
	}
	for _, listen := range []string{"", "localhost", "host:0", "host:65536", "$(whoami):1080", "foo';echo x:1080"} {
		if url, command := trayProxy(listen); url != "" || command != "" {
			t.Fatalf("unsafe proxy command: %s", command)
		}
	}
}

func TestProxyActionsOnlyForSOCKS(t *testing.T) {
	for _, direction := range []string{"local", "remote", "dynamic"} {
		var sampler traySampler
		for _, state := range []string{"running", "stopped", "starting", "error", "stopped", "running", "unknown"} {
			result := sampler.sample([]daemon.TunnelView{{Status: tunnel.Status{Name: "example", State: state}, Direction: direction, LocalListen: "127.0.0.1:1080"}}, time.Now())
			want := direction == "dynamic" && state != "stopped" && state != "unknown"
			if result.Lines[0].ShowHistory != (direction == "dynamic") {
				t.Fatal("history navigation lost on stopped SOCKS tunnel")
			}
			if line := result.Lines[0]; (line.ProxyURL != "") != want || (line.ProxyCommand != "") != want {
				t.Fatalf("%s/%s: %+v", direction, state, line)
			}
		}
	}
}
