import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const root = new URL('../../', import.meta.url)
const read = path => readFileSync(new URL(path, root), 'utf8')
const config = JSON.parse(read('desktop/wails.json'))
const build = read('scripts/desktop.mjs')

test('Wails display and executable names stay distinct and intentional', () => {
  assert.equal(config.outputfilename, 'portway')
  assert.equal(config.name, 'Portway')
  assert.equal(config.info.productName, 'Portway')
})

test('desktop binaries and distribution names use the shared configuration', () => {
  assert.match(build, /const executableName = config\.outputfilename/)
  assert.ok(build.includes('const stem = `${executableName}-${version}-${system}-${targetArch}`'))
  assert.ok(build.includes('name.startsWith(`${config.name}-`)'))
  assert.ok(build.includes("path.join(macos, executableName)"))
  assert.ok(build.includes(".replaceAll('{{.OutputFilename}}', xml(executableName))"))
  assert.ok(build.includes('path.join(bin, `${executableName}.exe`)'))
  assert.ok(build.includes("path.join(stage, 'usr/bin', executableName)"))
  assert.ok(build.includes('Name=Portway\\nExec=${executableName}'))
  assert.ok(build.includes("fs.rmSync(path.join(macos, 'ssh-tunnel-manager'), { force: true })"))
})

test('existing application and Linux package identities are preserved', () => {
  assert.match(read('desktop/build/darwin/Info.plist'), /io\.github\.ssh-tunnel-manager\.desktop/)
  assert.ok(build.includes('Package: ssh-tunnel-manager\\n'))
  assert.ok(build.includes("path.join(applications, 'ssh-tunnel-manager.desktop')"))
  assert.match(read('desktop/autostart/files.go'), /io\.github\.ssh-tunnel-manager\.desktop\.autostart/)
})
