import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

const source = readFileSync(new URL('../src/proxyCommand.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText
function setup(navigator = {}, document = {}) {
  const api = {}
  runInNewContext(compiled, { exports: api, navigator, document })
  return api
}

test('SOCKS URLs use proxy-side DNS, preserve IPv6 and normalize wildcard listeners', () => {
  const { proxyURL } = setup()
  for (const [listen, host] of [['127.0.0.1:1080', '127.0.0.1'], ['0.0.0.0:1080', '127.0.0.1'], [':1080', '127.0.0.1'], ['[::]:1080', '[::1]'], ['[::1]:1080', '[::1]'], ['[2001:db8::1]:1080', '[2001:db8::1]'], ['proxy.local:1080', 'proxy.local']]) {
    assert.equal(proxyURL(listen), `socks5h://${host}:1080`)
  }
  assert.equal(proxyURL(' localhost:08080 '), 'socks5h://localhost:8080')
})

test('invalid listeners and shell injection cannot produce a command', () => {
  for (const listen of ['', 'localhost', 'localhost:0', 'localhost:65536', '::1:1080', "host'; touch /tmp/pwn;':1080", '$(whoami):1080', 'host:1080/path', 'user@host:1080']) {
    assert.throws(() => setup().proxyURL(listen), /Invalid SOCKS5/)
  }
})

test('generates POSIX, PowerShell and URL commands without executing anything', () => {
  const { proxyCommand } = setup()
  const url = 'socks5h://127.0.0.1:1080'
  assert.equal(proxyCommand(url, 'posix'), `export all_proxy='${url}' http_proxy='${url}' https_proxy='${url}'`)
  assert.equal(proxyCommand(url, 'powershell'), `$env:all_proxy='${url}'; $env:http_proxy=$env:all_proxy; $env:https_proxy=$env:all_proxy`)
  assert.equal(proxyCommand(url, 'url'), url)
})

test('uses clipboard API, with fallback and cleanup on HTTP or denied access', async () => {
  let copied = ''
  await setup({ clipboard: { writeText: async text => { copied = text } } }).copyText('command')
  assert.equal(copied, 'command')
  for (const navigator of [{}, { clipboard: { writeText: async () => { throw new Error('denied') } } }]) {
    let removed = false, restored = false, selected = false
    const input = { style: {}, select: () => { selected = true }, remove: () => { removed = true } }
    const document = {
      activeElement: { focus: () => { restored = true } },
      createElement: () => input, body: { appendChild: () => {} },
      execCommand: name => { assert.equal(name, 'copy'); return true },
    }
    await setup(navigator, document).copyText('fallback')
    assert.equal(input.value, 'fallback')
    assert.ok(removed && restored && selected)
    document.execCommand = () => false
    await assert.rejects(setup(navigator, document).copyText('fallback'), /Clipboard unavailable/)
  }
})
