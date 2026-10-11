package tunnel

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPProxyConnectPreservesBufferedPayload(t *testing.T) {
	src, client := net.Pipe()
	defer src.Close()
	defer client.Close()
	upstream, target := net.Pipe()
	defer upstream.Close()
	defer target.Close()
	result := make(chan error, 1)
	var requested string
	go func() {
		_, err := (&Instance{}).dialHTTPTarget(func(_, address string) (net.Conn, error) {
			if address != "example.com:443" {
				return nil, errors.New("wrong target")
			}
			return upstream, nil
		}, src, func(address string) { requested = address })
		result <- err
	}()
	client.SetDeadline(time.Now().Add(2 * time.Second))
	target.SetDeadline(time.Now().Add(2 * time.Second))
	go func() {
		_, _ = io.WriteString(client, "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\nhello")
	}()
	line, err := bufio.NewReader(client).ReadString('\n')
	if err != nil || !strings.Contains(line, "200 Connection Established") {
		t.Fatalf("CONNECT response = %q, %v", line, err)
	}
	data := make([]byte, 5)
	if _, err := io.ReadFull(target, data); err != nil || string(data) != "hello" {
		t.Fatalf("upstream data = %q, %v", data, err)
	}
	if err := <-result; err != nil || requested != "example.com:443" {
		t.Fatalf("dial result = %v, target = %q", err, requested)
	}
}

func TestHTTPProxyForwardsAbsoluteURLWithoutProxyCredentials(t *testing.T) {
	src, client := net.Pipe()
	defer src.Close()
	defer client.Close()
	upstream, target := net.Pipe()
	defer upstream.Close()
	defer target.Close()
	result := make(chan error, 1)
	go func() {
		_, err := (&Instance{}).dialHTTPTarget(func(_, address string) (net.Conn, error) {
			if address != "example.com:80" {
				return nil, errors.New("wrong target")
			}
			return upstream, nil
		}, src, func(string) {})
		result <- err
	}()
	client.SetDeadline(time.Now().Add(2 * time.Second))
	target.SetDeadline(time.Now().Add(2 * time.Second))
	go func() {
		_, _ = io.WriteString(client, "GET http://example.com/path?q=1 HTTP/1.1\r\nHost: example.com\r\nProxy-Authorization: Basic secret\r\n\r\n")
	}()
	request, err := http.ReadRequest(bufio.NewReader(target))
	if err != nil {
		t.Fatal(err)
	}
	if request.RequestURI != "/path?q=1" || request.Header.Get("Proxy-Authorization") != "" || !request.Close {
		t.Fatalf("forwarded request = %+v", request)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestHTTPListenValidation(t *testing.T) {
	cfg := Config{Name: "proxy", Direction: DirectionDynamic, LocalListen: "127.0.0.1:1080", SSHAddress: "host", HTTPListen: "127.0.0.1:8080"}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.HTTPListen = cfg.LocalListen
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("same HTTP and SOCKS5 listen address accepted")
	}
	cfg.Direction = DirectionLocal
	cfg.ForwardAddress = "example.com:80"
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("HTTP listen accepted for local forwarding")
	}
}
