const modes = {
  local: {
    kicker: 'LOCAL FORWARDING', number: '01', title: '在本机，访问内网数据库。',
    description: '数据库不必开放公网端口。让本机的数据库客户端，经由 SSH 跳板连接远端服务。',
    from: '你的电脑', fromAddress: '127.0.0.1:15432', gateway: 'jump', to: '内网数据库', toAddress: 'db:5432',
    command: 'ssh -L 15432:db:5432 jump', note: '相同的转发意图，不必每次重新输入。示例地址仅用于说明。',
  },
  remote: {
    kicker: 'REMOTE FORWARDING', number: '02', title: '让远端，也能访问本机服务。',
    description: '服务还在本机开发。通过远程转发，让 SSH 服务器上的程序连接到你的本地开发端口。',
    from: '远端监听', fromAddress: '127.0.0.1:9000', gateway: 'dev-box', to: '本机服务', toAddress: 'localhost:3000',
    command: 'ssh -R 9000:localhost:3000 dev-box', note: '对公网开放还需配置 GatewayPorts 与防火墙；示例仅监听远端回环地址。',
  },
  socks: {
    kicker: 'DYNAMIC FORWARDING', number: '03', title: '目的地不固定，就走 SOCKS5。',
    description: '在本机开一个代理端口。浏览器或终端按需指定目标，由 SSH 服务器发起连接。',
    from: '本机代理', fromAddress: '127.0.0.1:1080', gateway: 'gateway', to: '请求目标', toAddress: '由客户端指定',
    command: 'ssh -D 127.0.0.1:1080 gateway', note: '支持 TCP CONNECT，不支持 UDP。建议仅监听回环地址。',
  },
}

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
  proxyToggle.textContent = stopped ? '▶ 启动线路' : '■ 停止线路'
  document.getElementById('proxy-line').classList.toggle('is-stopped', stopped)
  document.getElementById('proxy-dot').setAttribute('aria-label', stopped ? '已停止' : '运行中')
  setText('proxy-status', stopped ? '已停止 · 配置保留' : '运行中')
  setText('running-count', stopped ? '2' : '3')
  setText('menubar-proxy-toggle', stopped ? '▶ 启动线路' : '■ 停止线路')
  setText('menubar-proxy-rate', stopped ? '已停止' : '244 KB/s')
  document.getElementById('menubar-proxy-dot').classList.toggle('is-stopped', stopped)
  document.getElementById('menubar-proxy-dot').setAttribute('aria-label', stopped ? '已停止' : '运行中')
  document.getElementById('menubar-copy-proxy').hidden = stopped
  setText('menubar-feedback', stopped ? '示例线路已停止，配置保留。' : '示例线路已启动。')
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
    setText(feedback, '已复制示例。使用前请确认本机 1080 端口已运行代理。')
  } catch {
    setText(feedback, `无法访问剪贴板，请手动复制：${proxyCommand}`)
  } finally { button.disabled = false }
}
document.getElementById('copy-proxy').addEventListener('click', (event) => copyProxy(event, 'copy-feedback'))
document.getElementById('menubar-copy-proxy').addEventListener('click', (event) => copyProxy(event, 'menubar-feedback'))

const platforms = {
  mac: { title: 'macOS', support: '12+ · Apple Silicon 与 Intel', artifact: 'portway-macos-universal',
    description: '打开 DMG 或解压 ZIP，将 Portway.app 拖入“应用程序”，再从那里打开。',
    warning: '当前 macOS 包尚未公证，系统可能提示来源未知。请确认下载来源可信。' },
  windows: { title: 'Windows', support: 'Windows 10 / 11 · x64', artifact: 'portway-windows-amd64',
    description: '运行安装程序，或直接使用独立 EXE。需要 WebView2；缺少时会引导安装。',
    warning: '当前 Windows 包尚未做 Authenticode 签名，系统可能显示信誉警告。请确认下载来源可信。' },
  linux: { title: 'Linux', support: 'Ubuntu 24.04 · x64', artifact: 'portway-linux-amd64',
    description: 'Ubuntu 推荐安装 DEB 包。其他发行版可使用 tar.gz，需自行安装 GTK3、WebKitGTK 4.1。',
    warning: '当前构建以 Ubuntu 24.04 为目标，不保证兼容所有发行版。Linux 暂无托盘入口。' },
}
function selectOS(key) {
  const platform = platforms[key]
  document.querySelectorAll('[data-os]').forEach((button) => button.setAttribute('aria-pressed', String(button.dataset.os === key)))
  for (const field of ['title', 'support', 'description', 'warning']) setText(`os-${field}`, platform[field])
  setText('artifact-name', platform.artifact)
}
document.querySelectorAll('[data-os]').forEach((button) => button.addEventListener('click', () => selectOS(button.dataset.os)))
const platform = navigator.userAgentData?.platform || navigator.platform || ''
if (/win/i.test(platform)) selectOS('windows')
else if (/linux/i.test(platform) && !/android/i.test(navigator.userAgent)) selectOS('linux')
