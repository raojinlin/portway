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
