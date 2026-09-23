package main

import (
	"strings"
	"testing"
	"time"

	"ssh-tunnel-manager/internal/daemon"
	"ssh-tunnel-manager/internal/tunnel"
)

func TestTrayRatesAndDetails(t *testing.T) {
	start := time.Unix(100, 0)
	views := []daemon.TunnelView{{
		Status:  tunnel.Status{Name: "drop", State: "running", StartedAt: start, BytesIn: 1024, BytesOut: 512, ActiveConns: 2},
		Enabled: true, Direction: "local", LocalListen: "127.0.0.1:2223", SSHAddress: "drop", ForwardAddress: "0.0.0.0:2222",
	}}
	var sampler traySampler
	first := sampler.sample(views, start)
	if !strings.Contains(first.Lines[0].Title, "采样中") {
		t.Fatal("first sample invented a transfer rate")
	}
	views[0].BytesIn += 4096
	views[0].BytesOut += 2048
	result := sampler.sample(views, start.Add(4*time.Second))
	if !strings.Contains(result.Lines[0].Title, "↓ 1.0 KiB/s  ↑ 512 B/s") {
		t.Fatalf("rate did not use actual elapsed time: %+v", result)
	}
	if !strings.Contains(result.Traffic, "2 连接") || !strings.Contains(result.Traffic, "1.0 KiB/s") {
		t.Fatal(result.Traffic)
	}
	details := strings.Join(result.Lines[0].Details, "\n")
	for _, want := range []string{"累计：↓ 5.0 KiB  ↑ 2.5 KiB", "当前连接：2", "本地监听：127.0.0.1:2223", "SSH 跳板：drop", "目标服务：0.0.0.0:2222"} {
		if !strings.Contains(details, want) {
			t.Fatalf("missing %q in %s", want, details)
		}
	}
	idle := sampler.sample(views, start.Add(6*time.Second))
	if !strings.Contains(idle.Traffic, "↓ 0 B/s  ↑ 0 B/s") {
		t.Fatal(idle.Traffic)
	}
}

func TestTrayResetsRateBaseline(t *testing.T) {
	for _, change := range []string{"restart", "counter reset", "removed", "same time", "clock backwards", "recover"} {
		t.Run(change, func(t *testing.T) {
			now := time.Unix(100, 0)
			view := daemon.TunnelView{Status: tunnel.Status{Name: "line", State: "running", StartedAt: now, BytesIn: 1024}, Enabled: true}
			var sampler traySampler
			if change == "recover" {
				view.State = "error"
			}
			sampler.sample([]daemon.TunnelView{view}, now)
			view.State = "running"
			view.BytesIn += 2048
			next := now.Add(2 * time.Second)
			switch change {
			case "restart":
				view.StartedAt = next
			case "counter reset":
				view.BytesIn = 1
			case "removed":
				sampler.sample(nil, now.Add(time.Second))
			case "same time":
				next = now
			case "clock backwards":
				next = now.Add(-time.Second)
			}
			result := sampler.sample([]daemon.TunnelView{view}, next)
			if !strings.Contains(result.Lines[0].Title, "采样中") {
				t.Fatalf("invalid rate across %s: %+v", change, result)
			}
		})
	}
}

func TestTrayStatesRoutesAndTotals(t *testing.T) {
	var sampler traySampler
	now := time.Unix(100, 0)
	views := []daemon.TunnelView{
		{Status: tunnel.Status{Name: "local", State: "running", StartedAt: now, ActiveConns: 2}, Direction: "local"},
		{Status: tunnel.Status{Name: "remote", State: "running", StartedAt: now, ActiveConns: 3}, Direction: "remote", RemoteListen: "0.0.0.0:80", ForwardAddress: "127.0.0.1:8081"},
		{Status: tunnel.Status{Name: "socks", State: "starting"}, Direction: "dynamic", LocalListen: "127.0.0.1:1080"},
		{Status: tunnel.Status{Name: "broken", State: "error", LastError: "host unreachable"}},
		{Status: tunnel.Status{Name: "disabled", State: "stopped"}},
	}
	sampler.sample(views, now)
	views[0].BytesIn = 2048
	views[1].BytesIn = 4096
	result := sampler.sample(views, now.Add(2*time.Second))
	if result.Summary != "运行 2 · 连接中 1 · 异常 1 · 停止 1" {
		t.Fatal(result.Summary)
	}
	if !strings.Contains(result.Traffic, "3.0 KiB/s") || !strings.Contains(result.Traffic, "5 连接") {
		t.Fatal(result.Traffic)
	}
	wantStates := []string{"running", "running", "starting", "error", "stopped"}
	for i, want := range wantStates {
		if result.Lines[i].State != want {
			t.Fatalf("line %d state = %q; want %q", i, result.Lines[i].State, want)
		}
	}
	if !strings.Contains(strings.Join(result.Lines[1].Details, "\n"), "远程监听：0.0.0.0:80") {
		t.Fatal(result.Lines[1])
	}
	if !strings.Contains(strings.Join(result.Lines[2].Details, "\n"), "目标：由客户端指定") {
		t.Fatal(result.Lines[2])
	}
	if !strings.Contains(strings.Join(result.Lines[3].Details, "\n"), "host unreachable") {
		t.Fatal(result.Lines[3])
	}
	empty := sampler.sample(nil, now.Add(4*time.Second))
	if len(empty.Lines) != 0 || len(sampler.previous) != 0 {
		t.Fatal("deleted tunnels remain in tray")
	}
}

func TestTrayRequestErrorDoesNotChangeRunningState(t *testing.T) {
	var sampler traySampler
	result := sampler.sample([]daemon.TunnelView{{Status: tunnel.Status{Name: "socks", State: "running", LastError: "dial target: timed out"}, Direction: "dynamic"}}, time.Now())
	if !strings.Contains(result.Summary, "运行 1") || !strings.Contains(result.Summary, "异常 0") {
		t.Fatal(result.Summary)
	}
}

func TestTrayFormatting(t *testing.T) {
	if trayBytes(1024) != "1.0 KiB" || trayBytes(-1) != "0 B" || trayBytes(0) != "0 B" {
		t.Fatal("unexpected byte formatting")
	}
	if trayShort("line\nname\tlong", 9) != "line name…" {
		t.Fatal("unsafe or unbounded menu title")
	}
	if trayShort("中文线路名字", 4) != "中文线路…" {
		t.Fatal("name truncation damaged Unicode")
	}
}
