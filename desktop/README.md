# Portway 桌面版

将 SSH 转发引擎和管理界面放在一个应用中。无需单独启动 daemon，也无需安装 Go、Node.js 或系统 `ssh` 命令即可使用普通 SSH 和 `ProxyJump`。

线路配置、SSH 认证和日志说明见 [项目 README](../README.md)。本页介绍安装使用、平台差异与桌面打包。

## 安装与启动

使用构建产物时，按平台选择对应包；从源码生成这些文件的方法见 [开发与构建](#开发与构建)。

| 平台 | 安装方式 | 运行依赖 |
| --- | --- | --- |
| macOS 12+ | 打开 DMG 或解压 ZIP，将 `Portway.app` 放入“应用程序” | 系统 WebKit；通用包兼容 Intel / Apple Silicon |
| Windows 10/11 | 运行 `-installer.exe`，或直接使用独立 `.exe` | WebView2；缺少时引导安装，需要联网 |
| Linux | Ubuntu 推荐安装 `.deb`；其他发行版可使用 `.tar.gz` | GTK3、WebKitGTK 4.1 |

Ubuntu 安装示例：

```bash
sudo apt install ./portway-linux-amd64.deb
```

tar.gz 解压后可运行 `usr/bin/portway`，但需自行安装运行依赖。当前 Linux 构建以 Ubuntu 24.04 为目标，不保证兼容所有发行版。

本地与 CI 构建尚未提供公开分发签名：macOS 使用 ad-hoc 签名，未做 Developer ID 公证；Windows 未做 Authenticode 签名。系统可能显示来源或信誉警告，请先确认文件来源可信。

首次启动后，点击“新建线路”即可配置转发。更新应用前，请通过菜单中的“退出 Portway”结束旧进程，再打开新版本；仅关闭 macOS / Windows 窗口不会退出。

## 开机自启

在“配置 → 开机自启”开启。这里的自启是**当前用户登录桌面后启动**，不是登录前运行的系统服务，不需要管理员权限。开关立即保存，无需点击“保存到配置文件”或重启应用；不会修改 YAML。

- macOS：请先将 `Portway.app` 放到固定位置（建议“应用程序”目录），从该位置打开后开启。启动项保存在 `~/Library/LaunchAgents/io.github.ssh-tunnel-manager.desktop.autostart.plist`。
- Windows：写入当前用户的 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`，值名为 `Portway Desktop`。
- Linux：写入 `$XDG_CONFIG_HOME/autostart/portway.desktop`（默认 `~/.config/autostart/portway.desktop`），需要桌面环境支持 XDG Autostart。

自启时 macOS / Windows 隐藏主窗口，通过菜单栏 / 托盘打开；Linux 没有托盘，正常显示主窗口。启用的线路随应用启动恢复。手动启动应用仍显示窗口。

移动应用后，从新位置打开配置，点击“更新启动项”。卸载前建议关闭此开关；关闭只移除 Portway 的启动项，不停止当前应用或线路。开关反映 Portway 管理的启动项是否存在；若另外在操作系统中禁用了启动权限，也需要在系统设置中重新允许。

## 窗口与后台运行

| 操作 | macOS | Windows | Linux |
| --- | --- | --- | --- |
| 关闭窗口 | 隐藏窗口，转发继续 | 隐藏窗口，转发继续 | 退出应用，停止转发 |
| 返回主窗口 | 菜单栏“打开 Portway” | 左键托盘图标或托盘菜单 | 通过桌面窗口管理器 |
| 完全退出 | 菜单“退出 Portway”或 `Cmd+Q` | 托盘“退出 Portway” | 关闭窗口 |
| 常驻入口 | 系统菜单栏 | 任务栏通知区域 | 暂不提供 |

正常退出会等待连接清理和历史记录写入。线路启用状态会保留，下次启动自动恢复；重复打开应用会唤回已有窗口。

### 菜单栏与托盘

macOS 菜单栏和 Windows 托盘可查看各条线路的状态、转发地址、累计流量、连接数和最近错误，数据每 2 秒更新。顶层菜单只保留紧凑的“MCP”入口；子菜单显示运行状态，并可直接启动或停止 MCP。macOS 可在状态项上悬停查看监听地址、授权方式和 Agent 权限；菜单不显示 MCP Token 或 OAuth 授权码。

macOS 线路名前同时显示状态圆点和服务图标。服务图标沿用线路的自动识别或手动选择，停止后仍保留；数据库图标带 MY / PG / R / M 标记便于区分。悬停可查看服务名称，修改图标后自动同步到菜单栏。

菜单提供：

- **打开 Portway**：显示主窗口。
- **MCP 子菜单**：查看简短状态，直接启动/停止 MCP，或显示主窗口并打开 MCP 设置。
- **打开日志**：显示主窗口并进入运行日志页。
- **打开配置目录**：定位本机配置与日志文件。
- **退出 Portway**：停止应用及其托管的转发。

每条线路的子菜单提供启停快捷操作：未启用时显示“启动线路”，已启用时显示“停止线路”（包括连接中和异常重连）。执行期间显示“处理中”并禁用重复操作；操作会保存启用状态，完成后立即刷新菜单，无需先打开主窗口。

macOS 的启停操作以播放 / 停止图标配合文字呈现，忙碌时显示沙漏和“处理中”。复制代理地址、复制代理命令也分别配有复制和终端图标。线路列表中的速率统一右对齐，较长的线路名称会省略显示。

macOS 线路子菜单的“查看连接”会唤回主窗口并直接打开该线路的连接详情。SOCKS5 提供当前连接和历史连接页签；线路停止后仍可通过此入口查看历史。

SOCKS5 线路的详情子菜单还提供“复制代理地址”和“复制代理命令”，停止状态下隐藏这两个入口。macOS 复制 shell 命令，Windows 复制 PowerShell 命令，同时设置 `all_proxy`、`http_proxy`、`https_proxy`。地址使用 `socks5h://`，通配监听地址会转换为本机回环地址，不会修改系统代理。

macOS 图标右侧分两行显示所有线路的合计速率：上方上传、下方下载，首次采样显示 `--`。这是 Portway 的转发流量，不是系统总网速。下拉菜单直接显示线路列表，不设顶部汇总区域。

### 语言与主题

窗口右上角依次提供语言、主题、MCP 和系统配置入口。语言可选自动检测、简体中文或 English，原生菜单同步切换；主题可选跟随系统、浅色或深色。偏好保存在本机 WebView 中。

macOS 标题栏与页面背景同步；Windows 标题栏颜色取决于系统支持；Linux 原生标题栏由桌面环境控制。原始 SSH 错误和日志不参与翻译。

## 配置与兼容性

桌面版与 CLI 共用配置格式和默认数据目录：

- macOS / Linux：`~/.config/ssh-tunnel-manager/`
- Windows：`%USERPROFILE%\.config\ssh-tunnel-manager\`
- 自定义目录：`$XDG_CONFIG_HOME/ssh-tunnel-manager/`

“系统配置”和“MCP”弹窗都保存到 `config.yaml`。MCP 弹窗内置 Codex、Claude Code 和通用 JSON 连接示例，并随当前 Token / OAuth 方式更新。MCP 配置立即生效，系统配置需退出并重开应用。线路单独保存在 `tunnels.json`，运行日志与连接历史位于 `logs/`。完整字段和保留策略见 [配置与数据](../README.md#配置与数据)。

需要注意的边界：

- 桌面后端在应用进程内运行，默认不监听管理页面与 REST API 的 HTTP 端口。YAML 的 `addr` 仅用于 CLI daemon；`portway list/start/stop` 不会连接桌面后端。启用 MCP 后会单独监听配置的本机回环端口，可使用静态 Token、OAuth 2.1 或同时启用两者。桌面版下载内置 Skill 时会弹出系统保存对话框；也可把独立 MCP 弹窗中的一句话指令交给 Agent，由它通过 `/skills/portway.zip` 自行下载安装。
- 桌面端和新版 CLI daemon 通过文件锁避免同时占用共享数据。切换使用方式前应退出另一端；旧版 daemon 可能不支持该锁。
- 普通 SSH 与 `ProxyJump` 不依赖外部命令，但自定义 `ProxyCommand` 仍可能依赖 shell 和其他程序。Windows 不提供 `/bin/sh`，建议使用原生 `ProxyJump`。
- agent 目前通过 Unix socket 接入，不支持 Windows OpenSSH 的命名管道 agent。Windows 可使用未加密私钥或密码。
- 配置包含敏感信息，应保存在私人目录。macOS / Linux 使用私有文件权限，Windows 的访问权限由目录 ACL 控制。

## 常见问题

### 看不到 macOS 菜单栏图标

先确认已退出旧版并打开新构建的 `Portway.app`，再检查刘海区域和第三方菜单栏管理工具是否隐藏图标。运行日志中搜索 `desktop native menus` 可查看初始化结果；创建成功不代表图标当前没有被遮挡。

### 看不到 Windows 托盘图标

检查任务栏“显示隐藏的图标”区域。左键图标打开窗口，右键展开菜单；资源管理器重启后应用会重新注册图标。

### 启动时提示配置正在使用

检查是否已有桌面实例或 CLI daemon 正在使用同一组数据。先正常退出对应进程，不要同时启动两份应用读写同一目录。

## 开发与构建

以下命令均从仓库根目录执行。构建需要 Go 1.25+、Node.js 20+ 和 npm，桌面 Go 依赖位于独立的 `desktop/go.mod` 中。

### 平台工具

| 构建平台 | 额外依赖 |
| --- | --- |
| macOS | Xcode Command Line Tools；不需要 Wails CLI |
| Windows | Wails CLI；生成安装器还需 NSIS，`makensis` 在 PATH 中 |
| Ubuntu 24.04 | Wails CLI、`build-essential`、`pkg-config`、`libgtk-3-dev`、`libwebkit2gtk-4.1-dev`；生成 `.deb` 需 `dpkg-deb` |

Windows / Linux 安装 Wails CLI：

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
```

构建脚本优先查找 `$GOPATH/bin/wails`，也可通过 `WAILS` 指定路径。

### 构建命令

```bash
# 构建前端并打包当前平台
make desktop

# macOS：Intel + Apple Silicon 通用 ZIP
make desktop-universal

# macOS：通用 ZIP 和 DMG
make desktop-dmg
```

没有 make 时（例如 Windows PowerShell）：

```bash
npm --prefix web ci
npm --prefix web run build
node scripts/desktop.mjs
```

Windows 前端构建完成后，使用 `node scripts/desktop.mjs --installer` 额外生成当前用户安装器。脚本只打包当前操作系统，Windows / Linux 应在对应平台构建。

`make build` 生成的是 CLI `portway`，不是桌面应用。桌面构建不会自动安装应用、启动转发或修改 SSH 配置。

### 构建产物

本地可执行应用位于 `desktop/build/bin/`，分发包位于 `dist/desktop/`：

| 平台 | 应用 | 分发格式 |
| --- | --- | --- |
| macOS | `Portway.app` | `.zip`，可选 `.dmg` |
| Windows | `portway.exe` | 独立 `.exe`，可选 `-installer.exe` |
| Linux | `portway` | `.tar.gz`；有 `dpkg-deb` 时同时生成 `.deb` |

包名使用 `portway-<系统>-<架构>` 前缀，macOS 通用包架构为 `universal`；应用内部执行文件为 `Portway.app/Contents/MacOS/portway`。公开分发前仍需单独完成签名、公证等发布步骤。

配置目录、macOS bundle ID、Linux 软件包 ID 和桌面入口 ID 保留原值，避免已有安装和配置失联。Windows / Linux 旧版自启路径含旧文件名，更新后请在应用配置中点击“更新启动项”。旧分发包不会自动删除，请使用新前缀产物。桌面版和 CLI 均名为 `portway`，请按使用方式选择，不要覆盖安装到同一个路径。

### 测试

先构建前端资源，再运行：

```bash
npm --prefix web test
node --test scripts/tests/*.test.mjs
go test ./...
cd desktop
go test -tags production,webkit2_41 ./...
```

自动化测试不代替实际窗口验收。发布前应检查线路创建、真实 SSH 连接、配置保存、历史恢复，以及 macOS / Windows 的关闭窗口继续转发、菜单唤回和退出释放端口。

macOS 速率图标还可单独做离屏绘图检查，不启动应用或 SSH 线路（在仓库根目录执行）：

```bash
clang -fobjc-arc -framework Cocoa -framework CoreText desktop/platform/tests/status_image.m -o /tmp/portway-status-image-test
/tmp/portway-status-image-test /tmp/portway-status-image.png
```

线路子菜单的启停、忙碌状态与复制入口可做原生离屏检查（不启动应用或连接 SSH）：

```bash
clang -fobjc-arc -framework Cocoa -framework CoreText desktop/platform/tests/menu_actions.m -o /tmp/portway-menu-actions-test
/tmp/portway-menu-actions-test
```

### CI 与发布

[Desktop and CLI Packages](../.github/workflows/desktop.yml) 工作流在相关路径的 Pull Request、推送 `v*` 标签或手动触发时运行，使用 macOS、Windows 和 Ubuntu runner 测试并打包。

产物上传到 GitHub Actions 的 `portway-*` artifacts，包含桌面包与 CLI；macOS 为通用包，Windows / Linux 为 amd64。**目前不会自动创建 GitHub Release，也未配置分发签名凭据。**
