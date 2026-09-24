import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

const source = readFileSync(new URL('../src/useLiveQuery.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText

// Minimal hook lifecycle harness; no DOM or live backend is required.
function harness(initialLoad) {
  const slots = [], timers = new Map()
  let index = 0, dirty = true, effects = [], load = initialLoad, live = true, state, timerID = 0
  const hooks = {
    useState(initial) {
      const key = index++
      if (!slots[key]) slots[key] = { value: typeof initial === 'function' ? initial() : initial }
      return [slots[key].value, value => { slots[key].value = typeof value === 'function' ? value(slots[key].value) : value; dirty = true }]
    },
    useRef(initial) { const key = index++; return slots[key] ??= { current: initial } },
    useEffect(callback, deps) {
      const key = index++, old = slots[key]
      if (!old || deps.some((value, i) => value !== old.deps[i])) {
        effects.push(() => { old?.cleanup?.(); slots[key] = { deps, cleanup: callback() } })
      }
    },
  }
  const api = {}
  runInNewContext(compiled, { exports: api, require: () => hooks, AbortController,
    setTimeout: callback => { const id = ++timerID; timers.set(id, callback); return id }, clearTimeout: id => timers.delete(id) })
  function render() {
    while (dirty) {
      dirty = false; index = 0; effects = []
      state = api.useLiveQuery(load, live)
      for (const effect of effects) effect()
    }
    return state
  }
  return {
    timers, render,
    async flush() { await new Promise(setImmediate); return render() },
    setLive(value) { live = value; dirty = true; return render() },
    setLoad(value) { load = value; dirty = true; return render() },
    tick() { const [id, callback] = timers.entries().next().value; timers.delete(id); callback(); return render() },
    dispose() { for (const slot of slots) slot?.cleanup?.() },
  }
}

test('polling waits for completion, pause aborts and manual refresh still works', async () => {
  const pending = []
  const load = signal => new Promise(resolve => pending.push({ resolve, signal }))
  const h = harness(load)
  h.render()
  assert.equal(pending.length, 1)
  assert.equal(h.timers.size, 0)
  pending[0].resolve('first')
  assert.equal((await h.flush()).data, 'first')
  assert.equal(h.timers.size, 1)
  h.tick()
  assert.equal(pending.length, 2)
  h.setLive(false)
  assert.ok(pending[1].signal.aborted)
  pending[1].resolve('stale')
  assert.equal((await h.flush()).data, 'first')
  assert.equal(h.timers.size, 0)
  h.render().refresh()
  h.render()
  pending[2].resolve('manual')
  assert.equal((await h.flush()).data, 'manual')
  assert.equal(h.timers.size, 0)
  h.setLive(true)
  assert.equal(pending.length, 4)
  h.dispose()
  assert.ok(pending[3].signal.aborted)
})

test('changing filters clears old data and ignores late results; errors preserve the last snapshot', async () => {
  let resolveOld
  const h = harness(() => new Promise(resolve => { resolveOld = resolve }))
  h.render()
  let fail = false
  const load = async () => { if (fail) throw new Error('unavailable'); return 'filtered' }
  h.setLoad(load)
  assert.equal(h.render().data, null)
  resolveOld('wrong filter')
  assert.equal((await h.flush()).data, 'filtered')
  fail = true
  h.tick()
  const state = await h.flush()
  assert.equal(state.data, 'filtered')
  assert.equal(state.error, 'unavailable')
  h.dispose()
  assert.equal(h.timers.size, 0)
})
