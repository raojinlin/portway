package tunnel

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestNativeProxyCommandRecognition(t *testing.T) {
	for _, tc := range []struct{ command, want string }{
		{"ssh -W %h:%p -q drop", "drop"},
		{"/usr/bin/ssh -q -p 2222 -l alice -W '%h:%p' drop", "alice@drop:2222"},
		{"ssh -W [%h]:%p root@[2001:db8::1]", "root@[2001:db8::1]"},
		{"ssh -W %h:%p -o StrictHostKeyChecking=no drop", ""},
		{"ssh -W other:22 drop", ""},
		{"ssh -W %h:%p drop && touch /tmp/no", ""},
		{"ssh -W %h:%p $(anything)", ""},
		{"ssh -W %h:%p drop;anything", ""},
		{"ssh -W %h:%p\n drop", ""},
		{"nc %h %p", ""},
		{"none", ""},
	} {
		got, ok := nativeProxyJump(tc.command)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("%q => %q,%v; want %q", tc.command, got, ok, tc.want)
		}
	}
}

func TestNativeRouteResolution(t *testing.T) {
	for _, tc := range []struct {
		name, config, want string
		wantErr            bool
	}{
		{"single", "Host target\n ProxyJump jump\n", "jump:22,target:22", false},
		{"multiple", "Host target\n ProxyJump alice@jump:2201,bob@[2001:db8::1]:2202\n", "jump:2201,[2001:db8::1]:2202,target:22", false},
		{"recursive", "Host target\n ProxyJump second\nHost second\n ProxyJump first\n", "first:22,second:22,target:22", false},
		{"cycle", "Host target\n ProxyJump jump\nHost jump\n ProxyJump target\n", "", true},
		{"duplicate", "Host target\n ProxyJump jump,jump\n", "", true},
		{"alias cycle", "Host target jump\n HostName same\n ProxyJump jump\n", "", true},
		{"empty hop", "Host target\n ProxyJump jump,\n", "", true},
		{"disabled", "Host target\n ProxyJump none\n", "target:22", false},
		{"command disabled", "Host target\n ProxyJump jump\n ProxyCommand none\n", "target:22", false},
		{"compat command", "Host target\n ProxyCommand ssh -W %h:%p -q jump\n", "jump:22,target:22", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config")
			if err := os.WriteFile(path, []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			route, err := resolveSSHRoute(Config{SSHAddress: "target", SSHConfigPath: path}, make(map[string]bool))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected route error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var addresses []string
			for _, endpoint := range route {
				addresses = append(addresses, endpoint.address)
			}
			if got := strings.Join(addresses, ","); got != tc.want {
				t.Fatalf("route = %s; want %s", got, tc.want)
			}
		})
	}
}

type nativeTestHost struct {
	name, address, user, keyPath string
	userKey, hostKey             ssh.Signer
	done                         chan struct{}
	extraHostKeys                []ssh.Signer
}

type testChannelConn struct{ ssh.Channel }

func (c testChannelConn) LocalAddr() net.Addr              { return fakeAddr{"127.0.0.1:22"} }
func (c testChannelConn) RemoteAddr() net.Addr             { return fakeAddr{"127.0.0.1:10000"} }
func (c testChannelConn) SetDeadline(time.Time) error      { return nil }
func (c testChannelConn) SetReadDeadline(time.Time) error  { return nil }
func (c testChannelConn) SetWriteDeadline(time.Time) error { return nil }

func serveNativeTestHost(t *testing.T, raw net.Conn, hosts []*nativeTestHost, index int, mode string) {
	t.Helper()
	host := hosts[index]
	defer close(host.done)
	defer raw.Close()
	config := &ssh.ServerConfig{PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if meta.User() != host.user || !bytes.Equal(key.Marshal(), host.userKey.PublicKey().Marshal()) {
			return nil, errors.New("wrong per-hop identity")
		}
		return nil, nil
	}}
	config.AddHostKey(host.hostKey)
	for _, key := range host.extraHostKeys {
		config.AddHostKey(key)
	}
	conn, channels, requests, err := ssh.NewServerConn(raw, config)
	if err != nil {
		return
	}
	defer conn.Close()
	go func() {
		for req := range requests {
			req.Reply(req.Type == "probe", []byte(host.name))
		}
	}()
	var children sync.WaitGroup
	defer children.Wait()
	for request := range channels {
		if request.ChannelType() != "direct-tcpip" {
			request.Reject(ssh.UnknownChannelType, "test")
			continue
		}
		var target struct {
			Host       string
			Port       uint32
			Origin     string
			OriginPort uint32
		}
		if err := ssh.Unmarshal(request.ExtraData(), &target); err != nil {
			request.Reject(ssh.ConnectionFailed, "bad target")
			continue
		}
		if mode == "reject" {
			request.Reject(ssh.ConnectionFailed, "test target unavailable")
			continue
		}
		if mode == "stall-open" {
			continue
		}
		if index+1 < len(hosts) && net.JoinHostPort(target.Host, fmt.Sprint(target.Port)) != hosts[index+1].address {
			t.Errorf("forwarded to wrong host: %+v", target)
			request.Reject(ssh.ConnectionFailed, "wrong target")
			continue
		}
		channel, reqs, err := request.Accept()
		if err != nil {
			continue
		}
		go ssh.DiscardRequests(reqs)
		children.Add(1)
		go func() {
			defer children.Done()
			defer channel.Close()
			if mode == "stall-handshake" {
				io.Copy(io.Discard, channel)
				return
			}
			if index+1 < len(hosts) {
				serveNativeTestHost(t, testChannelConn{channel}, hosts, index+1, mode)
			} else {
				io.Copy(channel, channel)
			}
		}()
	}
}

func nativeFixture(t *testing.T, proxy string, count int, badHost int, mode string) (Config, contextDialer, []*nativeTestHost) {
	t.Helper()
	dir := t.TempDir()
	knownPath := filepath.Join(dir, "known_hosts")
	var hosts []*nativeTestHost
	var config, known strings.Builder
	for n := 0; n < count; n++ {
		name := fmt.Sprintf("host%d", n)
		path := filepath.Join(dir, name+"-key")
		_, userKey := testPrivateKey(t, path, "")
		_, hostKey := testPrivateKey(t, "", "")
		host := &nativeTestHost{name: name, address: fmt.Sprintf("%s.test:%d", name, 2200+n), user: name + "-user", keyPath: path, userKey: userKey, hostKey: hostKey, done: make(chan struct{})}
		hosts = append(hosts, host)
		fmt.Fprintf(&config, "Host %s\n HostName %s.test\n Port %d\n User %s\n IdentityFile %s\n IdentityAgent none\n UserKnownHostsFile %s\n", name, name, 2200+n, host.user, path, knownPath)
		if n == count-1 {
			fmt.Fprintf(&config, " %s\n", proxy)
		}
		key := hostKey.PublicKey()
		if n == badHost {
			key = genPublicKey(t)
		}
		known.WriteString(knownhosts.Line([]string{knownhosts.Normalize(host.address)}, key) + "\n")
	}
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(config.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(knownPath, []byte(known.String()), 0600); err != nil {
		t.Fatal(err)
	}
	dial := func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != hosts[0].address {
			return nil, fmt.Errorf("unexpected direct dial: %s %s", network, address)
		}
		a, b := bufferedTestPipe(t)
		go serveNativeTestHost(t, b, hosts, 0, mode)
		return a, nil
	}
	return Config{SSHAddress: hosts[count-1].name, SSHConfigPath: path}, dial, hosts
}

func waitNativeHosts(t *testing.T, hosts []*nativeTestHost) {
	t.Helper()
	for _, host := range hosts {
		select {
		case <-host.done:
		case <-time.After(2 * time.Second):
			t.Fatalf("SSH connection leaked: %s", host.name)
		}
	}
}

func TestNativeSSHWithoutSSHExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, tc := range []struct {
		name, proxy string
		count       int
	}{
		{"direct", "", 1},
		{"single jump", "ProxyJump host0", 2},
		{"multiple jumps", "ProxyJump host0,host1", 3},
		{"legacy command", "ProxyCommand ssh -W %h:%p -q host0", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, dial, hosts := nativeFixture(t, tc.proxy, tc.count, -1, "")
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			client, err := dialNativeSSH(ctx, cfg, dial)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ok, reply, err := client.SendRequest("probe", true, nil)
			if err != nil || !ok || string(reply) != hosts[len(hosts)-1].name {
				t.Fatalf("probe = %v %q %v", ok, reply, err)
			}
			stream, err := client.Dial("tcp", "echo.test:443")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := stream.Write([]byte("native-forwarding")); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, len("native-forwarding"))
			if _, err := io.ReadFull(stream, got); err != nil || string(got) != "native-forwarding" {
				t.Fatalf("forwarding = %q %v", got, err)
			}
			stream.Close()
			client.Close()
			waitNativeHosts(t, hosts)
		})
	}
}

func TestNativeJumpHostKeyVerification(t *testing.T) {
	for _, badHost := range []int{0, 1} {
		t.Run(fmt.Sprint(badHost), func(t *testing.T) {
			cfg, dial, hosts := nativeFixture(t, "ProxyJump host0", 2, badHost, "")
			// A target-only bypass must never disable the jump's host-key verification.
			if badHost == 0 {
				cfg.InsecureSkipHostKeyCheck = true
			}
			client, err := dialNativeSSH(context.Background(), cfg, dial)
			if client != nil {
				client.Close()
			}
			if err == nil || !strings.Contains(err.Error(), "key mismatch") {
				t.Fatalf("host %d not verified: %v", badHost, err)
			}
			waitNativeHosts(t, hosts[:badHost+1])
		})
	}
}

func TestNativeJumpFailureAndCancellation(t *testing.T) {
	for _, mode := range []string{"reject", "stall-open", "stall-handshake"} {
		t.Run(mode, func(t *testing.T) {
			cfg, dial, hosts := nativeFixture(t, "ProxyJump host0", 2, -1, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			client, err := dialNativeSSH(ctx, cfg, dial)
			if client != nil {
				client.Close()
			}
			if err == nil {
				t.Fatal("expected failure")
			}
			if mode != "reject" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("cancellation = %v", err)
			}
			waitNativeHosts(t, hosts[:1])
		})
	}
}

func TestNativeJumpTargetAuthFailureCleansChain(t *testing.T) {
	cfg, dial, hosts := nativeFixture(t, "ProxyJump host0", 2, -1, "")
	hosts[1].user = "reject-target-user"
	client, err := dialNativeSSH(context.Background(), cfg, dial)
	if client != nil {
		client.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "unable to authenticate") {
		t.Fatalf("auth failure = %v", err)
	}
	waitNativeHosts(t, hosts)
}

func TestJumpDoesNotInheritTargetCredentials(t *testing.T) {
	cfg, _, _ := nativeFixture(t, "ProxyJump host0", 2, -1, "")
	cfg.SSHPassword, cfg.SSHKeyPath = "target-password", "/target/key"
	cfg.TrustNewHostKey, cfg.InsecureSkipHostKeyCheck = true, true
	route, err := resolveSSHRoute(cfg, make(map[string]bool))
	if err != nil {
		t.Fatal(err)
	}
	hop := route[0].config
	if hop.SSHPassword != "" || hop.SSHKeyPath != "" || hop.TrustNewHostKey || hop.InsecureSkipHostKeyCheck {
		t.Fatal("target credentials or insecure options propagated to jump")
	}
}

func addECDSAHostKeys(t *testing.T, hosts []*nativeTestHost) {
	t.Helper()
	for _, host := range hosts {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		signer, err := ssh.NewSignerFromKey(key)
		if err != nil {
			t.Fatal(err)
		}
		host.extraHostKeys = []ssh.Signer{signer}
	}
}

func TestDefaultSSHAlgorithmsReproduceKnownHostMismatch(t *testing.T) {
	cfg, dial, hosts := nativeFixture(t, "", 1, -1, "")
	addECDSAHostKeys(t, hosts)
	endpoint, err := resolveEndpoint(cfg)
	if err != nil {
		t.Fatal(err)
	}
	callback, err := buildHostKeyCallback(cfg, endpoint.alias)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := dial(context.Background(), "tcp", endpoint.address)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	client, _, _, err := ssh.NewClientConn(raw, endpoint.address, &ssh.ClientConfig{User: endpoint.user, Auth: []ssh.AuthMethod{ssh.PublicKeys(hosts[0].userKey)}, HostKeyCallback: callback})
	if client != nil {
		client.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "key mismatch") || !strings.Contains(err.Error(), "received=ecdsa-sha2-nistp256") {
		t.Fatalf("old behavior not reproduced: %v", err)
	}
	waitNativeHosts(t, hosts)
}

func TestNativeJumpPrefersTrustedHostKeys(t *testing.T) {
	for _, badHost := range []int{-1, 0, 1} {
		t.Run(fmt.Sprint(badHost), func(t *testing.T) {
			cfg, dial, hosts := nativeFixture(t, "ProxyJump host0", 2, badHost, "")
			addECDSAHostKeys(t, hosts)
			data, err := os.ReadFile(cfg.SSHConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, []byte("Host *\n HostKeyAlgorithms +ssh-rsa\n")...)
			if err := os.WriteFile(cfg.SSHConfigPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			client, err := dialNativeSSH(context.Background(), cfg, dial)
			if badHost >= 0 {
				if client != nil {
					client.Close()
				}
				if err == nil || !strings.Contains(err.Error(), "key mismatch") || !strings.Contains(err.Error(), "received=ssh-ed25519") {
					t.Fatalf("changed trusted key not rejected: %v", err)
				}
				waitNativeHosts(t, hosts[:badHost+1])
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ok, reply, err := client.SendRequest("probe", true, nil)
			if err != nil || !ok || string(reply) != hosts[1].name {
				t.Fatalf("jump connection failed: %s %v", reply, err)
			}
			client.Close()
			waitNativeHosts(t, hosts)
		})
	}
}
