import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const root = new URL('../../', import.meta.url)
const read = path => readFileSync(new URL(path, root), 'utf8')
const config = JSON.parse(read('desktop/wails.json'))
const build = read('scripts/desktop.mjs')

test('Wails executable and NSIS project names agree while display name stays Portway', () => {
  assert.equal(config.outputfilename, 'portway')
  // The default NSIS template installs ${INFO_PROJECTNAME}.exe, not outputfilename.
  assert.equal(config.name, config.outputfilename)
  assert.equal(config.info.productName, 'Portway')
})

test('desktop binaries and distribution names use the shared configuration', () => {
  assert.match(build, /const executableName = config\.outputfilename/)
  assert.ok(build.includes('const stem = `${executableName}-${system}-${targetArch}`'))
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
