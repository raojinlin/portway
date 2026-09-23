package main

import (
	"fmt"
	"strings"
	"time"

	"ssh-tunnel-manager/desktop/platform"
	"ssh-tunnel-manager/internal/daemon"
)

type traySampler struct {
	at       time.Time
	previous map[string]daemon.TunnelView
}

func (s *traySampler) sample(views []daemon.TunnelView, now time.Time) platform.TraySnapshot {
	result := platform.TraySnapshot{Lines: make([]platform.TrayLine, 0, len(views))}
	next := make(map[string]daemon.TunnelView, len(views))
	counts := map[string]int{}
	var totalIn, totalOut float64
	var connections int64
	allSampled := true
	seconds := now.Sub(s.at).Seconds()
	for _, view := range views {
		next[view.Name] = view
		state, label := trayState(view.State)
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
		rates := trayRates(inRate, outRate, sampled)
		line := platform.TrayLine{
			Name:  view.Name,
			Title: fmt.Sprintf("%s · %s   %s", trayShort(view.Name, 28), label, rates),
			Details: []string{
				"线路：" + view.Name,
				"状态：" + label,
				"速率：" + rates,
				fmt.Sprintf("累计：↓ %s  ↑ %s", trayBytes(float64(view.BytesIn)), trayBytes(float64(view.BytesOut))),
				fmt.Sprintf("当前连接：%d", view.ActiveConns),
			},
		}
		switch view.Direction {
		case "remote":
			line.Details = append(line.Details, "远程监听："+view.RemoteListen, "SSH 服务："+view.SSHAddress, "本机目标："+view.ForwardAddress)
		case "dynamic":
			line.Details = append(line.Details, "SOCKS5 监听："+view.LocalListen, "SSH 跳板："+view.SSHAddress, "目标：由客户端指定")
		default:
			line.Details = append(line.Details, "本地监听："+view.LocalListen, "SSH 跳板："+view.SSHAddress, "目标服务："+view.ForwardAddress)
		}
		if view.LastError != "" {
			line.Details = append(line.Details, "最近错误："+view.LastError)
		}
		line.Details = append(line.Details, "速率为采样间隔平均值；累计流量在线路重启后清零")
		result.Lines = append(result.Lines, line)
	}
	result.Summary = fmt.Sprintf("运行 %d · 连接中 %d · 异常 %d · 停止 %d", counts["running"], counts["starting"], counts["error"], counts["stopped"])
	result.Traffic = fmt.Sprintf("合计 %s · %d 连接", trayRates(totalIn, totalOut, allSampled), connections)
	s.at, s.previous = now, next
	return result
}

func trayState(state string) (string, string) {
	switch state {
	case "running":
		return state, "运行中"
	case "starting":
		return state, "连接中"
	case "error":
		return state, "异常"
	case "stopping":
		return "stopped", "停止中"
	default:
		return "stopped", "已停止"
	}
}

func trayRates(in, out float64, sampled bool) string {
	if !sampled {
		return "速率采样中"
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
