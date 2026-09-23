package tunnel

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// buildHostKeyCallback resolves a known_hosts file for the given tunnel
// config and returns an ssh.HostKeyCallback that verifies server host keys
// against it. If TrustNewHostKey is set, unknown hosts are trusted on first
// use (TOFU) and appended to the known_hosts file; a host whose key differs
// from a previously recorded one is always rejected, regardless of that flag.
func buildHostKeyCallback(cfg Config, sshHostForConfig string) (ssh.HostKeyCallback, error) {
	if cfg.InsecureSkipHostKeyCheck {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	path, err := resolveKnownHostsPath(cfg, sshHostForConfig)
	if err != nil {
		return nil, err
	}

	if _, statErr := os.Stat(path); statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			return nil, fmt.Errorf("stat known_hosts %s: %w", path, statErr)
		}
		if !cfg.TrustNewHostKey {
			return nil, fmt.Errorf("known_hosts file %s not found; run `ssh-keyscan -H <host> >> %s` or enable --trust-new-host-key", path, path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create known_hosts dir: %w", err)
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create known_hosts %s: %w", path, err)
		}
		f.Close()
	}

	base, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts %s: %w", path, err)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := base(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if cfg.TrustNewHostKey && errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
			// Unknown host: trust on first use and persist the key.
			line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
			f, openErr := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
			if openErr != nil {
				return fmt.Errorf("record new host key: %w", openErr)
			}
			defer f.Close()
			if _, writeErr := f.WriteString(line + "\n"); writeErr != nil {
				return fmt.Errorf("record new host key: %w", writeErr)
			}
			return nil
		}
		// Known host with a mismatched key (or any other error): reject.
		return describeHostKeyError(err, hostname, path, key)
	}, nil
}

func describeHostKeyError(err error, hostname, path string, received ssh.PublicKey) error {
	var mismatch *knownhosts.KeyError
	var revoked *knownhosts.RevokedError
	var expected []string
	if errors.As(err, &mismatch) {
		for n, want := range mismatch.Want {
			if n == 8 {
				expected = append(expected, fmt.Sprintf("and %d more entries", len(mismatch.Want)-n))
				break
			}
			expected = append(expected, fmt.Sprintf("%s %s at %s:%d", want.Key.Type(), ssh.FingerprintSHA256(want.Key), want.Filename, want.Line))
		}
		if len(expected) == 0 {
			expected = append(expected, "no matching entry in "+path)
		}
	} else if errors.As(err, &revoked) {
		expected = append(expected, fmt.Sprintf("revoked at %s:%d", revoked.Revoked.Filename, revoked.Revoked.Line))
	} else {
		return err
	}
	return fmt.Errorf("%w; host=%s; received=%s %s; known_hosts=%s", err, hostname, received.Type(), ssh.FingerprintSHA256(received), strings.Join(expected, "; "))
}

// resolveKnownHostsPath picks the known_hosts file to use, in priority
// order: explicit Config override, the resolved host's ssh config
// UserKnownHostsFile entry, then the default ~/.ssh/known_hosts.
func resolveKnownHostsPath(cfg Config, sshHostForConfig string) (string, error) {
	if cfg.KnownHostsPath != "" {
		return expandHome(cfg.KnownHostsPath)
	}

	if sshHostForConfig != "" {
		host, _, err := splitSSHAddress(sshHostForConfig)
		if err != nil {
			return "", err
		}
		if path := lookupUserKnownHostsFile(host, cfg.SSHConfigPath); path != "" {
			return expandHome(path)
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "known_hosts"), nil
}

func expandHome(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	if len(path) > 1 && path[1] == '/' {
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}
