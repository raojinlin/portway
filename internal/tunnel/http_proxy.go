package tunnel

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// dialHTTPTarget handles one HTTP proxy request over the shared SSH transport.
// Non-CONNECT requests use a single upstream request with Connection: close.
func (i *Instance) dialHTTPTarget(dial func(string, string) (net.Conn, error), src net.Conn, setTarget func(string)) (net.Conn, error) {
	reader := bufio.NewReader(src)
	request, err := http.ReadRequest(reader)
	if err != nil {
		writeHTTPProxyError(src, http.StatusBadRequest)
		return nil, fmt.Errorf("http proxy request: %w", err)
	}
	defer request.Body.Close()

	target := request.URL.Host
	if request.Method == http.MethodConnect {
		target = request.Host
	} else if request.URL.Scheme != "http" || target == "" {
		writeHTTPProxyError(src, http.StatusBadRequest)
		return nil, fmt.Errorf("http proxy requires an absolute http URL")
	}
	if !strings.Contains(target, ":") {
		port := "80"
		if request.Method == http.MethodConnect {
			port = "443"
		}
		target = net.JoinHostPort(target, port)
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil || host == "" || port == "" {
		writeHTTPProxyError(src, http.StatusBadRequest)
		return nil, fmt.Errorf("invalid http proxy target %q", target)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		writeHTTPProxyError(src, http.StatusBadRequest)
		return nil, fmt.Errorf("invalid http proxy port %q", port)
	}
	setTarget(target)
	dst, err := dial("tcp", target)
	if err != nil {
		writeHTTPProxyError(src, http.StatusBadGateway)
		return nil, fmt.Errorf("http proxy dial %s: %w", target, err)
	}
	if request.Method == http.MethodConnect {
		if _, err := src.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			dst.Close()
			return nil, fmt.Errorf("http proxy connect reply: %w", err)
		}
		if _, err := io.CopyN(dst, reader, int64(reader.Buffered())); err != nil {
			dst.Close()
			return nil, fmt.Errorf("http proxy buffered connect data: %w", err)
		}
		return dst, nil
	}

	request.Header.Del("Proxy-Authorization")
	request.Header.Del("Proxy-Connection")
	request.Close = true
	if request.Header.Get("Expect") == "100-continue" {
		_, _ = src.Write([]byte("HTTP/1.1 100 Continue\r\n\r\n"))
		request.Header.Del("Expect")
	}
	if err := request.Write(dst); err != nil {
		dst.Close()
		return nil, fmt.Errorf("http proxy forward request: %w", err)
	}
	return dst, nil
}

func writeHTTPProxyError(conn net.Conn, status int) {
	_, _ = fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\nConnection: close\r\n\r\n", status, http.StatusText(status))
}
