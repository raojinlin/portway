# 桌面版

基于 Wails 2，复用现有 React 界面和 Go 转发引擎。用户不需要安装 Go、Node.js 或单独运行 daemon。macOS 使用系统 WebKit，Windows 使用 WebView2，Linux 使用 WebKitGTK。

## 行为

- macOS：菜单栏常驻，显示运行中/异常线路数量，可打开主窗口、打开配置目录和退出。关闭窗口仅隐藏，转发继续；从菜单选择退出或按 Cmd+Q 才停止应用。
- 常驻菜单每 2 秒更新线路状态与收发速率，包含已停止的线路；macOS 菜单在每条线路名称前以原生绘制的绿、黄、红、灰圆点分别表示运行中、连接中、异常和停止状态，顶部显示状态汇总、总速率和当前连接数。展开单条线路可查看累计流量、连接数、转发地址和最近错误。速率按两次采样的实际时间间隔计算，首个样本或线路重启后先显示“采样中”；累计流量沿用页面的本次启动统计口径，不是历史总量。菜单只展示信息，不会自动启停线路。
- 左上角应用菜单显式命名为 Portway；右上角常驻图标在原生启动阶段创建，不等待网页或外部字体加载。初始化结果写入配置的运行日志（默认 `logs/daemon.log`，搜索 `desktop native menus`）；创建失败会弹出错误并退出，不会静默留下无法唤回的后台应用。日志中的创建成功不代表图标未被刘海区域或第三方菜单栏管理工具遮挡。
- Windows：任务栏通知区域常驻托盘，左键打开主窗口，右键查看与 macOS 相同的线路状态、速率和连接详情，或打开配置目录、退出应用。关闭窗口仅隐藏，转发继续；从托盘选择“退出 Portway”才停止应用。图标可能收纳在任务栏的“显示隐藏的图标”中。资源管理器重启后会重新注册图标；不额外依赖托盘软件。
- Windows 托盘每 2 秒刷新数据，已打开菜单中的状态和流量也会更新；新增/删除线路和详情条目数量的变化在下次展开菜单时完整重建，避免刷新打断当前菜单操作。
- Linux：独立桌面窗口，关闭窗口即退出并停止应用托管的转发，目前不创建托盘图标。
- 页面主题可选择“跟随系统 / 浅色 / 深色”，首次使用默认跟随系统，系统主题变化时自动切换；已有浅色/深色偏好保留，可从顶部主题菜单改为跟随系统。标题栏随页面主题同步：macOS 使用与页面相同的背景；Windows 配置同色标题栏（自定义颜色取决于系统版本支持）；Linux 原生标题栏外观由桌面环境决定。
- 退出等待连接清理、历史日志写入完成；线路启用状态保留，下次启动恢复。
- 重复打开桌面程序会唤起已有窗口。共享配置、状态、日志的桌面端和新版 CLI daemon 使用操作系统文件锁互斥，不会同时写入，也不会自动终止另一个进程。旧版 daemon 不支持文件锁，使用桌面版前必须先手动停止。
- 桌面后端在应用进程内运行，不启动子进程，不监听 HTTP 端口。页面 API 通过 Wails 的内置资源服务直接转发给 Go；CLI 的 `list/start/stop` 不连接桌面后端，需要使用桌面界面操作。
- YAML、线路定义和历史日志沿用原有路径。macOS/Linux 默认 `~/.config/ssh-tunnel-manager/`；Windows 默认 `%USERPROFILE%\.config\ssh-tunnel-manager\`。支持 `XDG_CONFIG_HOME`。配置弹窗保存后退出并重开应用生效；监听地址仅用于独立 HTTP daemon，在桌面模式下不生效。
- macOS/Linux 创建私有权限的配置与日志文件；Windows 访问权限由目录继承的 ACL 控制，Unix `0600` 权限位不能代表 Windows 的访问控制。请将配置保存在自己的用户目录，不要放入公共共享目录。
- 普通 SSH、ProxyJump、识别的 `ProxyCommand ssh -W` 仍然不依赖系统 ssh。自定义 ProxyCommand 继续依赖外部命令；Windows 不提供 `/bin/sh`，不要直接使用 Unix 自定义代理命令。当前 ssh-agent 通过 `SSH_AUTH_SOCK` 的 Unix socket 接入，不支持 Windows OpenSSH 的命名管道 agent；Windows 可使用未加密密钥或密码。

## 构建环境

构建者需要 Go 1.25+、Node.js 20+ / npm；最终用户不需要这些工具。桌面依赖放在独立 `desktop/go.mod` 中，CLI 保持原来的依赖和构建入口。

- macOS：macOS 12+，Xcode Command Line Tools。可直接生成 `.app` 和 ZIP，支持 Intel / Apple Silicon 通用包，不需要安装 Wails CLI。
- Windows：Windows 10/11。安装 Wails CLI；生成安装向导还需要 NSIS。打包后的程序内嵌 WebView2 引导安装器，缺少运行时时会提示安装，安装运行时需要联网，并非完整离线运行时包。
- Linux：目标构建环境为 Ubuntu 24.04 / 同等 GTK3 + WebKitGTK 4.1 环境。需要 `build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev`，安装 Wails CLI。`.deb` 由包管理器安装运行依赖；其他发行版可使用 tar.gz，但需要自行安装兼容运行库。

Windows/Linux 构建工具安装一次即可：

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
```

脚本优先使用 `$GOPATH/bin/wails`，也可用 `WAILS` 环境变量指定可执行文件路径。

### 通用入口

有 make 的环境：

```sh
make desktop
```

没有 make（例如 Windows PowerShell）：

```sh
npm --prefix web ci
npm --prefix web run build
node scripts/desktop.mjs
```

脚本只打包当前系统，Windows/Linux 应在相应系统构建。macOS 可在同一台机器上构建两种架构：

```sh
make desktop-universal
make desktop-dmg
```

Windows 安装向导（NSIS 的 `makensis` 需在 PATH 中）：

```sh
node scripts/desktop.mjs --installer
```

### 输出

- macOS 应用：`desktop/build/bin/Portway.app`。旧的 `SSH Tunnel Manager.app` 不会自动删除；更新后请退出旧进程再打开 `Portway.app`。
- 可分发文件：`dist/desktop/`；macOS 为 ZIP（可选 DMG），Windows 为 `.exe`（可选当前用户安装器），Linux 为 `.tar.gz` 和 `.deb`（需 `dpkg-deb`）。
- Linux tar.gz 包含 `usr/` 目录结构；可解压后运行 `usr/bin/ssh-tunnel-manager`。Ubuntu 推荐 `sudo apt install ./ssh-tunnel-manager-linux-amd64.deb`，让包管理器处理依赖。
- 现有 `make build` 仍只生成 CLI 的 `tunnel`，不会被改成桌面程序。

构建不会安装应用、启动转发或修改真实 SSH 配置。macOS 包做本机 ad-hoc 签名，不包含 Developer ID 公证；公开分发仍需自行签名、公证后重新制作归档。Windows 未做 Authenticode 签名，SmartScreen 可能提示未知发布者。

## CI

`.github/workflows/desktop.yml` 使用原生 macOS、Windows、Ubuntu runner 测试并生成安装包，上传为 Actions artifacts；可手动运行或推送 `v*` 标签触发。它不会自动发布 GitHub Release，也没有签名凭据。

本地可以验证后端与桌面传输（不会启动 GUI）：

```sh
go test ./...
cd desktop
go test -tags production,webkit2_41 ./...
go test ssh-tunnel-manager/internal/daemon ssh-tunnel-manager/internal/tunnel
```

完整验收还需在三端实际打开应用，检查创建线路、SSH 连接、配置保存、历史恢复；macOS / Windows 额外检查关闭窗口后转发继续、菜单栏/托盘重新打开及退出后端口释放。Windows 还需检查键盘访问托盘菜单、资源管理器重启后的图标恢复和隐藏图标区域。Linux 原生窗口需要图形会话；无图形环境的 Go 测试不等于窗口验收。
