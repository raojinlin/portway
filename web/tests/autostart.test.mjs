import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

const compiled = ts.transpileModule(readFileSync(new URL('../src/components/AutostartSettings.tsx', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
}).outputText

function harness(api) {
  const slots = []
  let index = 0, dirty = true, effects = [], tree
  const hooks = {
    useState(initial) {
      const key = index++
      slots[key] ??= { value: initial }
      return [slots[key].value, value => { slots[key].value = typeof value === 'function' ? value(slots[key].value) : value; dirty = true }]
    },
    useRef(initial) { return slots[index++] ??= { current: initial } },
    useEffect(callback, deps) {
      const key = index++, old = slots[key]
      if (!old || deps.some((value, i) => value !== old.deps[i])) effects.push(() => {
        old?.cleanup?.(); slots[key] = { deps, cleanup: callback() }
      })
    },
  }
  const exports = {}
  const element = (type, props) => ({ type, props })
  runInNewContext(compiled, { exports, AbortController, require(id) {
    if (id === 'react') return hooks
    if (id === 'react/jsx-runtime') return { jsx: element, jsxs: element }
    if (id === 'antd') return { Switch: 'Switch', Alert: 'Alert', Button: 'Button' }
    if (id === '../api') return { api }
    if (id === '../I18n') return { useI18n: () => ({ tr: value => value }) }
    throw new Error(id)
  } })
  function render() {
    while (dirty) {
      dirty = false; index = 0; effects = []
      tree = exports.default()
      for (const effect of effects) effect()
    }
    return tree
  }
  function find(type, node = render()) {
    if (!node || typeof node !== 'object') return undefined
    if (node.type === type) return node.props
    for (const child of [node.props?.children].flat()) {
      if (!child || typeof child !== 'object') continue
      const found = find(type, child)
      if (found) return found
    }
  }
  return { render, find, async flush() { await new Promise(setImmediate); render() }, dispose() { for (const slot of slots) slot?.cleanup?.() } }
}

test('autostart switch waits for persistence, preserves state on errors and can retry', async () => {
  let resolveWrite, rejectWrite, writes = 0
  const h = harness({
    autostart: async () => ({ supported: true, enabled: false }),
    setAutostart: () => { writes++; return new Promise((resolve, reject) => { resolveWrite = resolve; rejectWrite = reject }) },
  })
  assert.ok(h.find('Switch').disabled)
  await h.flush()
  assert.equal(h.find('Switch').disabled, false)
  const first = h.find('Switch').onChange(true)
  assert.equal(h.find('Switch').checked, false)
  assert.ok(h.find('Switch').loading)
  await h.find('Switch').onChange(true)
  assert.equal(writes, 1)
  rejectWrite(new Error('permission denied'))
  await first
  assert.equal(h.find('Switch').checked, false)
  assert.equal(h.find('Alert').message, 'permission denied')
  const second = h.find('Switch').onChange(true)
  resolveWrite({ supported: true, enabled: true })
  await second
  assert.equal(h.find('Switch').checked, true)
  assert.equal(h.find('Alert'), undefined)
  h.dispose()
})

test('unmount aborts reads and ignores late results', async () => {
  let signal, resolveRead
  const h = harness({ autostart: value => { signal = value; return new Promise(resolve => { resolveRead = resolve }) } })
  h.render()
  h.dispose()
  assert.ok(signal.aborted)
  resolveRead({ supported: true, enabled: true })
  await h.flush()
  assert.equal(h.find('Switch').checked, false)
})
