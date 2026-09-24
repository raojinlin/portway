import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

const compiled = ts.transpileModule(readFileSync(new URL('../src/desktopNavigation.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText

test('native log navigation subscribes before ready, acknowledges and cleans up', () => {
  let listener, opened = 0
  const emitted = []
  const runtime = {
    EventsOn(name, callback) {
      assert.equal(name, 'desktop:navigate')
      listener = callback
      return () => { listener = undefined }
    },
    EventsEmit(name, ...args) {
      emitted.push([name, ...args])
      if (name === 'desktop:navigation-ready') listener('activity')
    },
  }
  const api = {}
  runInNewContext(compiled, { exports: api, window: { runtime } })
  const stop = api.listenForDesktopNavigation(() => opened++)
  assert.equal(opened, 1)
  assert.deepEqual(emitted[1], ['desktop:navigation-applied', 'activity'])
  listener('unknown')
  assert.equal(opened, 1)
  listener('activity')
  assert.equal(opened, 2)
  stop()
  assert.equal(listener, undefined)
})

test('regular browser without desktop runtime remains usable', () => {
  const api = {}
  runInNewContext(compiled, { exports: api, window: {} })
  assert.doesNotThrow(() => api.listenForDesktopNavigation(() => assert.fail())())
})

test('connection navigation preserves names and history, rejects malformed requests', () => {
  let listener
  const opened = [], emitted = []
  const api = {}
  const runtime = {
    EventsOn(_name, callback) { listener = callback; return () => {} },
    EventsEmit(name, ...args) { emitted.push([name, ...args]) },
  }
  runInNewContext(compiled, { exports: api, window: { runtime } })
  api.listenForDesktopNavigation(() => assert.fail('unexpected log navigation'), target => opened.push(target))
  for (const name of ['stopped SOCKS', '线路 / test']) {
    const target = { page: 'connections', name, showHistory: true }
    listener(target)
    assert.equal(opened.at(-1).name, name)
    assert.equal(opened.at(-1).showHistory, true)
    assert.deepEqual(emitted.at(-1), ['desktop:navigation-applied', target])
  }
  listener({ page: 'connections', name: 'local', showHistory: false })
  assert.equal(opened.at(-1).showHistory, false)
  const count = emitted.length
  for (const invalid of [null, {}, { page: 'connections' }, { page: 'connections', name: '', showHistory: true }, { page: 'connections', name: 'test', showHistory: 'true' }]) listener(invalid)
  assert.equal(opened.length, 3)
  assert.equal(emitted.length, count)
})
