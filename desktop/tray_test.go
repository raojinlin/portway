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
	if !first.Lines[0].Enabled {
		t.Fatal("tray lost enabled state used by start/stop actions")
	}
	if first.StatusText != "--\n--" {
		t.Fatalf("first status bar sample invented a transfer rate: %q", first.StatusText)
	}
	views[0].BytesIn += 4096
	views[0].BytesOut += 2048
	result := sampler.sample(views, start.Add(4*time.Second))
	if strings.Contains(result.Lines[0].Title, "运行中") {
		t.Fatalf("title repeats state represented by its status dot: %q", result.Lines[0].Title)
	}
	if !strings.Contains(result.Lines[0].Title, "↓ 1.0 KiB/s  ↑ 512 B/s") {
		t.Fatalf("rate did not use actual elapsed time: %+v", result)
	}
	if result.Lines[0].RateText != "↓ 1.0 KiB/s  ↑ 512 B/s" {
		t.Fatal("missing separate rate column")
	}
	if !strings.Contains(result.Traffic, "2 连接") || !strings.Contains(result.Traffic, "1.0 KiB/s") {
		t.Fatal(result.Traffic)
	}
	if result.StatusText != "512 B/s\n1.0 KiB/s" {
		t.Fatalf("incorrect status bar upload/download rates: %q", result.StatusText)
	}
	details := strings.Join(result.Lines[0].Details, "\n")
	if len(result.Lines[0].Details) != 5 {
		t.Fatalf("details contain redundant rows: %q", result.Lines[0].Details)
	}
	for _, want := range []string{"累计：↓ 5.0 KiB  ↑ 2.5 KiB", "当前连接：2", "本地监听：127.0.0.1:2223", "SSH 跳板：drop", "目标服务：0.0.0.0:2222"} {
		if !strings.Contains(details, want) {
			t.Fatalf("missing %q in %s", want, details)
		}
	}
	idle := sampler.sample(views, start.Add(6*time.Second))
	if !strings.Contains(idle.Traffic, "↓ 0 B/s  ↑ 0 B/s") {
		t.Fatal(idle.Traffic)
	}
	if idle.StatusText != "0 B/s\n0 B/s" {
		t.Fatal(idle.StatusText)
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
			if result.StatusText != "--\n--" {
				t.Fatalf("invalid status bar rate across %s: %q", change, result.StatusText)
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
	views[0].BytesOut = 1024
	views[1].BytesOut = 3072
	result := sampler.sample(views, now.Add(2*time.Second))
	if result.Summary != "运行 2 · 连接中 1 · 异常 1 · 停止 1" {
		t.Fatal(result.Summary)
	}
	if !strings.Contains(result.Traffic, "3.0 KiB/s") || !strings.Contains(result.Traffic, "5 连接") {
		t.Fatal(result.Traffic)
	}
	if result.StatusText != "2.0 KiB/s\n3.0 KiB/s" {
		t.Fatalf("incorrect aggregate status bar rates: %q", result.StatusText)
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
	if empty.StatusText != "0 B/s\n0 B/s" {
		t.Fatal(empty.StatusText)
	}
}

func TestTrayRequestErrorDoesNotChangeRunningState(t *testing.T) {
	var sampler traySampler
	result := sampler.sample([]daemon.TunnelView{{Status: tunnel.Status{Name: "socks", State: "running", LastError: "dial target: timed out"}, Direction: "dynamic"}}, time.Now())
	if result.Summary != "运行 1" {
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
	if traySummary(nil, "zh") != "暂无线路" || traySummary(map[string]int{"running": 2, "error": 1}, "zh") != "运行 2 · 异常 1" {
		t.Fatal("summary did not omit empty states")
	}
}

func TestTrayServiceIconsFollowTargetAndPreference(t *testing.T) {
	for _, test := range []struct{ direction, target, preference, want string }{
		{"local", "localhost:22", "", "ssh"},
		{"remote", "localhost:3306", "auto", "mysql"},
		{"local", "localhost:5432", "auto", "postgresql"},
		{"local", "localhost:80", "auto", "http"},
		{"local", "localhost:443", "auto", "https"},
		{"local", "localhost:5432", "redis", "redis"},
		{"dynamic", "", "auto", "socks5"},
		{"local", "localhost:9999", "auto", "generic"},
	} {
		var sampler traySampler
		view := daemon.TunnelView{Status: tunnel.Status{Name: "service", State: "running"}, Direction: test.direction, ForwardAddress: test.target, ServiceIcon: test.preference, SSHAddress: "jump:22"}
		line := sampler.sample([]daemon.TunnelView{view}, time.Now()).Lines[0]
		if line.ServiceIcon != test.want || line.State != "running" {
			t.Fatalf("%+v: %+v", test, line)
		}
		view.ServiceIcon, view.State = "rdp", "stopped"
		line = sampler.sample([]daemon.TunnelView{view}, time.Now()).Lines[0]
		if line.ServiceIcon != "rdp" || line.State != "stopped" {
			t.Fatal("icon preference or stopped state was lost")
		}
	}
}

func TestTrayLanguageSwitchPreservesRateBaseline(t *testing.T) {
	now := time.Unix(100, 0)
	views := []daemon.TunnelView{{Status: tunnel.Status{Name: "线路", State: "running", StartedAt: now, LastError: "raw SSH error"}, Direction: "dynamic"}}
	var sampler traySampler
	sampler.sample(views, now)
	sampler.language = "en"
	views[0].BytesIn = 2048
	views[0].BytesOut = 1024
	result := sampler.sample(views, now.Add(2*time.Second))
	if result.Summary != "Running 1" || result.StatusText != "512 B/s\n1.0 KiB/s" || result.Labels["quit"] != "Quit Portway" {
		t.Fatalf("language switch damaged rates or labels: %+v", result)
	}
	if !strings.HasPrefix(result.Lines[0].Title, "线路") || !strings.Contains(strings.Join(result.Lines[0].Details, "\n"), "Last error: raw SSH error") {
		t.Fatal("localized user data or lost raw error")
	}
	for _, direction := range []string{"local", "remote", "dynamic"} {
		views[0].Direction = direction
		result = sampler.sample(views, now.Add(4*time.Second))
		for _, detail := range result.Lines[0].Details {
			if !strings.Contains(detail, ": ") {
				t.Fatalf("missing English detail separator: %q", detail)
			}
		}
	}
	if traySummary(nil, "en") != "No tunnels" || trayRates(0, 0, false, "en") != "Sampling rates" {
		t.Fatal("missing English empty or loading state")
	}
	sampler.language = "zh"
	result = sampler.sample(views, now.Add(6*time.Second))
	if result.Summary != "运行 1" || result.Labels["quit"] != "退出 Portway" {
		t.Fatal(result)
	}
}
