package tunnel

import (
	"net"
	"strconv"
)

func ValidServiceIcon(icon string) bool {
	switch icon {
	case "", "auto", "generic", "ssh", "mysql", "postgresql", "http", "https", "redis", "mongodb", "rdp", "socks5":
		return true
	}
	return false
}

// ServiceIcon identifies the forwarded service, not the SSH transport or listen port.
func (c Config) ResolvedServiceIcon() string {
	if c.ServiceIcon != "" && c.ServiceIcon != "auto" && ValidServiceIcon(c.ServiceIcon) {
		return c.ServiceIcon
	}
	if c.Direction == DirectionDynamic {
		return "socks5"
	}
	_, port, err := net.SplitHostPort(c.ForwardAddress)
	if err != nil {
		return "generic"
	}
	n, _ := strconv.Atoi(port)
	switch n {
	case 22:
		return "ssh"
	case 3306:
		return "mysql"
	case 5432:
		return "postgresql"
	case 80, 8080, 8000, 3000:
		return "http"
	case 443, 8443:
		return "https"
	case 6379:
		return "redis"
	case 27017:
		return "mongodb"
	case 3389:
		return "rdp"
	default:
		return "generic"
	}
}
