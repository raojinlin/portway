import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { runInNewContext } from 'node:vm'

const source = readFileSync(new URL('../theme.js', import.meta.url), 'utf8')
function setup({ stored = null, dark = false, denied = false, noMedia = false } = {}) {
  const root = { dataset: {} }, meta = {}, events = {}, selectEvents = {}, windowEvents = {}, writes = []
  const media = { matches: dark, addEventListener(name, callback) { events[name] = callback } }
  const select = { value: '', addEventListener(name, callback) { selectEvents[name] = callback } }
  let ready = false
  runInNewContext(source, {
    window: {
      matchMedia() { if (noMedia) throw new Error('unavailable'); return media },
      localStorage: { getItem() { if (denied) throw new Error('denied'); return stored }, setItem(key, value) { if (denied) throw new Error('denied'); writes.push([key, value]) } },
      addEventListener(name, callback) { windowEvents[name] = callback },
    },
    document: {
      documentElement: root, querySelector: () => meta,
      getElementById: () => ready ? select : null,
      addEventListener(name, callback) { events[name] = callback },
    },
  })
  return { root, meta, select, writes,
    ready() { ready = true; events.DOMContentLoaded() },
    choose(value) { select.value = value; selectEvents.change() },
    system(value) { media.matches = value; events.change() },
    storage(key, newValue) { windowEvents.storage({ key, newValue }) },
  }
}

test('initial theme is applied before DOM ready and follows live system changes', () => {
  const h = setup({ dark: true })
  assert.equal(h.root.dataset.theme, 'dark')
  assert.equal(h.meta.content, '#181a1d')
  h.ready()
  assert.equal(h.select.value, 'system')
  h.system(false)
  assert.equal(h.root.dataset.theme, 'light')
  assert.equal(h.meta.content, '#f3f5f7')
  assert.equal(h.writes.length, 0)
})

test('explicit preference persists and ignores system changes until system is selected', () => {
  const h = setup({ stored: 'light', dark: true })
  assert.equal(h.root.dataset.theme, 'light')
  h.ready()
  h.choose('dark')
  assert.deepEqual(h.writes, [['portway-site-theme', 'dark']])
  h.system(false)
  assert.equal(h.root.dataset.theme, 'dark')
  h.choose('system')
  assert.equal(h.root.dataset.theme, 'light')
  assert.deepEqual(h.writes[1], ['portway-site-theme', 'system'])
})

test('invalid stored values, blocked storage and unavailable media query do not break theme controls', () => {
  for (const options of [{ stored: 'invalid', dark: true }, { denied: true, dark: true }, { noMedia: true }]) {
    const h = setup(options)
    h.ready()
    assert.equal(h.select.value, 'system')
    h.choose('dark')
    assert.equal(h.root.dataset.theme, 'dark')
    h.choose('light')
    assert.equal(h.root.dataset.theme, 'light')
  }
})

test('theme stays synchronized across tabs without overwriting unrelated preferences', () => {
  const h = setup()
  h.ready()
  h.storage('portway-site-theme', 'dark')
  assert.equal(h.root.dataset.theme, 'dark')
  assert.equal(h.select.value, 'dark')
  h.storage('other', 'light')
  assert.equal(h.root.dataset.theme, 'dark')
  h.storage(null, null)
  assert.equal(h.select.value, 'system')
  assert.equal(h.writes.length, 0)
})

test('light and dark palette values match the app and support a no-JavaScript fallback', () => {
  const site = readFileSync(new URL('../style.css', import.meta.url), 'utf8')
  const app = readFileSync(new URL('../../web/src/index.css', import.meta.url), 'utf8')
  const palette = (css, selector) => {
    const start = css.indexOf(selector)
    const block = css.slice(start, css.indexOf('}', start))
    return Object.fromEntries([...block.matchAll(/(--[\w-]+):\s*(#[\da-f]+);/g)].map(match => [match[1], match[2]]))
  }
  for (const selector of [':root {', ":root[data-theme='dark'] {"]) {
    const expected = palette(app, selector), actual = palette(site, selector)
    for (const [key, value] of Object.entries(expected)) assert.equal(actual[key], value, `${selector} ${key}`)
  }
  assert.match(site, /@media \(prefers-color-scheme: dark\)/)
  assert.deepEqual(palette(site, ':root:not([data-theme]) {'), palette(site, ":root[data-theme='dark'] {"))
  const html = readFileSync(new URL('../index.html', import.meta.url), 'utf8')
  assert.ok(html.indexOf('src="./theme.js"') < html.indexOf('href="./style.css"'))
})
