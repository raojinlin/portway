package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ssh-tunnel-manager/internal/logging"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type contextDialer func(context.Context, string, string) (net.Conn, error)

func buildSSHAuth(ctx context.Context, cfg Config, settings SSHConfig) ([]ssh.AuthMethod, func(), error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, func() {}, err
	}
	return loadSSHAuth(ctx, cfg, settings, home, (&net.Dialer{}).DialContext)
}

// Agent connections must stay open until authentication finishes, then be closed.
// No private key is exported by the agent and no external command is executed.
func loadSSHAuth(ctx context.Context, cfg Config, settings SSHConfig, home string, dial contextDialer) ([]ssh.AuthMethod, func(), error) {
	logger := logging.FromContext(ctx)
	paths := settings.Identities
	if len(paths) == 0 && settings.Identity != "" {
		paths = []string{settings.Identity}
	}
	if cfg.SSHKeyPath != "" {
		paths = []string{cfg.SSHKeyPath}
	}
	explicit := len(paths) > 0
	if !explicit {
		paths = []string{"~/.ssh/id_ed25519", "~/.ssh/id_ecdsa", "~/.ssh/id_rsa"}
	}
	source := "default_keys"
	if explicit {
		source = "identity_files"
	}
	if cfg.SSHKeyPath != "" {
		source = "explicit_key"
	}
	logger.Debug("SSH authentication loading", "identity_source", source, "identities_only", settings.IdentitiesOnly, "agent_enabled", settings.IdentityAgent != "none", "password_configured", cfg.SSHPassword != "")
	expand := func(path string) string {
		if strings.HasPrefix(path, "~/") {
			return filepath.Join(home, path[2:])
		}
		return path
	}
	var signers []ssh.Signer
	allowed, seen := make(map[string]bool), make(map[string]bool)
	add := func(signer ssh.Signer) {
		key := string(signer.PublicKey().Marshal())
		if !seen[key] {
			seen[key] = true
			signers = append(signers, signer)
		}
	}
	var problems []string
	for _, path := range paths {
		if path == "none" {
			continue
		}
		path = expand(path)
		data, err := os.ReadFile(path)
		if err == nil {
			var signer ssh.Signer
			signer, err = ssh.ParsePrivateKey(data)
			if err == nil {
				allowed[string(signer.PublicKey().Marshal())] = true
				add(signer)
			} else {
				var encrypted *ssh.PassphraseMissingError
				if errors.As(err, &encrypted) {
					if encrypted.PublicKey != nil {
						allowed[string(encrypted.PublicKey.Marshal())] = true
					}
					problems = append(problems, fmt.Sprintf("encrypted key %s: unlock it in ssh-agent (interactive passphrase prompts are not supported)", path))
				} else {
					problems = append(problems, fmt.Sprintf("parse key %s: %v", path, err))
				}
			}
		} else if explicit || !errors.Is(err, os.ErrNotExist) {
			problems = append(problems, fmt.Sprintf("read key %s: %v", path, err))
		}
		// A public-only IdentityFile or sidecar also selects the matching agent key.
		for _, publicData := range [][]byte{data, readPublicKey(path + ".pub")} {
			if key, _, _, _, err := ssh.ParseAuthorizedKey(publicData); err == nil {
				allowed[string(key.Marshal())] = true
			}
		}
	}

	cleanup := func() {}
	if settings.IdentityAgent != "none" {
		socket := settings.IdentityAgent
		if socket == "" || socket == "SSH_AUTH_SOCK" || socket == "$SSH_AUTH_SOCK" || socket == "${SSH_AUTH_SOCK}" {
			socket = os.Getenv("SSH_AUTH_SOCK")
		}
		if socket != "" {
			agentSigners, closeAgent, err := connectSSHAgent(ctx, expand(socket), dial)
			if err == nil {
				cleanup = closeAgent
				for _, signer := range agentSigners {
					if allowed[string(signer.PublicKey().Marshal())] {
						add(signer)
					}
				}
				if !settings.IdentitiesOnly {
					for _, signer := range agentSigners {
						add(signer)
					}
				}
			}
			if err != nil {
				problems = append(problems, fmt.Sprintf("ssh-agent: %v", err))
				logger.Debug("SSH agent unavailable; trying other authentication", "error", logging.SafeError(err, cfg.SSHPassword))
			}
		}
	}
	var methods []ssh.AuthMethod
	if len(signers) > 0 {
		methods = append(methods, ssh.PublicKeys(signers...))
	}
	if cfg.SSHPassword != "" {
		methods = append(methods, ssh.Password(cfg.SSHPassword))
	}
	if ctx.Err() != nil {
		cleanup()
		return nil, func() {}, ctx.Err()
	}
	if len(methods) == 0 {
		cleanup()
		detail := ""
		if len(problems) > 0 {
			detail = ": " + strings.Join(problems, "; ")
		}
		return nil, func() {}, fmt.Errorf("no usable SSH authentication (checked IdentityFile/default keys and ssh-agent; configure a key/password or load a key into the agent)%s", detail)
	}
	logger.Debug("SSH authentication prepared", "public_keys", len(signers), "methods", len(methods), "skipped_identities", len(problems))
	return methods, cleanup, nil
}

func connectSSHAgent(ctx context.Context, socket string, dial contextDialer) ([]ssh.Signer, func(), error) {
	// A stale optional agent must not consume the whole SSH handshake timeout
	// and prevent otherwise valid file-key/password authentication.
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	conn, err := dial(probeCtx, "unix", socket)
	if err != nil {
		return nil, func() {}, err
	}
	stopProbe := context.AfterFunc(probeCtx, func() { conn.Close() })
	signers, err := agent.NewClient(conn).Signers()
	stopped := stopProbe()
	if probeCtx.Err() != nil {
		err = probeCtx.Err()
	}
	if err != nil || !stopped {
		conn.Close()
		if err == nil {
			err = context.DeadlineExceeded
		}
		return nil, func() {}, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	_ = conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	return signers, func() { stop(); conn.Close() }, nil
}

func readPublicKey(path string) []byte {
	data, _ := os.ReadFile(path)
	return data
}
