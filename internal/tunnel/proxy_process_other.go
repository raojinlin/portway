//go:build !unix

package tunnel

import "os/exec"

func configureProxyProcess(cmd *exec.Cmd) {}
