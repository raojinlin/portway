package tunnel

import (
	"encoding/binary"
	"net"
	"testing"
)

func pipeConn(t *testing.T) (client, server net.Conn) {
	t.Helper()
	client, server = net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	return client, server
}

// runClient plays the SOCKS5 client side of the handshake against server
// (via the paired pipe end "client"): it drains any bytes the server writes
// back in a background goroutine (net.Pipe is unbuffered/synchronous, so
// without a reader any reply write on the server side would block forever),
// then writes the greeting and, if given, the request.
func runClient(t *testing.T, conn net.Conn, methods []byte, request []byte) {
	t.Helper()
	go func() {
		buf := make([]byte, 256)
		for {
			if _, err := conn.Read(buf); err != nil {
				return
			}
		}
	}()

	greeting := append([]byte{socks5Version, byte(len(methods))}, methods...)
	if _, err := conn.Write(greeting); err != nil {
		return
	}
	if len(request) > 0 {
		conn.Write(request)
	}
}

func TestSocks5ReadRequest_IPv4(t *testing.T) {
	client, server := pipeConn(t)
	req := []byte{socks5Version, socks5CmdConnect, 0x00, socks5AtypIPv4, 10, 0, 0, 1, 0x01, 0xBB}
	go runClient(t, client, []byte{socks5AuthNone}, req)

	target, err := socks5ReadRequest(server)
	if err != nil {
		t.Fatalf("socks5ReadRequest: %v", err)
	}
	if target != "10.0.0.1:443" {
		t.Fatalf("target = %q, want 10.0.0.1:443", target)
	}
}

func TestSocks5ReadRequest_Domain(t *testing.T) {
	client, server := pipeConn(t)
	domain := "example.com"
	req := []byte{socks5Version, socks5CmdConnect, 0x00, socks5AtypDomain, byte(len(domain))}
	req = append(req, []byte(domain)...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, 8080)
	req = append(req, portBytes...)
	go runClient(t, client, []byte{socks5AuthNone}, req)

	target, err := socks5ReadRequest(server)
	if err != nil {
		t.Fatalf("socks5ReadRequest: %v", err)
	}
	if target != "example.com:8080" {
		t.Fatalf("target = %q, want example.com:8080", target)
	}
}

func TestSocks5ReadRequest_IPv6(t *testing.T) {
	client, server := pipeConn(t)
	ip := net.ParseIP("2001:db8::1").To16()
	req := []byte{socks5Version, socks5CmdConnect, 0x00, socks5AtypIPv6}
	req = append(req, ip...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, 22)
	req = append(req, portBytes...)
	go runClient(t, client, []byte{socks5AuthNone}, req)

	target, err := socks5ReadRequest(server)
	if err != nil {
		t.Fatalf("socks5ReadRequest: %v", err)
	}
	host, port, err := net.SplitHostPort(target)
	if err != nil || host != "2001:db8::1" || port != "22" {
		t.Fatalf("target = %q, want host 2001:db8::1 port 22", target)
	}
}

func TestSocks5ReadRequest_RejectsNonConnect(t *testing.T) {
	client, server := pipeConn(t)
	// CMD 0x02 = BIND, unsupported; only CONNECT is implemented.
	req := []byte{socks5Version, 0x02, 0x00, socks5AtypIPv4, 127, 0, 0, 1, 0x00, 0x50}
	go runClient(t, client, []byte{socks5AuthNone}, req)

	if _, err := socks5ReadRequest(server); err == nil {
		t.Fatalf("expected error for unsupported BIND command")
	}
}

func TestSocks5Negotiate_RejectsUnsupportedVersion(t *testing.T) {
	client, server := pipeConn(t)
	go func() {
		client.Write([]byte{0x04, 0x01, socks5AuthNone}) // SOCKS4 greeting, not SOCKS5
	}()

	if _, err := socks5ReadRequest(server); err == nil {
		t.Fatalf("expected error for unsupported socks version")
	}
}

func TestSocks5Negotiate_RejectsWhenNoAcceptableMethod(t *testing.T) {
	client, server := pipeConn(t)
	// Offer only username/password auth (0x02); we only accept no-auth (0x00).
	go runClient(t, client, []byte{0x02}, nil)

	if _, err := socks5ReadRequest(server); err == nil {
		t.Fatalf("expected error when no acceptable auth method is offered")
	}
}
