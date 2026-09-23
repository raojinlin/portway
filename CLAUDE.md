# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Snapshot
- A daemon-based Go CLI + Web UI SSH tunnel manager. `.claude/settings.local.json` allows `Bash(bash:*)`, `Bash(env:*)`; no cursor/copilot rules.
- Purpose: 管理本地↔远程端口转发，支持多隧道持久化、指标展示（流量/速度/状态），以及 CLI 与 Web 两种管理方式。

## Commands
- **Build frontend first** (required before `go build` on a clean checkout, and after any `web/` change — `go:embed` needs `internal/daemon/webui/dist` to exist at compile time): `make web` (or `cd web && npm install && npm run build`)
- Build: `make build` (frontend + `go build ./...`); Go-only once frontend is built: `go build ./cmd/tunnel`
- Run daemon: `make run` (or `go run ./cmd/tunnel daemon`; blocks, serves HTTP API + Web UI on 127.0.0.1:7777 by default)
- Run help: `go run ./cmd/tunnel --help`
- Test: `make test` (or `go test ./...`; frontend has no tests yet)

## Architecture Notes
- **Daemon-first**: the CLI does *not* own tunnel state — every `add/list/status/rm/start/stop` invocation is a short-lived HTTP client of the long-running `tunnel daemon` process. A CLI command that can't reach the daemon prints a clear "run `tunnel daemon` first" error rather than silently doing nothing.
- `cmd/tunnel`: subcommands `daemon/add/list/status/rm/start/stop` using stdlib `flag`. Talks to the daemon via `internal/apiclient`. Global `--addr` (or `TUNNEL_DAEMON_ADDR` env) picks the daemon address.
- `internal/tunnel`: core per-tunnel logic, supports all three SSH forwarding modes via `Config.Direction` (`local` default / `remote` / `dynamic`).
  - `Config` describes a tunnel (`Direction`, local listen, SSH target/user/auth, `RemoteListen` (remote only), forward target (local/remote only), keepalive, reconnect, optional ssh config path, `KnownHostsPath`, `TrustNewHostKey`). `ValidateConfig(cfg)` holds the per-direction required-field rules and is shared by `Start` and the daemon's HTTP API (`daemon.TunnelRequest.toConfig`), so the rules live in exactly one place.
  - `Instance` manages one tunnel: SSH dial (applies ssh config host/user/key if provided, verifies host key — see below), then `setupDirection` opens the right listener and returns a `dstDialer` matching the mode — local dials the fixed target through the tunnel, remote asks the server to listen (`ssh.Client.Listen`) and dials the fixed target directly from this host, dynamic runs a SOCKS5 proxy (`socks5.go`) and dials whatever target each client requests, through the tunnel. `handleConn` is mode-agnostic: it just calls the `dstDialer` and pipes bytes both ways with byte-count metrics.
  - `Manager` manages multiple in-process tunnel instances (add/remove/list/status, stop all). Lives inside the daemon process only.
  - `hostkey.go`: `buildHostKeyCallback` resolves known_hosts (explicit override → ssh config `UserKnownHostsFile` → `~/.ssh/known_hosts`) and verifies via `golang.org/x/crypto/ssh/knownhosts`. `TrustNewHostKey` enables TOFU for hosts not yet recorded; a key change on an already-known host is always rejected.
  - `socks5.go`: minimal hand-written SOCKS5 server (RFC 1928) for dynamic forwarding — only "no auth" + CONNECT, no new dependency.
- `internal/store`: persists tunnel definitions (`Config` + `Enabled`) as JSON, default `~/.config/ssh-tunnel-manager/tunnels.json`, atomic write (temp file + rename).
- `internal/daemon`: the long-running process. `Run(ctx, Options)` loads `store` state, starts enabled tunnels via `Manager`, serves `/api/tunnels...` JSON API + the embedded Web UI (`webui/dist`, `go:embed` — see `embed.go`) on one `net/http` server, and shuts down gracefully (stop all tunnels, `http.Server.Shutdown`) when `ctx` is cancelled. The JSON API contract (`TunnelRequest`/`TunnelView` in `api.go`) is the boundary between Go and the frontend; it hasn't changed since the Web UI was rewritten in React, so backend and frontend can be worked on independently as long as that contract holds.
- `internal/apiclient`: HTTP client used by `cmd/tunnel` to talk to the daemon's JSON API; classifies connection failures into a clear "daemon unreachable" message.
- `web/`: the Web UI frontend — React + TypeScript + antd, built with Vite (`web/vite.config.ts` sets `build.outDir` straight to `internal/daemon/webui/dist`, so `npm run build` produces exactly what `go:embed` picks up, no copy step). `src/api.ts`/`src/types.ts` mirror the daemon's JSON API 1:1 (field names, including the mixed casing from `tunnel.Status` having no json tags — keep both sides in sync by hand if the API changes). `src/theme.ts` carries the deliberate "departure board" visual design (dark control-room palette, per-tunnel line-identity colors from a name hash, IBM Plex type) into antd's theme tokens rather than using antd's default look. The "add line" form is an antd `Modal` (`components/AddLineModal.tsx`), opened only via the header's button — it is not shown inline by default.
- Dependencies: Go side unchanged — `golang.org/x/crypto/ssh` (+ `ssh/knownhosts`), `github.com/kevinburke/ssh_config`; routing uses Go 1.22+ `net/http.ServeMux` patterns, assets use `embed`. Frontend brings the project's first Node/npm toolchain (`web/package.json`): react, antd, vite.

## Security/Hardening TODOs
- Web UI has no auth; it binds to 127.0.0.1 by default — do not expose `--addr` beyond localhost without adding auth/reverse-proxy in front.
- Add timeouts/bandwidth limits/log levels; improve reconnect/exit policy.
- Broaden test coverage to integration/end-to-end against a real SSH server.

## Usage snapshot
- Start daemon: `tunnel daemon` (or `--addr`, `--state` to override defaults)
- Add local (ssh config): `tunnel add --name demo --local 127.0.0.1:8080 --ssh hostAlias --ssh-config ~/.ssh/config --forward 10.0.0.1:80`
- Add local (explicit): `tunnel add --name demo --local 127.0.0.1:8080 --ssh host:22 --user user --key ~/.ssh/id_rsa --forward 10.0.0.1:80`
- Add remote: `tunnel add --name expose --direction remote --ssh host:22 --user user --remote-listen 0.0.0.0:9000 --forward 127.0.0.1:3000`
- Add dynamic/SOCKS5: `tunnel add --name proxy --direction dynamic --local 127.0.0.1:1080 --ssh host:22 --user user`
- Add with TOFU host key trust: append `--trust-new-host-key`
- List: `tunnel list`
- Status: `tunnel status demo`
- Stop (keep config): `tunnel stop demo` / Start again: `tunnel start demo`
- Remove: `tunnel rm demo`
- Web UI: `http://127.0.0.1:7777/` — line list only by default; "添加线路" button in the header opens the add-tunnel modal (same operations as the CLI)
