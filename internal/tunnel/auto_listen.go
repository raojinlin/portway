package tunnel

import (
	"fmt"
	"net"
	"strconv"
)

func listenProxy(address string, firstPort int) (net.Listener, error) {
	if address != "auto" {
		return net.Listen("tcp", address)
	}
	for port := firstPort; port <= 65535; port++ {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err == nil {
			return ln, nil
		}
		if !isUnavailableProxyPort(err) {
			return nil, fmt.Errorf("listen on proxy port %d: %w", port, err)
		}
	}
	return nil, fmt.Errorf("no available proxy port from %d to 65535", firstPort)
}
