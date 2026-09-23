package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"ssh-tunnel-manager/internal/logging"

	"golang.org/x/crypto/ssh"
)

type sshEndpoint struct {
	config               Config
	settings             SSHConfig
	alias, address, user string
}

func resolveEndpoint(cfg Config) (sshEndpoint, error) {
	alias, _, err := splitSSHAddress(cfg.SSHAddress)
	if err != nil {
		return sshEndpoint{}, err
	}
	settings, err := LoadSSHConfig(alias, cfg.SSHConfigPath)
	if err != nil && !(cfg.SSHConfigPath == "" && errors.Is(err, os.ErrNotExist)) {
		return sshEndpoint{}, fmt.Errorf("load ssh config for %s: %w", alias, err)
	}
	address, user, _, err := resolveSSHSettingsWithConfig(cfg, settings)
	return sshEndpoint{cfg, settings, alias, address, user}, err
}

func resolveSSHRoute(cfg Config, stack map[string]bool) ([]sshEndpoint, error) {
	if len(stack) >= 16 {
		return nil, errors.New("SSH jump chain exceeds 16 hosts")
	}
	endpoint, err := resolveEndpoint(cfg)
	if err != nil {
		return nil, err
	}
	key := endpoint.user + "@" + endpoint.address
	if stack[key] {
		return nil, fmt.Errorf("SSH jump cycle at %s (%s)", endpoint.alias, key)
	}
	stack[key] = true
	defer delete(stack, key)
	jump := endpoint.settings.ProxyJump
	if endpoint.settings.ProxyCommand != "" {
		var native bool
		jump, native = nativeProxyJump(endpoint.settings.ProxyCommand)
		if !native {
			jump = ""
		}
	}
	if jump == "" || jump == "none" {
		return []sshEndpoint{endpoint}, nil
	}
	destinations := strings.Split(jump, ",")
	if len(destinations) >= 16 {
		return nil, errors.New("SSH jump chain exceeds 16 hosts")
	}
	var route []sshEndpoint
	for n, destination := range destinations {
		hop, err := jumpConfig(strings.TrimSpace(destination), cfg.SSHConfigPath)
		if err != nil {
			return nil, fmt.Errorf("ProxyJump for %s: %w", endpoint.alias, err)
		}
		if n == 0 {
			// The first jump may itself use a configured jump chain.
			route, err = resolveSSHRoute(hop, stack)
		} else {
			var next sshEndpoint
			next, err = resolveEndpoint(hop)
			// Explicit comma-separated jumps determine the route after the first hop.
			next.settings.ProxyCommand, next.settings.ProxyJump = "", ""
			route = append(route, next)
		}
		if err != nil {
			return nil, err
		}
	}
	endpoint.settings.ProxyCommand, endpoint.settings.ProxyJump = "", ""
	route = append(route, endpoint)
	seen := make(map[string]bool)
	for _, hop := range route {
		key := hop.user + "@" + hop.address
		if seen[key] {
			return nil, fmt.Errorf("SSH jump cycle at %s (%s)", hop.alias, key)
		}
		seen[key] = true
	}
	if len(route) > 16 {
		return nil, errors.New("SSH jump chain exceeds 16 hosts")
	}
	return route, nil
}

func jumpConfig(destination, configPath string) (Config, error) {
	// Never inherit target credentials or host-key bypass flags into a jump.
	cfg := Config{SSHConfigPath: configPath}
	if at := strings.LastIndexByte(destination, '@'); at >= 0 {
		cfg.SSHUser, destination = destination[:at], destination[at+1:]
		if cfg.SSHUser == "" || strings.Contains(cfg.SSHUser, "@") {
			return cfg, errors.New("invalid jump username")
		}
	}
	if destination == "" || strings.ContainsAny(destination, " /\\\t\r\n") || strings.HasPrefix(destination, "-") {
		return cfg, fmt.Errorf("invalid jump destination %q", destination)
	}
	if _, _, err := splitSSHAddress(destination); err != nil {
		return cfg, err
	}
	cfg.SSHAddress = destination
	return cfg, nil
}

// Convert only the unambiguous ssh -W template; never silently discard options,
// shell syntax, or a different forwarding destination in a custom ProxyCommand.
func nativeProxyJump(command string) (string, bool) {
	if strings.ContainsAny(command, "\r\n") {
		return "", false
	}
	words := strings.Fields(command)
	if len(words) < 4 || (words[0] != "ssh" && words[0] != "/usr/bin/ssh") {
		return "", false
	}
	var user, port, destination string
	forward := false
	for n := 1; n < len(words); n++ {
		switch words[n] {
		case "-q":
		case "-W", "-p", "-l":
			option := words[n]
			n++
			if n >= len(words) {
				return "", false
			}
			value := words[n]
			if option == "-W" {
				if forward {
					return "", false
				}
				if value != "%h:%p" && value != "[%h]:%p" && value != "'%h:%p'" && value != "\"%h:%p\"" {
					return "", false
				}
				forward = true
			} else if option == "-p" && port == "" {
				port = value
			} else if option == "-l" && user == "" {
				user = value
			} else {
				return "", false
			}
		default:
			if n != len(words)-1 || strings.HasPrefix(words[n], "-") {
				return "", false
			}
			destination = words[n]
		}
	}
	if !forward || destination == "" {
		return "", false
	}
	for _, value := range []string{destination, user, port} {
		for _, ch := range value {
			if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-:@[]", ch) {
				return "", false
			}
		}
	}
	cfg, err := jumpConfig(destination, "")
	if err != nil {
		return "", false
	}
	if user != "" {
		if cfg.SSHUser != "" {
			return "", false
		}
		cfg.SSHUser = user
	}
	if port != "" {
		host, _, err := splitSSHAddress(cfg.SSHAddress)
		if err != nil {
			return "", false
		}
		cfg.SSHAddress = net.JoinHostPort(host, port)
	}
	if cfg.SSHUser != "" {
		cfg.SSHAddress = cfg.SSHUser + "@" + cfg.SSHAddress
	}
	return cfg.SSHAddress, true
}

func dialNativeSSH(ctx context.Context, cfg Config, dial contextDialer) (*ssh.Client, error) {
	logger := logging.FromContext(ctx).With("tunnel", cfg.Name)
	route, err := resolveSSHRoute(cfg, make(map[string]bool))
	if err != nil {
		logger.Debug("SSH route resolution failed", "error", logging.SafeError(err, cfg.SSHPassword))
		return nil, err
	}
	logger.Debug("SSH route resolved", "hosts", len(route))
	var client *ssh.Client
	for index, endpoint := range route {
		hopLogger := logger.With("hop", index+1, "host", endpoint.alias, "address", endpoint.address, "user", endpoint.user)
		mode := "direct"
		if client != nil {
			mode = "native_jump"
		} else if endpoint.settings.ProxyCommand != "" && endpoint.settings.ProxyCommand != "none" {
			mode = "external_proxy"
		}
		hopLogger.Info("SSH hop connecting", "transport", mode)
		started := time.Now()
		next, err := dialSSHEndpoint(logging.WithLogger(ctx, hopLogger), endpoint, client, dial)
		if err != nil {
			hopLogger.Debug("SSH hop failed", "elapsed", time.Since(started), "error", logging.SafeError(err, cfg.SSHPassword))
			if client != nil {
				client.Close()
			}
			return nil, fmt.Errorf("SSH host %s (%s): %w", endpoint.alias, endpoint.address, err)
		}
		hopLogger.Info("SSH hop connected", "elapsed", time.Since(started))
		client = next
	}
	go func() {
		err := client.Wait()
		logger.Info("SSH connection closed", "error", logging.SafeError(err, cfg.SSHPassword))
	}()
	return client, nil
}

func dialSSHEndpoint(ctx context.Context, endpoint sshEndpoint, parent *ssh.Client, dial contextDialer) (*ssh.Client, error) {
	lifetimeCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	logger := logging.FromContext(ctx)
	auth, closeAuth, err := buildSSHAuth(ctx, endpoint.config, endpoint.settings)
	if err != nil {
		return nil, fmt.Errorf("authentication setup: %w", err)
	}
	defer closeAuth()
	hostKey, err := buildHostKeyCallback(endpoint.config, endpoint.alias)
	if err != nil {
		return nil, fmt.Errorf("host key verification: %w", err)
	}
	algorithms, err := hostKeyAlgorithms(endpoint.config, endpoint.alias, endpoint.address, endpoint.settings.HostKeyAlgorithms)
	if err != nil {
		return nil, fmt.Errorf("host key algorithms: %w", err)
	}
	logger.Debug("SSH host key algorithms selected", "algorithms", algorithms)
	if endpoint.config.InsecureSkipHostKeyCheck {
		logger.Warn("SSH host key verification disabled")
	} else {
		logger.Debug("SSH host key verification configured", "trust_new_host_key", endpoint.config.TrustNewHostKey)
	}
	verifiedHostKey := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if err := hostKey(hostname, remote, key); err != nil {
			return err
		}
		logger.Debug("SSH host key accepted", "key_type", key.Type(), "fingerprint", ssh.FingerprintSHA256(key), "verification_skipped", endpoint.config.InsecureSkipHostKeyCheck)
		return nil
	}
	config := &ssh.ClientConfig{User: endpoint.user, Auth: auth, HostKeyCallback: verifiedHostKey, HostKeyAlgorithms: algorithms, Timeout: 10 * time.Second}
	if parent == nil && endpoint.settings.ProxyCommand != "" && endpoint.settings.ProxyCommand != "none" {
		return dialSSHTransport(lifetimeCtx, endpoint.address, endpoint.alias, endpoint.settings, config)
	}
	var conn net.Conn
	logger.Debug("SSH transport dialing")
	if parent == nil {
		conn, err = dial(ctx, "tcp", endpoint.address)
	} else {
		stop := context.AfterFunc(ctx, func() { parent.Close() })
		conn, err = parent.DialContext(ctx, "tcp", endpoint.address)
		stop()
		if err == nil {
			conn = &jumpConn{Conn: conn, parent: parent, address: endpoint.address}
		}
	}
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	logger.Debug("SSH handshake starting")
	c, channels, requests, err := ssh.NewClientConn(conn, endpoint.address, config)
	stop()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	return ssh.NewClient(c, channels, requests), nil
}

// The final client's Close (including remote EOF) owns every preceding jump.
type jumpConn struct {
	net.Conn
	parent  *ssh.Client
	address string
	once    sync.Once
}

func (c *jumpConn) RemoteAddr() net.Addr { return proxyAddr(c.address) }
func (c *jumpConn) Close() error {
	c.once.Do(func() { c.parent.Close(); c.Conn.Close() })
	return nil
}
