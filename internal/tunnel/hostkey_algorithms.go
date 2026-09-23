package tunnel

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Reuse knownhosts' hostname/port/wildcard/hash matching without doing a network
// handshake. This impossible key only queries matching records; it never verifies
// or enrolls a server, and is never passed to the TOFU callback.
type hostKeyProbe struct{}

func (hostKeyProbe) Type() string                        { return "known-hosts-query" }
func (hostKeyProbe) Marshal() []byte                     { return nil }
func (hostKeyProbe) Verify([]byte, *ssh.Signature) error { return errors.New("not a signing key") }

func hostKeyAlgorithms(cfg Config, alias, address, specification string) ([]string, error) {
	algorithms, preferKnown, err := configuredHostKeyAlgorithms(specification)
	if err != nil || !preferKnown || cfg.InsecureSkipHostKeyCheck {
		return algorithms, err
	}
	filename, err := resolveKnownHostsPath(cfg, alias)
	if err != nil {
		return nil, err
	}
	base, err := knownhosts.New(filename)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts algorithms: %w", err)
	}
	err = base(address, proxyAddr(address), hostKeyProbe{})
	var keyError *knownhosts.KeyError
	if !errors.As(err, &keyError) {
		return nil, fmt.Errorf("query known_hosts algorithms for %s: %w", address, err)
	}
	if len(keyError.Want) == 0 {
		return algorithms, nil
	}
	// CA key types do not restrict the certified host key's type. Preserve
	// certificate preference for any matching @cert-authority entry.
	caLines, err := certificateAuthorityLines(filename)
	if err != nil {
		return nil, err
	}
	knownTypes := make(map[string]bool)
	knownCA := false
	for _, known := range keyError.Want {
		if caLines[known.Line] {
			knownCA = true
			continue
		}
		knownTypes[known.Key.Type()] = true
	}
	var preferred, remaining []string
	for _, algorithm := range algorithms {
		known := knownTypes[hostKeyFormat(algorithm)]
		if strings.Contains(algorithm, "-cert-v01@openssh.com") {
			known = known || knownCA
		}
		if known {
			preferred = append(preferred, algorithm)
		} else {
			remaining = append(remaining, algorithm)
		}
	}
	return append(preferred, remaining...), nil
}

func hostKeyFormat(algorithm string) string {
	switch algorithm {
	case ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSASHA512:
		return ssh.KeyAlgoRSA
	case ssh.CertAlgoRSASHA256v01, ssh.CertAlgoRSASHA512v01:
		return ssh.CertAlgoRSAv01
	default:
		return algorithm
	}
}

func certificateAuthorityLines(filename string) (map[int]bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	lines := make(map[int]bool)
	scanner := bufio.NewScanner(file)
	for n := 1; scanner.Scan(); n++ {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 0 && fields[0] == "@cert-authority" {
			lines[n] = true
		}
	}
	return lines, scanner.Err()
}

// OpenSSH list syntax: replace, append (+), remove (-), or prepend (^).
// Explicit replacement/prepending preserves the user's order; defaults and
// +/- lists prefer already trusted key types without adding excluded algorithms.
func configuredHostKeyAlgorithms(specification string) ([]string, bool, error) {
	defaults := ssh.SupportedAlgorithms().HostKeys
	if specification == "" {
		return defaults, true, nil
	}
	universe := append(ssh.SupportedAlgorithms().HostKeys, ssh.InsecureAlgorithms().HostKeys...)
	operation := byte(0)
	if strings.ContainsRune("+-^", rune(specification[0])) {
		operation, specification = specification[0], specification[1:]
	}
	var selected []string
	for _, pattern := range strings.Split(specification, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return nil, false, errors.New("HostKeyAlgorithms contains an empty algorithm")
		}
		matched := false
		for _, algorithm := range universe {
			match, err := path.Match(pattern, algorithm)
			if err != nil {
				return nil, false, fmt.Errorf("invalid HostKeyAlgorithms pattern %q: %w", pattern, err)
			}
			if match {
				selected = append(selected, algorithm)
				matched = true
			}
		}
		if !matched && operation != '-' {
			return nil, false, fmt.Errorf("unsupported HostKeyAlgorithms %q", pattern)
		}
	}
	var result []string
	switch operation {
	case '+':
		result = append(defaults, selected...)
	case '^':
		result = append(selected, defaults...)
	case '-':
		excluded := make(map[string]bool)
		for _, algorithm := range selected {
			excluded[algorithm] = true
		}
		for _, algorithm := range defaults {
			if !excluded[algorithm] {
				result = append(result, algorithm)
			}
		}
	default:
		result = selected
	}
	seen := make(map[string]bool)
	unique := make([]string, 0, len(result))
	for _, algorithm := range result {
		if !seen[algorithm] {
			unique = append(unique, algorithm)
			seen[algorithm] = true
		}
	}
	if len(unique) == 0 {
		return nil, false, errors.New("HostKeyAlgorithms leaves no supported algorithms")
	}
	return unique, operation == '+' || operation == '-', nil
}
