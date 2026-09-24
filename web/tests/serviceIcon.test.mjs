import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

const source = readFileSync(new URL('../src/serviceIcon.ts', import.meta.url), 'utf8')
const exports = {}
runInNewContext(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText, { exports })
const { resolveServiceIcon, serviceIcons } = exports

test('service icons detect the forwarded target and allow explicit overrides', () => {
  for (const [port, expected] of [[22, 'ssh'], [3306, 'mysql'], [5432, 'postgresql'], [80, 'http'], [443, 'https'], [8080, 'http'], [8443, 'https'], [6379, 'redis'], [27017, 'mongodb'], [3389, 'rdp'], [9999, 'generic']]) {
    for (const direction of ['local', 'remote']) {
      for (const host of ['127.0.0.1', '[::1]', 'db.local']) {
        assert.equal(resolveServiceIcon('auto', direction, `${host}:${port}`), expected)
        assert.equal(resolveServiceIcon('postgresql', direction, `${host}:${port}`), 'postgresql')
      }
    }
  }
  assert.equal(resolveServiceIcon(undefined, 'dynamic', 'localhost:3306'), 'socks5')
  for (const target of ['', 'localhost', 'localhost:0', 'localhost:65536']) {
    assert.equal(resolveServiceIcon('auto', 'local', target), 'generic')
  }
  for (const kind of serviceIcons) assert.equal(resolveServiceIcon(kind, 'dynamic', ''), kind)
})
