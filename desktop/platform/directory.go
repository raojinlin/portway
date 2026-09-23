package platform

import (
	"os/exec"
	"runtime"
)

func OpenDirectory(path string) {
	var command string
	switch runtime.GOOS {
	case "darwin":
		command = "/usr/bin/open"
	case "windows":
		command = "explorer.exe"
	default:
		command = "xdg-open"
	}
	// Run asynchronously, but always reap the short-lived launcher process.
	go func() { _ = exec.Command(command, path).Run() }()
}
