import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { version } from './version.mjs'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const output = path.join(root, 'dist', 'cli')
const flags = new Set(process.argv.slice(2))
for (const flag of flags) {
  if (flag !== '--universal') throw new Error(`Unknown option: ${flag}`)
}

const system = { darwin: 'darwin', win32: 'windows', linux: 'linux' }[process.platform]
const arch = { arm64: 'arm64', x64: 'amd64' }[process.arch]
if (!system || !arch) throw new Error('Supported builders: macOS, Windows and Linux (amd64/arm64)')
if (flags.has('--universal') && system !== 'darwin') throw new Error('--universal requires macOS')
if (!fs.existsSync(path.join(root, 'internal/daemon/webui/dist/index.html'))) {
  throw new Error('Build the frontend first: npm --prefix web ci && npm --prefix web run build')
}

function run(command, args, env = process.env) {
  const result = spawnSync(command, args, { cwd: root, env, stdio: 'inherit' })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error(`${command} failed (exit ${result.status})`)
}

function build(target, architecture) {
  run('go', [
    'build', '-buildvcs=false', '-trimpath', '-ldflags', `-s -w -X main.version=${version}`,
    '-o', target, './cmd/tunnel',
  ], { ...process.env, CGO_ENABLED: '0', GOOS: system, GOARCH: architecture })
}

fs.mkdirSync(output, { recursive: true })
const targetArch = flags.has('--universal') ? 'universal' : arch
const extension = system === 'windows' ? '.exe' : ''
const target = path.join(output, `portway${extension}`)

if (targetArch === 'universal') {
  const stage = fs.mkdtempSync(path.join(os.tmpdir(), 'portway-cli-build-'))
  try {
    const binaries = ['amd64', 'arm64'].map((architecture) => {
      const binary = path.join(stage, architecture)
      build(binary, architecture)
      return binary
    })
    run('lipo', ['-create', ...binaries, '-output', target])
  } finally {
    fs.rmSync(stage, { recursive: true, force: true })
  }
} else {
  build(target, arch)
}

if (system !== 'windows') fs.chmodSync(target, 0o755)
console.log(`Command-line app: ${target}`)
