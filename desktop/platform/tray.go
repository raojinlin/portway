package platform

import "runtime"

func HasTray() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "windows" }

type TrayActions struct {
	Show       func()
	Quit       func()
	Directory  func()
	Logs       func()
	Copy       func(string)
	SetEnabled func(string, bool)
}

type TunnelAction struct {
	Name    string
	Enabled bool
}

type TraySnapshot struct {
	Language   string            `json:"language"`
	Labels     map[string]string `json:"labels"`
	Summary    string            `json:"summary"`
	Traffic    string            `json:"traffic"`
	StatusText string            `json:"statusText"`
	Lines      []TrayLine        `json:"lines"`
}

func MenuLabels(language, title string) map[string]string {
	if language == "en" {
		return map[string]string{"open": "Open " + title, "directory": "Open Configuration Folder", "quit": "Quit " + title,
			"logs": "Open Logs", "copyProxy": "Copy Proxy URL", "copyCommand": "Copy Proxy Command",
			"startTunnel": "Start Tunnel", "stopTunnel": "Stop Tunnel", "working": "Working...",
			"about": "About " + title, "hide": "Hide " + title, "hideOthers": "Hide Others", "showAll": "Show All",
			"edit": "Edit", "undo": "Undo", "redo": "Redo", "cut": "Cut", "copy": "Copy", "paste": "Paste",
			"pasteStyle": "Paste and Match Style", "delete": "Delete", "selectAll": "Select All",
			"speech": "Speech", "speak": "Start Speaking", "stopSpeaking": "Stop Speaking", "window": "Window",
			"minimize": "Minimize", "zoom": "Zoom", "fullscreen": "Full Screen", "ok": "OK", "version": "Version",
			"aboutDescription": "SSH tunnel manager for local, remote and SOCKS5 forwarding.",
			"empty":            "No tunnels", "removed": "Removed", "upload": "Upload", "download": "Download"}
	}
	return map[string]string{"open": "打开 " + title, "directory": "打开配置目录", "quit": "退出 " + title,
		"logs": "打开日志", "copyProxy": "复制代理地址", "copyCommand": "复制代理命令",
		"startTunnel": "启动线路", "stopTunnel": "停止线路", "working": "处理中…",
		"about": "关于 " + title, "hide": "隐藏 " + title, "hideOthers": "隐藏其他", "showAll": "显示全部",
		"edit": "编辑", "undo": "撤销", "redo": "重做", "cut": "剪切", "copy": "复制", "paste": "粘贴",
		"pasteStyle": "粘贴并匹配样式", "delete": "删除", "selectAll": "全选",
		"speech": "语音", "speak": "开始朗读", "stopSpeaking": "停止朗读", "window": "窗口",
		"minimize": "最小化", "zoom": "缩放", "fullscreen": "全屏", "ok": "好", "version": "版本",
		"aboutDescription": "管理本地转发、远程转发和 SOCKS5 代理的 SSH 隧道工具。",
		"empty":            "暂无线路", "removed": "已移除", "upload": "上传", "download": "下载"}
}

type TrayLine struct {
	Enabled      bool     `json:"enabled"`
	Busy         bool     `json:"busy"`
	ProxyURL     string   `json:"proxyURL,omitempty"`
	ProxyCommand string   `json:"proxyCommand,omitempty"`
	Name         string   `json:"name"`
	State        string   `json:"state"`
	Title        string   `json:"title"`
	Details      []string `json:"details"`
}
