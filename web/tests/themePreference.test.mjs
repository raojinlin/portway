import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

const source = readFileSync(new URL('../src/themePreference.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 },
}).outputText

function setup(saved, dark = false) {
  const storage = new Map(saved ? [['stm-theme-mode', saved]] : [])
  const listeners = new Set()
  const query = {
    matches: dark,
    addEventListener(type, listener) {
      assert.equal(type, 'change')
      listeners.add(listener)
    },
    removeEventListener(type, listener) {
      assert.equal(type, 'change')
      listeners.delete(listener)
    },
  }
  const window = {
    localStorage: { getItem: key => storage.get(key), setItem: (key, value) => storage.set(key, value) },
    matchMedia: value => {
      assert.equal(value, '(prefers-color-scheme: dark)')
      return query
    },
  }
  const api = {}
  runInNewContext(compiled, { exports: api, window })
  return { api, window, storage, listeners, change(dark) {
    query.matches = dark
    for (const listener of listeners) listener()
  } }
}

test('defaults to system and preserves existing explicit preferences', () => {
  for (const [saved, expected] of [[undefined, 'system'], ['invalid', 'system'], ['system', 'system'], ['light', 'light'], ['dark', 'dark']]) {
    assert.equal(setup(saved).api.loadThemePreference(), expected)
  }
})

test('stores the preference, not the resolved system colour', () => {
  const { api, storage } = setup('dark')
  api.saveThemePreference('system')
  assert.equal(storage.get('stm-theme-mode'), 'system')
  assert.equal(api.loadThemePreference(), 'system')
  assert.equal(api.resolveTheme('system', false), 'light')
  assert.equal(api.resolveTheme('system', true), 'dark')
  assert.equal(storage.get('stm-theme-mode'), 'system')
})

test('reacts to system changes, preserves manual overrides and cleans up listeners', () => {
  const { api, listeners, change } = setup()
  const colours = []
  let preference = 'system'
  const unsubscribe = api.watchSystemTheme(dark => colours.push(api.resolveTheme(preference, dark)))
  change(true)
  change(false)
  preference = 'dark'
  change(false)
  preference = 'light'
  change(true)
  preference = 'system'
  change(true)
  assert.deepEqual(colours, ['light', 'dark', 'light', 'dark', 'light', 'dark'])
  unsubscribe()
  assert.equal(listeners.size, 0)
  change(false)
  assert.equal(colours.length, 6)
})

test('unavailable browser storage or media queries do not break startup', () => {
  const { api, window } = setup()
  Object.defineProperty(window, 'localStorage', { get() { throw new Error('blocked') } })
  assert.equal(api.loadThemePreference(), 'system')
  assert.doesNotThrow(() => api.saveThemePreference('dark'))
  window.matchMedia = undefined
  assert.doesNotThrow(() => api.watchSystemTheme(() => assert.fail('unexpected update'))())
})
