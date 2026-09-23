package tunnel

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/kevinburke/ssh_config"
)

// SSHConfig holds resolved SSH connection parameters.
type SSHConfig struct {
	Host              string
	Port              string
	User              string
	Identity          string
	Identities        []string
	IdentityAgent     string
	IdentitiesOnly    bool
	ProxyCommand      string
	ProxyJump         string
	HostKeyAlgorithms string
}

// splitSSHAddress separates an optional explicit port before matching Host
// aliases. A bare IPv6 address is a host; a port on IPv6 requires brackets.
func splitSSHAddress(address string) (host, port string, err error) {
	if host, port, err = net.SplitHostPort(address); err == nil {
		if host == "" || port == "" {
			return "", "", fmt.Errorf("invalid ssh address %q: host and port must not be empty", address)
		}
		return host, port, nil
	}
	host = strings.TrimPrefix(strings.TrimSuffix(address, "]"), "[")
	if _, parseErr := netip.ParseAddr(host); parseErr == nil {
		return host, "", nil
	}
	if address == "" || strings.ContainsAny(address, ":[]") {
		return "", "", fmt.Errorf("invalid ssh address %q: use host, host:port, or [IPv6]:port", address)
	}
	return address, "", nil
}

// LoadSSHConfig resolves host settings from ssh config file.
// host is the name passed by user (may map to HostName in config).
// cfgPath can be empty to use default ~/.ssh/config.
func LoadSSHConfig(host, cfgPath string) (SSHConfig, error) {
	if host == "" {
		return SSHConfig{}, fmt.Errorf("host required")
	}
	if cfgPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return SSHConfig{}, err
		}
		cfgPath = filepath.Join(home, ".ssh", "config")
	} else {
		var err error
		cfgPath, err = expandHome(cfgPath)
		if err != nil {
			return SSHConfig{}, err
		}
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return SSHConfig{}, err
	}
	cfg, err := ssh_config.Decode(strings.NewReader(string(data)))
	if err != nil {
		return SSHConfig{}, err
	}

	hostname, _ := cfg.Get(host, "HostName")
	if hostname == "" {
		hostname = host
	}
	port, _ := cfg.Get(host, "Port")
	user, _ := cfg.Get(host, "User")
	identity, _ := cfg.Get(host, "IdentityFile")
	identities, _ := cfg.GetAll(host, "IdentityFile")
	identityAgent, _ := cfg.Get(host, "IdentityAgent")
	identitiesOnly, _ := cfg.Get(host, "IdentitiesOnly")
	proxyCommand, _ := cfg.Get(host, "ProxyCommand")
	proxyJump, _ := cfg.Get(host, "ProxyJump")
	hostKeyAlgorithms, _ := cfg.Get(host, "HostKeyAlgorithms")

	return SSHConfig{Host: hostname, Port: port, User: user, Identity: identity, Identities: identities,
		IdentityAgent: identityAgent, IdentitiesOnly: strings.EqualFold(identitiesOnly, "yes"),
		ProxyCommand: proxyCommand, ProxyJump: proxyJump, HostKeyAlgorithms: hostKeyAlgorithms}, nil
}

// lookupUserKnownHostsFile returns the UserKnownHostsFile entry for host from
// the ssh config, if any. It returns "" if unavailable rather than erroring,
// since callers treat it as an optional hint.
func lookupUserKnownHostsFile(host, cfgPath string) string {
	if host == "" {
		return ""
	}
	if cfgPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		cfgPath = filepath.Join(home, ".ssh", "config")
	} else {
		var err error
		cfgPath, err = expandHome(cfgPath)
		if err != nil {
			return ""
		}
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return ""
	}
	cfg, err := ssh_config.Decode(strings.NewReader(string(data)))
	if err != nil {
		return ""
	}
	known, _ := cfg.Get(host, "UserKnownHostsFile")
	// ssh_config may return a space-separated list; take the first entry.
	if idx := strings.IndexByte(known, ' '); idx >= 0 {
		known = known[:idx]
	}
	return known
}

// FileExists checks existence for config path override.
func FileExists(path string) bool {
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false
		}
	}
	return true
}
