package tunnel

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
)

// Minimal SOCKS5 server (RFC 1928) used by dynamic (-D style) forwarding.
// Only "no authentication" and the CONNECT command are supported, which
// covers every mainstream SOCKS5 client (browsers, curl --socks5, etc.)
// used against a tunnel like this.
const (
	socks5Version = 0x05

	socks5CmdConnect = 0x01

	socks5AtypIPv4   = 0x01
	socks5AtypDomain = 0x03
	socks5AtypIPv6   = 0x04

	socks5AuthNone         = 0x00
	socks5AuthNoAcceptable = 0xFF

	socks5RepSucceeded           = 0x00
	socks5RepGeneralFailure      = 0x01
	socks5RepCommandNotSupported = 0x07
	socks5RepAddrNotSupported    = 0x08
)

// socks5ReadRequest negotiates the SOCKS5 handshake on conn (accepting only
// "no authentication") and reads a CONNECT request, returning the
// "host:port" the client wants to reach.
func socks5ReadRequest(conn net.Conn) (string, error) {
	if err := socks5Negotiate(conn); err != nil {
		return "", err
	}
	return socks5ReadConnectRequest(conn)
}

func socks5Negotiate(conn net.Conn) error {
	head := make([]byte, 2)
	if _, err := io.ReadFull(conn, head); err != nil {
		return fmt.Errorf("read greeting: %w", err)
	}
	if head[0] != socks5Version {
		return fmt.Errorf("unsupported socks version %d", head[0])
	}
	methods := make([]byte, head[1])
	if _, err := io.ReadFull(conn, methods); err != nil {
		return fmt.Errorf("read methods: %w", err)
	}
	for _, m := range methods {
		if m == socks5AuthNone {
			_, err := conn.Write([]byte{socks5Version, socks5AuthNone})
			return err
		}
	}
	_, _ = conn.Write([]byte{socks5Version, socks5AuthNoAcceptable})
	return fmt.Errorf("client offered no acceptable auth method")
}

func socks5ReadConnectRequest(conn net.Conn) (string, error) {
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return "", fmt.Errorf("read request header: %w", err)
	}
	if head[0] != socks5Version {
		return "", fmt.Errorf("unsupported socks version %d", head[0])
	}
	if head[1] != socks5CmdConnect {
		_ = socks5WriteReply(conn, socks5RepCommandNotSupported, "")
		return "", fmt.Errorf("unsupported socks command %d (only CONNECT is supported)", head[1])
	}

	var host string
	switch head[3] {
	case socks5AtypIPv4:
		addr := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", fmt.Errorf("read ipv4 address: %w", err)
		}
		host = net.IP(addr).String()
	case socks5AtypDomain:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return "", fmt.Errorf("read domain length: %w", err)
		}
		domain := make([]byte, lenBuf[0])
		if _, err := io.ReadFull(conn, domain); err != nil {
			return "", fmt.Errorf("read domain: %w", err)
		}
		host = string(domain)
	case socks5AtypIPv6:
		addr := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return "", fmt.Errorf("read ipv6 address: %w", err)
		}
		host = net.IP(addr).String()
	default:
		_ = socks5WriteReply(conn, socks5RepAddrNotSupported, "")
		return "", fmt.Errorf("unsupported socks address type %d", head[3])
	}

	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBuf); err != nil {
		return "", fmt.Errorf("read port: %w", err)
	}
	port := binary.BigEndian.Uint16(portBuf)

	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

// socks5WriteReply writes a SOCKS5 reply. bindAddr, if non-empty and
// parseable, is echoed back as the bound address; callers without a
// meaningful bind address (e.g. reporting a failure) may pass "".
func socks5WriteReply(conn net.Conn, rep byte, bindAddr string) error {
	host, port := "0.0.0.0", 0
	if bindAddr != "" {
		if h, p, err := net.SplitHostPort(bindAddr); err == nil {
			host = h
			if v, err := strconv.Atoi(p); err == nil {
				port = v
			}
		}
	}

	atyp := byte(socks5AtypIPv4)
	addrBytes := net.IPv4zero.To4()
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			atyp, addrBytes = socks5AtypIPv4, ip4
		} else {
			atyp, addrBytes = socks5AtypIPv6, ip.To16()
		}
	}

	reply := make([]byte, 0, 6+len(addrBytes))
	reply = append(reply, socks5Version, rep, 0x00, atyp)
	reply = append(reply, addrBytes...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	reply = append(reply, portBytes...)

	_, err := conn.Write(reply)
	return err
}

// socks5ReplyForDialError maps a failed dial of the client's requested
// target to a REP code. The SOCKS5 RFC has more specific codes (network
// unreachable, TTL expired, ...) but standard Go errors don't reliably
// distinguish them, so a generic failure code is reported.
func socks5ReplyForDialError(err error) byte {
	return socks5RepGeneralFailure
}
