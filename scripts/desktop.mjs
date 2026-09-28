import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { version } from './version.mjs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const desktop = path.join(root, 'desktop')
const config = JSON.parse(fs.readFileSync(path.join(desktop, 'wails.json'), 'utf8'))
const executableName = config.outputfilename
if (!/^[a-zA-Z0-9_-]+$/.test(executableName)) throw new Error('Invalid desktop executable name')
const output = path.join(root, 'dist', 'desktop')
const flags = new Set(process.argv.slice(2))
for (const flag of flags) {
  if (!['--universal', '--installer', '--dmg'].includes(flag)) throw new Error(`Unknown option: ${flag}`)
}
const system = { darwin: 'darwin', win32: 'windows', linux: 'linux' }[process.platform]
const arch = { arm64: 'arm64', x64: 'amd64' }[process.arch]
if (!system || !arch) throw new Error('Supported builders: macOS, Windows and Linux (amd64/arm64)')
if (flags.has('--universal') && system !== 'darwin') throw new Error('--universal requires macOS')
if (flags.has('--dmg') && system !== 'darwin') throw new Error('--dmg requires macOS')
if (flags.has('--installer') && system !== 'windows') throw new Error('--installer requires Windows and NSIS')
const environment = { ...process.env }
if (system === 'darwin') {
  environment.MACOSX_DEPLOYMENT_TARGET ??= '12.0'
  // Explicit flags participate in Go's CGO cache keys; the environment variable alone does not.
  for (const key of ['CGO_CFLAGS', 'CGO_CXXFLAGS', 'CGO_LDFLAGS']) {
    const value = environment[key] || '-O2 -g'
    environment[key] = value.includes('-mmacosx-version-min=') ? value : `${value} -mmacosx-version-min=${environment.MACOSX_DEPLOYMENT_TARGET}`
  }
}

function run(command, args, cwd = root, env = environment) {
  const result = spawnSync(command, args, { cwd, env, stdio: 'inherit' })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(`${command} failed (exit ${result.status})`)
}

function wailsCommand() {
  if (process.env.WAILS) return process.env.WAILS
  const result = spawnSync('go', ['env', 'GOPATH'], { encoding: 'utf8' })
  const first = result.stdout?.trim().split(path.delimiter)[0]
  if (first) {
    const executable = path.join(first, 'bin', system === 'windows' ? 'wails.exe' : 'wails')
    if (fs.existsSync(executable)) return executable
  }
  return 'wails'
}

if (!fs.existsSync(path.join(root, 'internal/daemon/webui/dist/index.html'))) {
  throw new Error('Build the frontend first: npm --prefix web ci && npm --prefix web run build')
}
fs.mkdirSync(output, { recursive: true })
run('go', ['run', './tools/desktopicon', 'desktop/build/appicon.png'])
const targetArch = flags.has('--universal') ? 'universal' : arch
const args = ['build', '-s', '-skipbindings', '-m', '-nosyncgomod', '-ldflags', `-s -w -X main.applicationVersion=${version}`, '-platform', `${system}/${targetArch}`]
if (system === 'linux') args.push('-tags', 'webkit2_41')
if (system === 'windows') args.push('-webview2', 'embed')
if (flags.has('--installer')) args.push('-nsis', '-installscope', 'user')
const bin = path.join(desktop, 'build', 'bin')
const stem = `${executableName}-${version}-${system}-${targetArch}`
if (system === 'darwin') {
  const bundleName = `${config.info.productName}.app`
  const app = path.join(bin, bundleName)
  const macos = path.join(app, 'Contents/MacOS')
  const resources = path.join(app, 'Contents/Resources')
  fs.mkdirSync(macos, { recursive: true })
  fs.mkdirSync(resources, { recursive: true })
  const stage = fs.mkdtempSync(path.join(os.tmpdir(), 'ssh-tunnel-mac-build-'))
  try {
    const architectures = targetArch === 'universal' ? ['amd64', 'arm64'] : [arch]
    for (const architecture of architectures) {
      run('go', ['build', '-buildvcs=false', '-tags', 'production', '-ldflags', `-s -w -X main.applicationVersion=${version}`, '-o', path.join(stage, architecture), '.'], desktop,
        { ...environment, CGO_ENABLED: '1', GOOS: 'darwin', GOARCH: architecture })
    }
    const executable = path.join(macos, executableName)
    if (architectures.length === 2) run('lipo', ['-create', ...architectures.map((name) => path.join(stage, name)), '-output', executable])
    else fs.copyFileSync(path.join(stage, arch), executable)
    fs.chmodSync(executable, 0o755)
    // Remove only the obsolete generated executable, not app data or old packages.
    if (executableName !== 'ssh-tunnel-manager') fs.rmSync(path.join(macos, 'ssh-tunnel-manager'), { force: true })
  } finally { fs.rmSync(stage, { recursive: true, force: true }) }
  const xml = (value) => String(value).replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;')
  const plist = fs.readFileSync(path.join(desktop, 'build/darwin/Info.plist'), 'utf8')
    .replaceAll('{{.Info.ProductName}}', xml(config.info.productName))
    .replaceAll('{{.OutputFilename}}', xml(executableName))
    .replaceAll('{{.Info.ProductVersion}}', xml(version))
    .replaceAll('{{.Info.Copyright}}', xml(config.info.copyright))
  fs.writeFileSync(path.join(app, 'Contents/Info.plist'), plist)
  // ICNS ic10 stores a PNG-encoded 1024px icon; macOS scales other display sizes.
  const png = fs.readFileSync(path.join(desktop, 'build/appicon.png'))
  const header = Buffer.alloc(16)
  header.write('icns', 0); header.writeUInt32BE(png.length + 16, 4)
  header.write('ic10', 8); header.writeUInt32BE(png.length + 8, 12)
  fs.writeFileSync(path.join(resources, 'iconfile.icns'), Buffer.concat([header, png]))
  // Ad-hoc signing is for local builds; distribution needs Developer ID + notarization.
  run('codesign', ['--force', '--sign', '-', app])
  run('codesign', ['--verify', '--deep', '--strict', app])
  run('ditto', ['-c', '-k', '--sequesterRsrc', '--keepParent', app, path.join(output, `${stem}.zip`)])
  if (flags.has('--dmg')) {
    const stage = fs.mkdtempSync(path.join(os.tmpdir(), 'ssh-tunnel-dmg-'))
    try {
      fs.cpSync(app, path.join(stage, bundleName), { recursive: true })
      fs.symlinkSync('/Applications', path.join(stage, 'Applications'))
      run('hdiutil', ['create', '-volname', config.info.productName, '-srcfolder', stage, '-format', 'UDZO', '-ov', path.join(output, `${stem}.dmg`)])
    } finally { fs.rmSync(stage, { recursive: true, force: true }) }
  }
  console.log(`Application: ${app}`)
} else if (system === 'windows') {
  run(wailsCommand(), args, desktop)
  fs.copyFileSync(path.join(bin, `${executableName}.exe`), path.join(output, `${stem}.exe`))
  if (flags.has('--installer')) {
    // Wails names the NSIS installer from the project name, not outputfilename.
    const installers = fs.readdirSync(bin).filter((name) => name.startsWith(`${config.name}-`) && name.endsWith('-installer.exe'))
    if (installers.length !== 1) throw new Error(`Expected one NSIS installer, found ${installers.length}`)
    fs.copyFileSync(path.join(bin, installers[0]), path.join(output, `${stem}-installer.exe`))
  }
} else {
  run(wailsCommand(), args, desktop)
  const stage = fs.mkdtempSync(path.join(os.tmpdir(), 'ssh-tunnel-linux-'))
  try {
    const executable = path.join(stage, 'usr/bin', executableName)
    fs.mkdirSync(path.dirname(executable), { recursive: true })
    fs.copyFileSync(path.join(bin, executableName), executable)
    fs.chmodSync(executable, 0o755)
    const applications = path.join(stage, 'usr/share/applications')
    const icons = path.join(stage, 'usr/share/icons/hicolor/1024x1024/apps')
    fs.mkdirSync(applications, { recursive: true })
    fs.mkdirSync(icons, { recursive: true })
    fs.copyFileSync(path.join(desktop, 'build/appicon.png'), path.join(icons, 'ssh-tunnel-manager.png'))
    // Keep the desktop-file, icon and package IDs stable for existing installations.
    fs.writeFileSync(path.join(applications, 'ssh-tunnel-manager.desktop'), `[Desktop Entry]\nType=Application\nName=Portway\nExec=${executableName}\nIcon=ssh-tunnel-manager\nTerminal=false\nCategories=Network;\nStartupWMClass=${executableName}\n`)
    run('tar', ['-czf', path.join(output, `${stem}.tar.gz`), '-C', stage, 'usr'])
    if (spawnSync('dpkg-deb', ['--version'], { stdio: 'ignore' }).status === 0) {
      fs.mkdirSync(path.join(stage, 'DEBIAN'))
      fs.writeFileSync(path.join(stage, 'DEBIAN/control'), `Package: ssh-tunnel-manager\nVersion: ${version}\nArchitecture: ${arch}\nMaintainer: SSH Tunnel Manager\nDepends: libgtk-3-0 | libwebkit2gtk-4.1-0\nSection: net\nPriority: optional\nDescription: Desktop SSH forwarding manager\n`)
      run('dpkg-deb', ['--build', '--root-owner-group', stage, path.join(output, `${stem}.deb`)])
    } else { console.log('dpkg-deb not found; created tar.gz only') }
  } finally { fs.rmSync(stage, { recursive: true, force: true }) }
}
console.log(`Packages: ${output}`)
