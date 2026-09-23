package platform

import "runtime"

func HasTray() bool { return runtime.GOOS == "darwin" || runtime.GOOS == "windows" }

type TraySnapshot struct {
	Summary string     `json:"summary"`
	Traffic string     `json:"traffic"`
	Lines   []TrayLine `json:"lines"`
}

type TrayLine struct {
	Name    string   `json:"name"`
	Title   string   `json:"title"`
	Details []string `json:"details"`
}
