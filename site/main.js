const english = document.documentElement?.lang === 'en'
const translated = (zh, en) => english ? en : zh

const modes = {
  local: {
    kicker: 'LOCAL FORWARDING', number: '01', title: '从本机连接内网数据库',
    description: '本机数据库客户端连接 127.0.0.1:15432，由 SSH 服务器转发到 db:5432。',
    from: '你的电脑', fromAddress: '127.0.0.1:15432', gateway: 'jump', to: '内网数据库', toAddress: 'db:5432',
    command: 'ssh -L 15432:db:5432 jump', note: '线路保存后可直接启停。示例地址仅用于说明。',
  },
  remote: {
    kicker: 'REMOTE FORWARDING', number: '02', title: '从远端连接本机开发服务',
    description: 'SSH 服务器监听 127.0.0.1:9000，并将连接转发到本机的 localhost:3000。',
    from: '远端监听', fromAddress: '127.0.0.1:9000', gateway: 'dev-box', to: '本机服务', toAddress: 'localhost:3000',
    command: 'ssh -R 9000:localhost:3000 dev-box', note: '对公网开放还需配置 GatewayPorts 与防火墙；示例仅监听远端回环地址。',
  },
  socks: {
    kicker: 'DYNAMIC FORWARDING', number: '03', title: '通过 SOCKS5 连接不同目标',
    description: '浏览器或终端连接本机代理端口，目标由客户端指定，连接从 SSH 服务器发出。',
    from: '本机代理', fromAddress: '127.0.0.1:1080', gateway: 'gateway', to: '请求目标', toAddress: '由客户端指定',
    command: 'ssh -D 127.0.0.1:1080 gateway', note: '支持 TCP CONNECT，不支持 UDP。建议仅监听回环地址。',
  },
}
if (english) Object.assign(modes, {
  local: {
    kicker: 'LOCAL FORWARDING', number: '01', title: 'Reach a private database from your computer',
    description: 'A local database client connects to 127.0.0.1:15432; the SSH server forwards it to db:5432.',
    from: 'Your computer', fromAddress: '127.0.0.1:15432', gateway: 'jump', to: 'Private database', toAddress: 'db:5432',
    command: 'ssh -L 15432:db:5432 jump', note: 'Save a tunnel to start or stop it. Addresses are illustrative.',
  },
  remote: {
    kicker: 'REMOTE FORWARDING', number: '02', title: 'Reach a local development service from the remote host',
    description: 'The SSH server listens on 127.0.0.1:9000 and forwards connections to localhost:3000 on your computer.',
    from: 'Remote listener', fromAddress: '127.0.0.1:9000', gateway: 'dev-box', to: 'Local service', toAddress: 'localhost:3000',
    command: 'ssh -R 9000:localhost:3000 dev-box', note: 'Public access also requires GatewayPorts and firewall settings; this example listens on remote loopback only.',
  },
  socks: {
    kicker: 'DYNAMIC FORWARDING', number: '03', title: 'Reach different destinations through SOCKS5',
    description: 'A browser or terminal connects to the local proxy port. The client chooses the destination, and traffic exits through the SSH server.',
    from: 'Local proxy', fromAddress: '127.0.0.1:1080', gateway: 'gateway', to: 'Destination', toAddress: 'Chosen by client',
    command: 'ssh -D 127.0.0.1:1080 gateway', note: 'Supports TCP CONNECT, not UDP. Binding to loopback is recommended.',
  },
})

function setText(id, value) { document.getElementById(id).textContent = value }

const modeTabs = [...document.querySelectorAll('[data-mode]')]
function selectMode(button) {
  const mode = modes[button.dataset.mode]
  modeTabs.forEach((tab) => {
    tab.setAttribute('aria-selected', String(tab === button))
    tab.tabIndex = tab === button ? 0 : -1
  })
  document.getElementById('mode-panel').setAttribute('aria-labelledby', button.id)
  for (const key of ['kicker', 'number', 'title', 'description', 'command', 'note']) setText(`mode-${key}`, mode[key])
  setText('route-from', mode.from)
  setText('route-from-address', mode.fromAddress)
  setText('route-gateway', mode.gateway)
  setText('route-to', mode.to)
  setText('route-to-address', mode.toAddress)
}
modeTabs.forEach((button, index) => {
  button.addEventListener('click', () => selectMode(button))
  button.addEventListener('keydown', (event) => {
    const next = { ArrowRight: (index + 1) % modeTabs.length, ArrowLeft: (index + modeTabs.length - 1) % modeTabs.length, Home: 0, End: modeTabs.length - 1 }[event.key]
    if (next === undefined) return
    event.preventDefault()
    selectMode(modeTabs[next])
    modeTabs[next].focus()
  })
})

const proxyToggle = document.getElementById('proxy-toggle')
function toggleProxy() {
  const stopped = proxyToggle.getAttribute('aria-pressed') !== 'true'
  proxyToggle.setAttribute('aria-pressed', String(stopped))
  proxyToggle.textContent = stopped ? translated('▶ 启动线路', '▶ Start tunnel') : translated('■ 停止线路', '■ Stop tunnel')
  document.getElementById('proxy-line').classList.toggle('is-stopped', stopped)
  document.getElementById('proxy-dot').setAttribute('aria-label', stopped ? translated('已停止', 'Stopped') : translated('运行中', 'Running'))
  setText('proxy-status', stopped ? translated('已停止 · 配置保留', 'Stopped · configuration saved') : translated('运行中', 'Running'))
  setText('running-count', stopped ? '2' : '3')
  setText('menubar-proxy-toggle', stopped ? translated('▶ 启动线路', '▶ Start tunnel') : translated('■ 停止线路', '■ Stop tunnel'))
  setText('menubar-proxy-rate', stopped ? translated('已停止', 'Stopped') : '244 KB/s')
  document.getElementById('menubar-proxy-dot').classList.toggle('is-stopped', stopped)
  document.getElementById('menubar-proxy-dot').setAttribute('aria-label', stopped ? translated('已停止', 'Stopped') : translated('运行中', 'Running'))
  document.getElementById('menubar-copy-proxy').hidden = stopped
  setText('menubar-feedback', stopped ? translated('示例线路已停止，配置保留。', 'Sample tunnel stopped; configuration saved.') : translated('示例线路已启动。', 'Sample tunnel started.'))
}
proxyToggle.addEventListener('click', toggleProxy)
document.getElementById('menubar-proxy-toggle').addEventListener('click', toggleProxy)

const menubarTrigger = document.getElementById('menubar-trigger')
function setMenubarOpen(open) {
  menubarTrigger.setAttribute('aria-expanded', String(open))
  document.getElementById('menubar-panel').hidden = !open
}
menubarTrigger.addEventListener('click', () => setMenubarOpen(menubarTrigger.getAttribute('aria-expanded') !== 'true'))
document.getElementById('menubar-panel').addEventListener('keydown', (event) => {
  if (event.key === 'Escape') { setMenubarOpen(false); menubarTrigger.focus() }
})

const dialog = document.getElementById('connections-dialog')
document.querySelectorAll('[data-open-connections]').forEach((button) => button.addEventListener('click', () => dialog.showModal()))
document.getElementById('close-dialog').addEventListener('click', () => dialog.close())
dialog.addEventListener('click', (event) => { if (event.target === dialog) {
  const bounds = dialog.getBoundingClientRect()
  if (event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom) dialog.close()
} })

const proxyCommand = "export all_proxy='socks5h://127.0.0.1:1080' http_proxy='socks5h://127.0.0.1:1080' https_proxy='socks5h://127.0.0.1:1080'"
async function copyProxy(event, feedback) {
  const button = event.currentTarget
  button.disabled = true
  try {
    await navigator.clipboard.writeText(proxyCommand)
    setText(feedback, translated('已复制示例。使用前请确认本机 1080 端口已运行代理。', 'Example copied. Before use, confirm a proxy is running on local port 1080.'))
  } catch {
    setText(feedback, `${translated('无法访问剪贴板，请手动复制：', 'Clipboard unavailable. Copy manually: ')}${proxyCommand}`)
  } finally { button.disabled = false }
}
document.getElementById('copy-proxy').addEventListener('click', (event) => copyProxy(event, 'copy-feedback'))
document.getElementById('menubar-copy-proxy').addEventListener('click', (event) => copyProxy(event, 'menubar-feedback'))

const platforms = {
  mac: { title: 'macOS', support: '12+ · Apple Silicon 与 Intel',
    description: '打开 DMG 或解压 ZIP，将 Portway.app 拖入“应用程序”，再从那里打开。',
    warning: '当前 macOS 包尚未公证，系统可能提示来源未知。请确认下载来源可信。' },
  windows: { title: 'Windows', support: 'Windows 10 / 11 · x64',
    description: '运行安装程序，或直接使用独立 EXE。需要 WebView2；缺少时会引导安装。',
    warning: '当前 Windows 包尚未做 Authenticode 签名，系统可能显示信誉警告。请确认下载来源可信。' },
  linux: { title: 'Linux', support: 'Ubuntu 24.04 · x64',
    description: 'Ubuntu 推荐安装 DEB 包。其他发行版可使用 tar.gz，需自行安装 GTK3、WebKitGTK 4.1。',
    warning: '当前构建以 Ubuntu 24.04 为目标，不保证兼容所有发行版。Linux 暂无托盘入口。' },
}
if (english) Object.assign(platforms, {
  mac: { title: 'macOS', support: '12+ · Apple Silicon and Intel',
    description: 'Open the DMG or extract the ZIP, drag Portway.app into Applications, and open it from there.',
    warning: 'The current macOS package is not notarized, so macOS may warn about an unknown developer. Verify the download source.' },
  windows: { title: 'Windows', support: 'Windows 10 / 11 · x64',
    description: 'Run the installer or use the standalone EXE. WebView2 is required; installation is offered if it is missing.',
    warning: 'The current Windows package is not Authenticode signed, so Windows may show a reputation warning. Verify the download source.' },
  linux: { title: 'Linux', support: 'Ubuntu 24.04 · x64',
    description: 'Install the DEB on Ubuntu. Other distributions can use the tar.gz with GTK3 and WebKitGTK 4.1 installed separately.',
    warning: 'The current build targets Ubuntu 24.04 and may not work on every distribution. Linux has no tray entry yet.' },
})
function selectOS(key) {
  const platform = platforms[key]
  document.querySelectorAll('[data-os]').forEach((button) => button.setAttribute('aria-pressed', String(button.dataset.os === key)))
  for (const field of ['title', 'support', 'description', 'warning']) setText(`os-${field}`, platform[field])
}
document.querySelectorAll('[data-os]').forEach((button) => button.addEventListener('click', () => selectOS(button.dataset.os)))
const platform = navigator.userAgentData?.platform || navigator.platform || ''
if (/win/i.test(platform)) selectOS('windows')
else if (/linux/i.test(platform) && !/android/i.test(navigator.userAgent)) selectOS('linux')
