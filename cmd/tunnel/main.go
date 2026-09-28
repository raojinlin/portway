package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ssh-tunnel-manager/internal/apiclient"
	"ssh-tunnel-manager/internal/daemon"
	"ssh-tunnel-manager/internal/logging"
)

var version = "dev"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Println(version)
		return
	}
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	switch cmd {
	case "daemon":
		daemonCmd(ctx, args)
	case "add":
		addCmd(args)
	case "list":
		listCmd(args)
	case "status":
		statusCmd(args)
	case "rm":
		rmCmd(args)
	case "start":
		startCmd(args)
	case "stop":
		stopCmd(args)
	default:
		usage()
		os.Exit(1)
	}
}

// addrFlag adds the shared --addr flag (also honoring TUNNEL_DAEMON_ADDR)
// to fs and returns a func to resolve its final value after Parse.
func addrFlag(fs *flag.FlagSet) *string {
	def := daemon.DefaultAddr
	if v := os.Getenv("TUNNEL_DAEMON_ADDR"); v != "" {
		def = v
	}
	return fs.String("addr", def, "daemon address (host:port); also settable via TUNNEL_DAEMON_ADDR")
}

func newClient(addr string) *apiclient.Client {
	return apiclient.New(addr)
}

func fail(err error) {
	if errors.Is(err, apiclient.ErrDaemonUnreachable) {
		fmt.Fprintln(os.Stderr, "无法连接到守护进程，请先运行: portway daemon")
		fmt.Fprintln(os.Stderr, "详情:", err)
	} else {
		fmt.Fprintln(os.Stderr, "错误:", err)
	}
	os.Exit(1)
}

func daemonCmd(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	configPath := fs.String("config", "", "YAML configuration file (default: ~/.config/ssh-tunnel-manager/config.yaml)")
	addr := fs.String("addr", "", "override listen address (default: config or 127.0.0.1:7777)")
	statePath := fs.String("state", "", "path to state file (default: ~/.config/ssh-tunnel-manager/tunnels.json)")
	logLevel := fs.String("log-level", "", "override log level: debug, info, warn, error (default: config or info)")
	logFormat := fs.String("log-format", "", "override log format: text or json (default: config or text)")
	_ = fs.Parse(args)
	logger, err := logging.New(os.Stderr, *logLevel, *logFormat)
	if err != nil {
		log.Fatal(err)
	}

	if err := daemon.Run(ctx, daemon.Options{ConfigPath: *configPath, Addr: *addr, StatePath: *statePath, LogLevel: *logLevel, LogFormat: *logFormat}); err != nil {
		logger.Error("daemon exited with error", "error", logging.SafeError(err))
		os.Exit(1)
	}
}

func addCmd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	addr := addrFlag(fs)
	name := fs.String("name", "", "tunnel name")
	direction := fs.String("direction", "local", "forwarding direction: local (-L), remote (-R), or dynamic (-D, SOCKS5)")
	local := fs.String("local", "", "local listen address (host:port); used by local and dynamic")
	sshAddr := fs.String("ssh", "", "ssh server address or ssh config host")
	sshCfg := fs.String("ssh-config", "", "path to ssh config (default ~/.ssh/config)")
	user := fs.String("user", "", "ssh username (default: ssh config User, then local user)")
	key := fs.String("key", "", "path to private key (default: IdentityFile, default SSH keys, or ssh-agent)")
	pass := fs.String("pass", "", "ssh password (discouraged; prefer key)")
	remoteListen := fs.String("remote-listen", "", "address the ssh server should bind (host:port); remote only")
	forward := fs.String("forward", "", "target host:port to reach; used by local and remote")
	knownHosts := fs.String("known-hosts", "", "path to known_hosts file (default: ssh config's UserKnownHostsFile or ~/.ssh/known_hosts)")
	trustNewHostKey := fs.Bool("trust-new-host-key", false, "trust-on-first-use for hosts not yet in known_hosts")
	insecureSkipHostKeyCheck := fs.Bool("insecure-skip-host-key-check", false, "disable SSH host key verification (unsafe)")
	keepAlive := fs.Duration("keepalive", 30*time.Second, "ssh keepalive interval")
	reconnect := fs.Duration("reconnect", 3*time.Second, "reconnect delay")
	_ = fs.Parse(args)

	req := daemon.TunnelRequest{
		Name:                     *name,
		Direction:                *direction,
		LocalListen:              *local,
		SSHAddress:               *sshAddr,
		SSHConfigPath:            *sshCfg,
		SSHUser:                  *user,
		SSHKeyPath:               *key,
		SSHPassword:              *pass,
		RemoteListen:             *remoteListen,
		ForwardAddress:           *forward,
		KnownHostsPath:           *knownHosts,
		TrustNewHostKey:          *trustNewHostKey,
		InsecureSkipHostKeyCheck: *insecureSkipHostKeyCheck,
		KeepAlive:                keepAlive.String(),
		ReconnectDelay:           reconnect.String(),
	}

	v, err := newClient(*addr).Create(req)
	if err != nil {
		fail(err)
	}
	fmt.Println("added", v.Name)
}

func listCmd(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	addr := addrFlag(fs)
	_ = fs.Parse(args)

	views, err := newClient(*addr).List()
	if err != nil {
		fail(err)
	}
	for _, v := range views {
		fmt.Printf("%s\t%s\tbytes_in=%d bytes_out=%d last_err=%s\n", v.Name, v.State, v.BytesIn, v.BytesOut, v.LastError)
	}
}

func statusCmd(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	addr := addrFlag(fs)
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		log.Fatal("status requires name")
	}
	name := fs.Arg(0)

	v, err := newClient(*addr).Get(name)
	if err != nil {
		fail(err)
	}
	fmt.Printf("%s\t%s\tbytes_in=%d bytes_out=%d conns=%d last_err=%s\n", v.Name, v.State, v.BytesIn, v.BytesOut, v.ActiveConns, v.LastError)
}

func rmCmd(args []string) {
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	addr := addrFlag(fs)
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		log.Fatal("rm requires name")
	}
	name := fs.Arg(0)

	if err := newClient(*addr).Delete(name); err != nil {
		fail(err)
	}
	fmt.Println("removed", name)
}

func startCmd(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	addr := addrFlag(fs)
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		log.Fatal("start requires name")
	}
	name := fs.Arg(0)

	v, err := newClient(*addr).Start(name)
	if err != nil {
		fail(err)
	}
	fmt.Println("started", v.Name)
}

func stopCmd(args []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	addr := addrFlag(fs)
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		log.Fatal("stop requires name")
	}
	name := fs.Arg(0)

	v, err := newClient(*addr).Stop(name)
	if err != nil {
		fail(err)
	}
	fmt.Println("stopped", v.Name)
}

func usage() {
	fmt.Println("portway <daemon|add|list|status|rm|start|stop> [flags]")
	fmt.Println("portway --version")
	fmt.Println()
	fmt.Println("daemon flags:")
	fmt.Println("  --config <path>        YAML configuration file")
	fmt.Println("  --addr <host:port>     listen address (default 127.0.0.1:7777)")
	fmt.Println("  --state <path>         state file path")
	fmt.Println()
	fmt.Println("add flags:")
	fmt.Println("  --name <name>")
	fmt.Println("  --direction <local|remote|dynamic>   local=-L (default), remote=-R, dynamic=-D (SOCKS5)")
	fmt.Println("  --local <host:port>      local listen address; local & dynamic")
	fmt.Println("  --ssh <host:port or ssh-config alias>")
	fmt.Println("  --ssh-config <path>")
	fmt.Println("  --user <ssh user>")
	fmt.Println("  --key <path to private key>")
	fmt.Println("  --pass <password>")
	fmt.Println("  --remote-listen <host:port>   server-side bind address; remote only")
	fmt.Println("  --forward <host:port>    fixed target; local & remote")
	fmt.Println("  --known-hosts <path>")
	fmt.Println("  --trust-new-host-key")
	fmt.Println("  --keepalive <dur>")
	fmt.Println("  --reconnect <dur>")
	fmt.Println()
	fmt.Println("all subcommands accept --addr (or TUNNEL_DAEMON_ADDR) to target a non-default daemon")
}
