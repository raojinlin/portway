//go:build windows

package tunnel

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isUnavailableProxyPort(err error) bool {
	return errors.Is(err, windows.WSAEADDRINUSE) || errors.Is(err, windows.WSAEACCES)
}
