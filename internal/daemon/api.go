package daemon

import (
	"fmt"
	"time"

	"ssh-tunnel-manager/internal/tunnel"
)

// TunnelRequest is the wire format for creating a tunnel. Durations are
// human-friendly strings (e.g. "30s") so both the CLI and the web UI can
// send plain text.
type TunnelRequest struct {
	Name                     string `json:"name"`
	Direction                string `json:"direction,omitempty"` // "local" (default), "remote", or "dynamic"
	LocalListen              string `json:"local_listen,omitempty"`
	SSHAddress               string `json:"ssh_address"`
	SSHUser                  string `json:"ssh_user,omitempty"`
	SSHKeyPath               string `json:"ssh_key_path,omitempty"`
	SSHPassword              string `json:"ssh_password,omitempty"`
	SSHConfigPath            string `json:"ssh_config_path,omitempty"`
	RemoteListen             string `json:"remote_listen,omitempty"` // server-side listen address; direction=remote only
	ForwardAddress           string `json:"forward_address,omitempty"`
	KnownHostsPath           string `json:"known_hosts_path,omitempty"`
	TrustNewHostKey          bool   `json:"trust_new_host_key,omitempty"`
	InsecureSkipHostKeyCheck bool   `json:"insecure_skip_host_key_check,omitempty"`
	KeepAlive                string `json:"keep_alive,omitempty"`
	ReconnectDelay           string `json:"reconnect_delay,omitempty"`
}

const (
	defaultKeepAlive      = 30 * time.Second
	defaultReconnectDelay = 3 * time.Second
)

// toConfig converts the request into a tunnel.Config and validates it via
// tunnel.ValidateConfig, so the required-field rules for each Direction
// live in one place shared with the in-process Start path.
func (r TunnelRequest) toConfig() (tunnel.Config, error) {
	direction := tunnel.Direction(r.Direction)
	switch direction {
	case "", tunnel.DirectionLocal, tunnel.DirectionRemote, tunnel.DirectionDynamic:
	default:
		return tunnel.Config{}, fmt.Errorf("invalid direction %q (must be local, remote, or dynamic)", r.Direction)
	}
	if r.SSHAddress == "" {
		return tunnel.Config{}, fmt.Errorf("ssh_address is required")
	}

	keepAlive := defaultKeepAlive
	if r.KeepAlive != "" {
		d, err := time.ParseDuration(r.KeepAlive)
		if err != nil {
			return tunnel.Config{}, fmt.Errorf("invalid keep_alive: %w", err)
		}
		keepAlive = d
	}
	reconnect := defaultReconnectDelay
	if r.ReconnectDelay != "" {
		d, err := time.ParseDuration(r.ReconnectDelay)
		if err != nil {
			return tunnel.Config{}, fmt.Errorf("invalid reconnect_delay: %w", err)
		}
		reconnect = d
	}

	cfg := tunnel.Config{
		Name:                     r.Name,
		Direction:                direction,
		LocalListen:              r.LocalListen,
		SSHAddress:               r.SSHAddress,
		SSHUser:                  r.SSHUser,
		SSHKeyPath:               r.SSHKeyPath,
		SSHPassword:              r.SSHPassword,
		SSHConfigPath:            r.SSHConfigPath,
		RemoteListen:             r.RemoteListen,
		ForwardAddress:           r.ForwardAddress,
		KnownHostsPath:           r.KnownHostsPath,
		TrustNewHostKey:          r.TrustNewHostKey,
		InsecureSkipHostKeyCheck: r.InsecureSkipHostKeyCheck,
		KeepAlive:                keepAlive,
		ReconnectDelay:           reconnect,
	}
	if err := tunnel.ValidateConfig(cfg); err != nil {
		return tunnel.Config{}, err
	}
	return cfg, nil
}

// TunnelView is the wire format for reading back a tunnel: live status plus
// a non-secret summary of its configuration. Passwords are never included.
type TunnelView struct {
	tunnel.Status
	Enabled                  bool   `json:"enabled"`
	Direction                string `json:"direction"`
	LocalListen              string `json:"local_listen"`
	SSHAddress               string `json:"ssh_address"`
	SSHUser                  string `json:"ssh_user"`
	SSHKeyPath               string `json:"ssh_key_path"`
	SSHConfigPath            string `json:"ssh_config_path"`
	RemoteListen             string `json:"remote_listen"`
	ForwardAddress           string `json:"forward_address"`
	KnownHostsPath           string `json:"known_hosts_path"`
	TrustNewHostKey          bool   `json:"trust_new_host_key"`
	InsecureSkipHostKeyCheck bool   `json:"insecure_skip_host_key_check"`
	KeepAlive                string `json:"keep_alive"`
	ReconnectDelay           string `json:"reconnect_delay"`
}

func newTunnelView(cfg tunnel.Config, enabled bool, status tunnel.Status) TunnelView {
	direction := cfg.Direction
	if direction == "" {
		direction = tunnel.DirectionLocal
	}
	return TunnelView{
		Status:                   status,
		Enabled:                  enabled,
		Direction:                string(direction),
		LocalListen:              cfg.LocalListen,
		SSHAddress:               cfg.SSHAddress,
		SSHUser:                  cfg.SSHUser,
		SSHKeyPath:               cfg.SSHKeyPath,
		SSHConfigPath:            cfg.SSHConfigPath,
		RemoteListen:             cfg.RemoteListen,
		ForwardAddress:           cfg.ForwardAddress,
		KnownHostsPath:           cfg.KnownHostsPath,
		TrustNewHostKey:          cfg.TrustNewHostKey,
		InsecureSkipHostKeyCheck: cfg.InsecureSkipHostKeyCheck,
		KeepAlive:                cfg.KeepAlive.String(),
		ReconnectDelay:           cfg.ReconnectDelay.String(),
	}
}
