# SSH Tunnel Manager

一个基于 Go 的 SSH 隧道管理器：常驻 daemon 进程负责建立/保持隧道并持久化配置，CLI 与内置 Web 管理页面都是它的客户端。

## 核心功能
- 三种 SSH 转发模式：本地转发 (`-L`)、远程转发 (`-R`)、动态转发 / SOCKS5 (`-D`)
- 隧道可视化：累计流量、当前连接数、状态（CLI 与 Web 页面均可查看）
- Web 页面点击“当前连接”打开详情弹窗：来源、目标、状态、接入时间、持续时长、逐连接收发流量；打开时每 2 秒刷新，仅展示当前连接，断开后移除。动态代理显示客户端请求的实际目标。也可通过 `GET /api/tunnels/{name}/connections` 获取。
- 单次目标连接失败（如 SOCKS5 目标超时或拒绝连接）只结束该请求，不将整条线路标为异常；SOCKS5 客户端仍会收到失败响应。SSH 建连、监听等线路级错误仍显示异常。
- SOCKS5 连接弹窗提供“历史连接”页签，包含已结束请求的来源、目标、接入/结束时间、持续时长、收发流量和失败原因，按结束时间倒序显示，每 2 秒刷新。请求结束时追加到独立 JSONL 日志，页面显示每条线路最近 500 条；停止或重新启动线路不会清空，daemon 重启时从保留的日志文件恢复。接口：`GET /api/tunnels/{name}/connections/history`，返回 `{ "connections": [...], "limit": 500, "persisted": true }`，写入失败时还会返回 `persistence_error` 并在页面告警。
- 多隧道持久化：daemon 重启后自动恢复已启用的隧道
- SSH host key 校验（默认强制校验 known_hosts，可选 TOFU）

## 转发模式
- **本地转发（`local`，默认）**：本机监听 `--local`，连接进来后通过隧道转发到固定的 `--forward` 目标。等价于 `ssh -L`。
- **远程转发（`remote`）**：让 SSH 服务器监听 `--remote-listen`，外部连上那个端口的流量通过隧道传回本机，再转发到本机可达的 `--forward` 目标。等价于 `ssh -R`，典型用途是把内网服务暴露给跳板机。
- **动态转发（`dynamic`）**：本机监听 `--local` 作为 SOCKS5 代理，目标地址由每次连接动态指定（不需要 `--forward`），通过隧道按需转发。等价于 `ssh -D`。

## 架构
- `cmd/tunnel`：CLI 入口。
  - `tunnel daemon`：启动常驻进程（HTTP API + Web UI），阻塞运行，`Ctrl+C` 优雅退出。
  - `tunnel add/list/status/rm/start/stop`：通过 HTTP 调用 daemon 的 API，本身不持有任何隧道状态。
- `internal/tunnel`：核心隧道逻辑。
  - `Config` 描述单个隧道（转发模式 `Direction`、本地/远程监听、SSH 目标/用户/认证、转发目标、keepalive、重连、known_hosts）。
  - `Instance` 负责 SSH 拨号（含 known_hosts 校验）、按 `Direction` 建立监听与目标拨号、双向转发、指标统计、保活、断线重连。
  - `Manager` 管理进程内运行中的多个 `Instance`。
  - `socks5.go`：动态转发用的最小 SOCKS5 服务端实现（仅 no-auth + CONNECT）。
- `internal/store`：把隧道定义（含是否启用）持久化为 JSON 文件，默认 `~/.config/ssh-tunnel-manager/tunnels.json`。
- `internal/daemon`：常驻进程本体。启动时从 `store` 加载并拉起已启用的隧道；提供 `/api/tunnels...` JSON API；同时把内嵌的 Web UI（`internal/daemon/webui/dist`，`go:embed`）serve 在 `/`。
- `internal/apiclient`：CLI 与 daemon 通信用的 HTTP 客户端。
- `web/`：Web UI 前端源码（React + TypeScript + antd，Vite 构建）。`npm run build` 直接产出到 `internal/daemon/webui/dist`（见 `web/vite.config.ts`），Go 端 `go:embed` 这个目录。

## 快速开始
1. 构建前端并启动 daemon（首次构建、或改动了 `web/` 之后都需要先构建前端 —— `go:embed` 要求目录在编译期就存在）：
   ```
   make run          # 等价于 cd web && npm install && npm run build && cd .. && go run ./cmd/tunnel daemon
   # 或者手动分步：
   cd web && npm install && npm run build && cd ..
   go run ./cmd/tunnel daemon
   # 也可以指定地址/状态文件：
   go run ./cmd/tunnel daemon --addr 127.0.0.1:7777 --state ~/.config/ssh-tunnel-manager/tunnels.json
   ```
2. 打开 Web 管理页面：`http://127.0.0.1:7777/`，默认只显示线路列表，点右上角"添加线路"弹窗新建；可在页面上新增、编辑、删除、启停隧道并查看实时状态。编辑运行中的线路会立即按新配置重启，密码留空则保留原密码。
3. 或者用 CLI 操作（默认连接 `127.0.0.1:7777`，可用 `--addr` 或环境变量 `TUNNEL_DAEMON_ADDR` 覆盖）：
   - 本地转发（ssh config）：`tunnel add --name demo --local 127.0.0.1:8080 --ssh hostAlias --ssh-config ~/.ssh/config --forward 10.0.0.1:80`
   - 本地转发（显式指定）：`tunnel add --name demo --local 127.0.0.1:8080 --ssh host:22 --user user --key ~/.ssh/id_rsa --forward 10.0.0.1:80`
   - 远程转发：`tunnel add --name expose --direction remote --ssh host:22 --user user --remote-listen 0.0.0.0:9000 --forward 127.0.0.1:3000`
   - 动态转发 / SOCKS5：`tunnel add --name proxy --direction dynamic --local 127.0.0.1:1080 --ssh host:22 --user user`
   - 列表：`tunnel list`
   - 状态：`tunnel status demo`
   - 停止（保留配置）：`tunnel stop demo`
   - 重新启动：`tunnel start demo`
   - 删除：`tunnel rm demo`

若命令连不上 daemon，会提示：`无法连接到守护进程，请先运行: tunnel daemon`。

## SSH 跳板连接

直接连接、本地/远程转发、SOCKS5，以及 `ProxyJump` 跳板连接均由 Go 原生实现，不依赖系统 `ssh` 可执行文件。例如：

```sshconfig
Host pg-web1
    HostName 10.0.3.179
    User your-user
    IdentityFile ~/.ssh/your-key
    ProxyJump drop

Host drop
    HostName your-jump-host
    Port 2222
    User your-jump-user
    IdentityFile ~/.ssh/your-jump-key
```

线路的 SSH 目标仍填写 `pg-web1`。支持 `ProxyJump user@jump:port`、IPv6 和逗号分隔的多跳，例如 `ProxyJump drop,alice@inner:2222`。第一跳可以使用自己配置的跳板；显式多跳列表决定后续各跳的路径。每跳独立读取同一份 SSH config 中自己的用户、密钥和 `UserKnownHostsFile`，不会继承目标的密码、私钥或跳过 Host Key 校验选项。循环或超过 16 个 SSH 主机的链会被拒绝。每跳建连/认证限时 10 秒；建连失败、取消或最终连接关闭会清理整条原生连接链。

兼容现有的 `ProxyCommand ssh -W %h:%p -q drop`，这种简单写法会自动转换为原生跳板，不启动 `ssh`。识别 `ssh` / `/usr/bin/ssh`、`-W %h:%p`（或 `[%h]:%p`）及可选 `-q`、`-p`、`-l`；不丢弃其他参数。**带其他选项、shell 语法或不同目标的自定义 ProxyCommand 仍通过 `/bin/sh` 执行，依赖其中使用的外部程序。**要完全避免外部命令，请使用 `ProxyJump`。

## SSH 自动认证

- 优先使用线路指定的私钥；未指定时读取该 Host 的全部 `IdentityFile`。都未配置时自动尝试 `~/.ssh/id_ed25519`、`~/.ssh/id_ecdsa`、`~/.ssh/id_rsa`，无需执行 `ssh` 命令。
- 支持通过 `SSH_AUTH_SOCK` 或 SSH config 的 `IdentityAgent` 连接已运行的 ssh-agent；`IdentityAgent none` 禁用 agent。`IdentitiesOnly yes` 仅允许配置/默认身份对应的 agent 密钥，避免尝试无关身份。不向远端转发 agent。
- 加密私钥不弹出交互输入框，需要预先在 agent 中解锁。支持用公钥文件或 `.pub` 配套文件选择 agent 密钥。登录密码与私钥口令不同，不会把登录密码当作私钥口令。
- 仍支持线路填写的登录密码。用户名按线路设置、SSH config 的 `User`、daemon 本机用户名顺序解析。
- daemon 必须能访问密钥文件和 agent socket；作为后台服务启动时，需要正确传入 `SSH_AUTH_SOCK` 或配置 `IdentityAgent`。macOS Keychain、交互式 MFA、硬件安全密钥等 OpenSSH 扩展并非全部兼容。

## Host key 校验
默认使用 `~/.ssh/known_hosts`（或 ssh config 中该 host 的 `UserKnownHostsFile`，或 `--known-hosts` 显式指定）严格校验服务器主机密钥，未收录的主机会直接拒绝连接。首次连接某台新主机时，可加 `--trust-new-host-key`（Web 表单里是"信任新主机密钥"勾选框）自动信任并写入 known_hosts（TOFU）；但一旦主机密钥已被记录，之后密钥发生变化（可能的中间人攻击）无论是否开启该选项都会被拒绝。

原生连接会优先协商该地址和端口在 known_hosts 中已信任的密钥类型，支持哈希主机记录和 IPv6。比如只信任 ED25519 时，即使服务器也提供 ECDSA/RSA，也会优先使用 ED25519；这只调整协商顺序，不放宽密钥内容校验。跳板和目标分别应用自己的配置。

支持 SSH config 的 `HostKeyAlgorithms`，包括完整列表、`+` 追加、`-` 排除和 `^` 前置。完整列表和 `^` 保留显式顺序，默认及 `+`/`-` 列表在允许范围内优先使用已信任类型，不会重新启用被排除的算法。默认使用 Go SSH 的安全算法集合；旧版 SHA-1 `ssh-rsa` 需通过如 `HostKeyAlgorithms +ssh-rsa` 显式开启，已知 RSA 公钥优先使用 RSA-SHA2 签名。`PubkeyAcceptedAlgorithms` 是用户认证的另一项设置，不属于本次主机密钥算法支持。

密钥不匹配时，错误包含服务器实际返回的密钥类型、SHA256 指纹，以及预期指纹对应的 known_hosts 文件和行号；不会自动删除、覆盖已有信任记录。debug 日志也会输出协商算法列表及被接受的主机密钥指纹。

受控内网或临时环境可以显式使用 `--insecure-skip-host-key-check`，或在 Web 编辑表单中勾选“跳过 Host Key 校验”。这会接受任何服务器密钥，包括发生变化的密钥，存在中间人攻击风险，因此默认关闭且不建议用于生产环境。

## 运行与测试
- 桌面版（macOS / Windows / Linux）：`make desktop`；macOS 通用包 `make desktop-universal`。复用现有界面，macOS 提供菜单栏常驻。构建依赖、安装包和退出行为见 [桌面版说明](desktop/README.md)。
- 构建前端（首次 / `web/` 有改动时必需）：`make web`（或 `cd web && npm install && npm run build`）
- 构建：`make build`（先构建前端，再执行 `go build -o tunnel ./cmd/tunnel`），在项目根目录生成包含前端资源的 `tunnel` 可执行文件。可用 `make build BINARY=其他路径` 指定输出位置；前端已构建过、只想编译 Go 侧时可直接执行 `go build -o tunnel ./cmd/tunnel`。
- 运行 daemon：`make run`（或 `go run ./cmd/tunnel daemon`，前提是 `internal/daemon/webui/dist` 已经是最新构建）
- 测试：`make test`（等价于 `go test ./...`；前端目前没有单测）

## Daemon 日志

日志默认同时输出到 stderr 和 `~/.config/ssh-tunnel-manager/logs/daemon.log`，使用 `info` 级别和可读的 `key=value` 文本格式。每条日志包含时间、级别和事件；线路日志带 `tunnel`，SSH 分跳日志带 `hop`、`host`、`address`，转发请求日志带 `connection`、来源和目标。

```bash
# 默认：启停、线路恢复、SSH 建连结果、重连次数/等待时间、请求失败
./tunnel daemon

# 排查：增加认证准备、传输/握手阶段、逐连接建立/结束、耗时和流量
./tunnel daemon --log-level debug

# 机器可读，每行一个 JSON 对象，便于日志采集
./tunnel daemon --log-level debug --log-format json
```

`--log-level` 支持 `debug`、`info`、`warn`、`error`，`--log-format` 支持 `text`、`json`。成功的页面 GET 轮询只在 debug 记录；配置修改、启动/停止等 API 操作在 info 记录，4xx/5xx 分别记为 warn/error。SOCKS5 单个目标失败记录 warn，不改变整条线路状态。

日志不输出密码、私钥内容、请求/响应正文、HTTP 头或查询参数；错误中的已知密码和 PEM 私钥块会脱敏。自定义 ProxyCommand 的命令正文和 stderr 不写入日志，避免泄露其中可能携带的凭据。日志仍包含服务器地址、用户名、文件路径和访问目标，应妥善保管。

SOCKS5 连接历史独立写入 `logs/connections.jsonl`，不受 `log_level` 过滤。每行一个 JSON 对象，包含 `tunnel`、`id`、`source`、`target`、`state`、`started_at`、`ended_at`、`age_seconds`、`bytes_in`、`bytes_out`、`error`。日志记录访问元数据，不记录业务内容；同名线路共用历史，删除线路不会删除日志。正常停止会等待活动连接结束并写入；强制终止时尚未结束的连接没有完整记录。文件追加不逐条 fsync，系统崩溃/断电仍可能丢失近期写入。

两种日志分别按大小轮转，默认单文件 20 MiB、各保留 5 个轮转文件（`.1` 最新，另有当前文件），最旧文件会被覆盖。页面运行期间缓存最近 500 条；重启后只恢复仍保留在文件中的记录，因此日志轮转后可恢复的条数可能少于 500。文件权限为 `0600`。写入失败不会中断转发，但失败期间未落盘的记录重启后无法恢复；请检查页面告警与 stderr。不要让多个 daemon 共用同一组日志文件。后台部署仍可由 systemd/launchd 收集 stderr。

## YAML 配置与页面设置

页面右上角“配置”可编辑 daemon 监听地址、线路状态文件、日志级别/格式、日志路径和轮转策略。保存会以原子替换方式写入 YAML 文件，**重启 daemon 后生效**，不会自动重启或中断线路。弹窗显示配置路径、当前运行配置，以及与文件配置的差异。页面保存会重新生成配置文件，不保留手写注释。

默认读取 `~/.config/ssh-tunnel-manager/config.yaml`；设置 `XDG_CONFIG_HOME` 时，配置、默认状态和日志路径均位于 `$XDG_CONFIG_HOME/ssh-tunnel-manager/`。默认配置文件不存在时使用内置默认值，首次页面保存时创建。也可指定现有文件：

```bash
./tunnel daemon --config /path/to/config.yaml
```

示例（相对路径以 YAML 文件所在目录为基准，支持 `~/`）：

```yaml
addr: 127.0.0.1:7777
state_file: tunnels.json
log_level: info
log_format: text
log_file: logs/daemon.log
connection_log: logs/connections.jsonl
log_max_size_mb: 20
log_max_backups: 5
```

配置允许省略字段，使用内置默认值；显式指定不存在的 `--config`、未知字段、重复字段或非法值会报错。优先级为**显式命令行参数 > YAML > 默认值**，`--addr`、`--state`、`--log-level`、`--log-format` 可覆盖文件配置。`log_file: ""` 表示只向 stderr 输出运行日志；连接日志路径不可为空。更改路径不会迁移旧状态或旧日志，需自行复制。线路定义和密码仍按原来的方式保存在 `tunnels.json`，不存入 YAML。

配置接口：`GET /api/config` 读取保存/生效配置，`PUT /api/config` 保存完整 JSON 配置对象到 YAML；保存失败不改变当前配置。API 没有登录认证，建议继续仅绑定回环地址，不要直接暴露到公网。

## 待办与改进
- 带宽/速率限制
- Web UI 增加鉴权（当前默认仅监听 127.0.0.1，暴露到公网前需自行加认证/反向代理）
- 更完善的集成/端到端测试（真实 SSH 服务器）
- 前端目前没有测试（组件/交互）
