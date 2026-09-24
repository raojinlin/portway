package main

import (
	"fmt"
	"strings"
	"time"

	"ssh-tunnel-manager/desktop/platform"
	"ssh-tunnel-manager/internal/daemon"
	"ssh-tunnel-manager/internal/tunnel"
)

type traySampler struct {
	language string
	at       time.Time
	previous map[string]daemon.TunnelView
}

func (s *traySampler) sample(views []daemon.TunnelView, now time.Time) platform.TraySnapshot {
	result := platform.TraySnapshot{Language: s.language, Labels: platform.MenuLabels(s.language, applicationTitle), Lines: make([]platform.TrayLine, 0, len(views))}
	tr := func(zh, en string) string { return trayText(s.language, zh, en) }
	next := make(map[string]daemon.TunnelView, len(views))
	counts := map[string]int{}
	var totalIn, totalOut float64
	var connections int64
	allSampled := true
	seconds := now.Sub(s.at).Seconds()
	for _, view := range views {
		next[view.Name] = view
		state := trayState(view.State)
		counts[state]++
		connections += int64(view.ActiveConns)
		var inRate, outRate float64
		sampled := state != "running"
		previous, exists := s.previous[view.Name]
		// A restarted instance or reset counter must establish a new baseline.
		if state == "running" && exists && seconds > 0 && view.StartedAt.Equal(previous.StartedAt) && previous.State == "running" &&
			view.BytesIn >= previous.BytesIn && view.BytesOut >= previous.BytesOut {
			inRate = float64(view.BytesIn-previous.BytesIn) / seconds
			outRate = float64(view.BytesOut-previous.BytesOut) / seconds
			sampled = true
		}
		allSampled = allSampled && sampled
		totalIn += inRate
		totalOut += outRate
		rates := trayRates(inRate, outRate, sampled, s.language)
		line := platform.TrayLine{
			ShowHistory: view.Direction == "dynamic",
			RateText:    rates,
			ServiceIcon: (tunnel.Config{ServiceIcon: view.ServiceIcon, Direction: tunnel.Direction(view.Direction), ForwardAddress: view.ForwardAddress}).ResolvedServiceIcon(),
			Enabled:     view.Enabled,
			Name:        view.Name,
			State:       state,
			Title:       fmt.Sprintf("%s   %s", trayShort(view.Name, 28), rates),
			Details: []string{
				fmt.Sprintf(tr("累计：↓ %s  ↑ %s", "Total: ↓ %s  ↑ %s"), trayBytes(float64(view.BytesIn)), trayBytes(float64(view.BytesOut))),
				fmt.Sprintf(tr("当前连接：%d", "Connections: %d"), view.ActiveConns),
			},
		}
		switch view.Direction {
		case "remote":
			line.Details = append(line.Details, tr("远程监听：", "Remote: ")+view.RemoteListen, tr("SSH 服务：", "SSH server: ")+view.SSHAddress, tr("本机目标：", "Local target: ")+view.ForwardAddress)
		case "dynamic":
			if state != "stopped" {
				line.ProxyURL, line.ProxyCommand = trayProxy(view.LocalListen)
			}
			line.Details = append(line.Details, tr("SOCKS5 监听：", "SOCKS5: ")+view.LocalListen, tr("SSH 跳板：", "SSH gateway: ")+view.SSHAddress, tr("目标：由客户端指定", "Target: Specified by client"))
		default:
			line.Details = append(line.Details, tr("本地监听：", "Local: ")+view.LocalListen, tr("SSH 跳板：", "SSH gateway: ")+view.SSHAddress, tr("目标服务：", "Target: ")+view.ForwardAddress)
		}
		if view.LastError != "" {
			line.Details = append(line.Details, tr("最近错误：", "Last error: ")+view.LastError)
		}
		result.Lines = append(result.Lines, line)
	}
	result.Summary = traySummary(counts, s.language)
	result.Traffic = fmt.Sprintf(tr("合计 %s · %d 连接", "Total %s · %d connections"), trayRates(totalIn, totalOut, allSampled, s.language), connections)
	result.StatusText = "--\n--"
	if allSampled {
		result.StatusText = fmt.Sprintf("%s/s\n%s/s", trayBytes(totalOut), trayBytes(totalIn))
	}
	s.at, s.previous = now, next
	return result
}

func trayText(language, zh, en string) string {
	if language == "en" {
		return en
	}
	return zh
}

func traySummary(counts map[string]int, language string) string {
	parts := make([]string, 0, 4)
	for _, item := range []struct {
		state   string
		label   string
		english string
	}{{"running", "运行", "Running"}, {"starting", "连接中", "Connecting"}, {"error", "异常", "Error"}, {"stopped", "停止", "Stopped"}} {
		if count := counts[item.state]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", trayText(language, item.label, item.english), count))
		}
	}
	if len(parts) == 0 {
		return trayText(language, "暂无线路", "No tunnels")
	}
	return strings.Join(parts, " · ")
}

func trayState(state string) string {
	switch state {
	case "running", "starting", "error":
		return state
	default:
		return "stopped"
	}
}

func trayRates(in, out float64, sampled bool, language string) string {
	if !sampled {
		return trayText(language, "速率采样中", "Sampling rates")
	}
	return fmt.Sprintf("↓ %s/s  ↑ %s/s", trayBytes(in), trayBytes(out))
}

func trayBytes(value float64) string {
	if value < 0 {
		value = 0
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	i := 0
	for value >= 1024 && i < len(units)-1 {
		value /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", value, units[i])
	}
	return fmt.Sprintf("%.1f %s", value, units[i])
}

func trayShort(value string, limit int) string {
	runes := []rune(strings.Join(strings.Fields(value), " "))
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return string(runes)
}
