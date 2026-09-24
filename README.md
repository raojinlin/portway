# Portway

管理 SSH 隧道的桌面应用与命令行工具。支持本地转发、远程转发和 SOCKS5 代理，可集中管理多条线路、查看流量与连接、排查运行日志，并在启动时恢复已启用的线路。

桌面版支持 macOS、Windows 和 Linux；也可以只运行一个内嵌 Web 界面的 daemon，通过浏览器或 CLI 管理。普通 SSH 连接和 `ProxyJump` 由 Go 原生实现，不需要安装系统 `ssh` 命令。

## 选择使用方式

| 方式 | 适合场景 | 如何管理 |
| --- | --- | --- |
| 桌面应用 | 日常使用、后台常驻 | 应用窗口；macOS 菜单栏 / Windows 托盘 |
| Daemon + Web / CLI | 无桌面环境、脚本操作 | 浏览器访问管理页，或使用 `portway` 命令 |

桌面安装、平台差异和打包方法见 [桌面版指南](desktop/README.md)。两种方式使用相同的数据格式，但 **CLI 不能控制桌面应用中的线路**；切换方式前请先退出当前进程，避免同时占用配置和端口。

## 主要功能

- **线路管理**：创建、编辑、启停和删除线路，支持 SSH config 别名、多级跳板、保活与断线重连。
- **服务图标**：按目标端口自动识别 SSH、MySQL、PostgreSQL、HTTP/HTTPS、Redis、MongoDB 和 RDP，SOCKS5 使用代理图标。新增或编辑线路时可手动选择；非标准端口可覆盖识别结果，仅修改图标不会重启线路。
- **连接与流量**：查看线路累计收发量、活动连接，以及每个连接的来源、目标、持续时间和流量。
- **日志与历史**：筛选运行日志和当前连接，查看落盘保存的 SOCKS5 历史请求及失败原因。
- **代理快捷操作**：复制 SOCKS5 地址或 Bash/Zsh、PowerShell 代理命令。
- **桌面常驻**：macOS 菜单栏、Windows 托盘展示线路状态与流量，可直接启停线路；macOS 图标旁显示上传 / 下载速率。
- **界面偏好**：中英文切换，自动检测语言，浅色 / 深色 / 跟随系统主题。

## 快速开始

以下命令从仓库根目录执行。源码构建推荐 Go 1.25+、Node.js 20+ 和 npm；使用已构建的程序不需要这些工具。

### 启动 Web 管理界面

```bash
make build
./portway daemon
```

`make build` 会先构建前端，再在项目根目录生成 `portway`。页面资源已嵌入二进制，不需要另行部署静态文件或运行 Node.js。

打开 <http://127.0.0.1:7777/>，点击“新建线路”，填写监听地址、SSH 服务和目标地址。SSH 服务既可以填写 `host:port`，也可以填写 `~/.ssh/config` 中的 Host 别名。默认严格校验主机密钥，首次连接前请阅读 [SSH 认证与安全](#ssh-认证与安全)。

daemon 在前台运行，按 `Ctrl+C` 退出。停止后保留线路定义，下次启动恢复已启用的线路。编辑正在运行的线路会按新配置重启该线路，并中断其现有连接。

没有 make 时，可分步构建：

```bash
npm --prefix web ci
npm --prefix web run build
go build -o portway ./cmd/tunnel
```

### 使用 CLI

在另一个终端执行。下例假设 SSH config 已配置 `my-server`，并已信任它的主机密钥：

```bash
./portway add --name web --ssh my-server \
  --local 127.0.0.1:8080 --forward 127.0.0.1:80

./portway list
./portway status web
./portway stop web
./portway start web
./portway rm web
```

`stop` 保留配置，`rm` 删除线路。CLI 默认连接 `127.0.0.1:7777`；连接其他 daemon 时使用 `--addr`，或设置 `TUNNEL_DAEMON_ADDR`。例如 `./portway status --addr 127.0.0.1:8888 web`。查看完整参数可运行 `./portway add --help`。

## 三种转发模式

地址的含义取决于转发方向，尤其要区分目标由哪台机器访问。

| 类型 | 监听位置 | 目标由谁访问 | 常见用途 |
| --- | --- | --- | --- |
| 本地转发 `local`（`ssh -L`） | 运行 Portway 的机器 | SSH 服务器 | 访问远端数据库、内网服务 |
| 远程转发 `remote`（`ssh -R`） | SSH 服务器 | 运行 Portway 的机器 | 将本地服务提供给远端访问 |
| SOCKS5 `dynamic`（`ssh -D`） | 运行 Portway 的机器 | SSH 服务器；目标由请求指定 | 按需代理多个目标 |

```bash
# 本地转发：本机 15432 → SSH 服务器可达的数据库
./portway add --name database --ssh my-server \
  --local 127.0.0.1:15432 --forward 10.0.0.10:5432

# 远程转发：SSH 服务器 9000 → 本机 3000
./portway add --name preview --direction remote --ssh my-server \
  --remote-listen 127.0.0.1:9000 --forward 127.0.0.1:3000

# SOCKS5：在本机 1080 提供代理，无需固定目标
./portway add --name proxy --direction dynamic --ssh my-server \
  --local 127.0.0.1:1080
```

远程监听是否允许外部访问，还取决于 SSH 服务器的转发策略和 `GatewayPorts` 配置。绑定 `0.0.0.0` 前，请确认暴露范围和防火墙规则。

SOCKS5 支持无认证的 TCP CONNECT，不支持 UDP 转发，建议仅监听回环地址。页面可复制 `socks5h://` 地址或代理命令，同时设置 `all_proxy`、`http_proxy`、`https_proxy`。命令只影响当前终端中支持这些变量及 SOCKS5 的程序，不会设置系统代理。复制结果默认用于运行 Portway 的机器，远程浏览器用户需自行确认地址是否可达。

## SSH 认证与安全

### 复用 SSH config 和跳板

默认读取 `~/.ssh/config`，也可通过 `--ssh-config` 指定文件。下面的配置允许线路直接填写 `app-server`，Portway 会经过 `jump` 连接目标：

```sshconfig
Host jump
    HostName jump.example.com
    Port 2222
    User alice
    IdentityFile ~/.ssh/id_ed25519

Host app-server
    HostName 10.0.0.20
    User alice
    IdentityFile ~/.ssh/id_ed25519
    ProxyJump jump
```

支持 `ProxyJump user@host:port` 和逗号分隔的多跳；每跳使用自己的认证与主机密钥配置。简单的 `ProxyCommand ssh -W %h:%p -q jump` 也可转换为原生跳板。其他自定义 `ProxyCommand` 仍依赖外部命令和 shell；需要完全避免外部程序时，请使用 `ProxyJump`。

### 登录凭据

优先使用线路指定的私钥，否则读取 SSH config 的 `IdentityFile`；未配置时尝试默认的 `id_ed25519`、`id_ecdsa`、`id_rsa`。也支持登录密码，以及通过 `SSH_AUTH_SOCK` / `IdentityAgent` 连接现有 ssh-agent。

加密私钥需要预先在 agent 中解锁，应用不会弹出私钥口令输入框。后台运行时应确认进程能访问密钥文件和 agent socket。并非所有 OpenSSH 扩展都受支持，例如交互式 MFA、硬件安全密钥和 macOS Keychain 集成。

### 主机密钥

默认严格校验 `known_hosts`。可使用 SSH config 的 `UserKnownHostsFile` 或 `--known-hosts` 指定文件。

- **首次信任**：`--trust-new-host-key`（界面中的“信任新主机密钥”）允许记录尚未收录的主机，但不会接受已记录密钥的变化。
- **密钥不匹配**：先通过可信渠道核对服务器指纹，再更新对应的 known_hosts 记录。错误中会给出实际指纹和已有记录的位置，应用不会自动覆盖。
- **跳过校验**：`--insecure-skip-host-key-check` 会接受任意服务器密钥，存在中间人攻击风险，仅适合明确知晓风险的临时环境。

支持 SSH config 的 `HostKeyAlgorithms` 及其 `+`、`-`、`^` 修饰符。默认优先使用允许范围内已信任的密钥类型；旧版 SHA-1 `ssh-rsa` 需要显式开启。

**Web / HTTP API 没有登录认证。** 默认仅监听 `127.0.0.1`，不要直接暴露到公网。线路密码保存在本机状态文件中；请保护配置目录，不要将其提交到代码仓库或放入共享目录。

## 配置与数据

默认数据目录为 `~/.config/ssh-tunnel-manager/`；Windows 对应 `%USERPROFILE%\.config\ssh-tunnel-manager\`。设置 `XDG_CONFIG_HOME` 后，使用其下的 `ssh-tunnel-manager` 目录。

| 文件 | 内容 |
| --- | --- |
| `config.yaml` | daemon 地址、日志与存储设置 |
| `tunnels.json` | 线路定义、凭据与启用状态 |
| `logs/daemon.log` | 运行日志 |
| `logs/connections.jsonl` | SOCKS5 已结束请求的历史记录 |

页面右上角“配置”可编辑并保存 YAML。保存后需重启 daemon；桌面版需退出并重新打开。保存不会立即中断线路，但会重写配置文件，手写注释不会保留。

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

相对路径以 YAML 所在目录为基准，也支持 `~/`。默认文件不存在时使用内置默认值，首次页面保存时创建；使用 `./portway daemon --config /path/to/config.yaml` 时，指定文件必须存在。

配置优先级为 **显式命令行参数 > YAML > 默认值**。daemon 可用 `--addr`、`--state`、`--log-level`、`--log-format` 覆盖文件设置。`log_file: ""` 表示运行日志仅输出到 stderr；连接历史日志路径不可为空。更改文件路径不会迁移旧数据。

## 日志与排障

“日志与连接”页面集中提供运行日志、当前连接和 SOCKS5 历史，支持线路、级别 / 状态和关键词筛选，可暂停自动刷新。点击线路的连接数，也可查看该线路的连接详情。

运行日志默认使用 `info` 级别，同时写入文件和 stderr。排查 SSH 认证、跳板和连接过程时可启用 debug：

```bash
./portway daemon --log-level debug
./portway daemon --log-level debug --log-format json
```

日志和统计的保留范围：

- 线路累计流量按本次线路运行统计，不是历史总量；菜单栏速率是隧道业务流量，不是系统总网速。
- SOCKS5 请求结束时写入独立 JSONL 文件，不受日志级别过滤。每条线路缓存最近 500 条，重启时从保留的日志恢复；删除线路不会删除历史日志。
- 两类日志分别按大小轮转，默认每个文件 20 MiB、各保留 5 个备份。日志页面每次最多扫描最近 2 MiB，显示最新 500 条匹配记录，不是无限期历史检索。
- 强制终止、断电或写入失败可能导致近期记录丢失。日志包含服务器、用户和访问目标等元数据，虽不记录业务内容，也应妥善保管。

| 现象 | 优先检查 |
| --- | --- |
| `no ssh auth provided` | 密钥路径、`IdentityFile`、agent 是否可访问；加密私钥是否已解锁 |
| `knownhosts: key mismatch` | 实际主机、端口和服务器指纹，不要直接关闭校验 |
| SSH `i/o timeout` | 网络可达性、端口、防火墙，以及 SSH config 的跳板设置 |
| SOCKS5 某个目标超时 | 目标是否能从 SSH 服务器访问；单个请求失败不代表线路异常 |
| CLI 无法连接 daemon | daemon 是否运行、地址是否一致；桌面版不提供 CLI 接入 |

## 开发

### 常用命令

| 命令 | 用途 |
| --- | --- |
| `make build` | 构建前端和 CLI，输出 `./portway`；可用 `BINARY=路径` 自定义 |
| `make run` | 构建前端并以前台方式启动 daemon |
| `make web` | 仅构建前端资源 |
| `make desktop` | 构建当前平台的桌面应用 |
| `make test` | 运行根 Go 模块测试 |
| `npm --prefix web test` | 运行前端测试 |

### 代码结构

```text
cmd/tunnel/          CLI 入口
internal/tunnel/     SSH、跳板、转发与连接统计
internal/daemon/     HTTP API、配置、日志与服务生命周期
internal/store/      线路持久化
internal/apiclient/  CLI 的 HTTP 客户端
web/                React + TypeScript 管理界面
desktop/            Wails 应用与原生菜单（独立 Go 模块）
scripts/            CLI 与桌面打包脚本
```

前端构建产物位于 `internal/daemon/webui/dist`，通过 `go:embed` 嵌入 Go 程序。首次编译或修改前端后，必须先构建前端。

页面翻译使用 `useI18n().tr()`，中文消息作为键，英文位于 `web/src/locales/en.json`；原生菜单翻译位于 `desktop/platform/tray.go`。原始 SSH 错误、日志和 CLI 输出不翻译。

HTTP API 的主要入口为 `/api/tunnels`、`/api/config`、`/api/logs` 和 `/api/connections`，路由见 [handlers.go](internal/daemon/handlers.go)。桌面测试、平台依赖和 CI 产物见 [桌面版指南](desktop/README.md#开发与构建)。
