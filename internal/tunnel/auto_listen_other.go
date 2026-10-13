//go:build !windows

package tunnel

import (
	"errors"
	"syscall"
)

func isUnavailableProxyPort(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
