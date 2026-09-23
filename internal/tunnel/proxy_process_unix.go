//go:build unix

package tunnel

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureProxyProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Kill the shell and its ssh child on timeout, stop, or reconnect.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}
