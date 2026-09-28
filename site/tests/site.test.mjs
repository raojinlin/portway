import assert from 'node:assert/strict'
import { readFileSync, existsSync } from 'node:fs'
import test from 'node:test'
import { runInNewContext } from 'node:vm'

const root = new URL('../', import.meta.url)
const html = readFileSync(new URL('index.html', root), 'utf8')
const source = readFileSync(new URL('main.js', root), 'utf8')

// Small DOM adapter exercises event behavior without a daemon, network, or browser dependencies.
function setup({ platform = 'MacIntel', clipboard = async () => {} } = {}) {
  const ids = new Map()
  function element(id, dataset = {}) {
    const attributes = new Map(), listeners = new Map(), classes = new Set()
    const node = { id, dataset, textContent: '', disabled: false,
      setAttribute: (key, value) => attributes.set(key, value),
      getAttribute: key => attributes.get(key),
      addEventListener: (name, callback) => listeners.set(name, callback),
      classList: { toggle: (name, enabled) => enabled ? classes.add(name) : classes.delete(name), contains: name => classes.has(name) },
      focus() { node.focused = true },
      showModal() { node.open = true }, close() { node.open = false },
      getBoundingClientRect: () => ({ top: 10, left: 10, bottom: 100, right: 100 }),
      async fire(name, extra = {}) { return listeners.get(name)?.({ target: node, currentTarget: node, preventDefault() {}, ...extra }) },
    }
    ids.set(id, node)
    return node
  }
  for (const match of html.matchAll(/\bid="([^"]+)"/g)) element(match[1])
  const modes = ['local', 'remote', 'socks'].map(mode => element(`tab-${mode}`, { mode }))
  const systems = ['mac', 'windows', 'linux'].map(os => element(`os-${os}`, { os }))
  const triggers = [...html.matchAll(/<button[^>]*data-open-connections/g)].map((_, index) => element(`connection-trigger-${index}`))
  ids.get('proxy-toggle').setAttribute('aria-pressed', 'false')
  ids.get('menubar-trigger').setAttribute('aria-expanded', 'true')
  runInNewContext(source, { document: {
    getElementById(id) { assert.ok(ids.has(id), `missing HTML element: ${id}`); return ids.get(id) },
    querySelectorAll(selector) { return { '[data-mode]': modes, '[data-os]': systems, '[data-open-connections]': triggers }[selector] },
  }, navigator: { platform, userAgent: platform, clipboard: { writeText: clipboard } } })
  return { ids, modes, systems, triggers, get: id => ids.get(id) }
}

test('landing page has local assets, valid anchors, unique IDs, and no live API dependency', () => {
  const ids = [...html.matchAll(/\bid="([^"]+)"/g)].map(match => match[1])
  assert.equal(new Set(ids).size, ids.length)
  for (const [, anchor] of html.matchAll(/href="#([^"]+)"/g)) assert.ok(ids.includes(anchor), `missing anchor ${anchor}`)
  for (const [, asset] of html.matchAll(/(?:href|src)="\.\/([^"]+)"/g)) assert.ok(existsSync(new URL(asset, root)), `missing asset ${asset}`)
  assert.match(html, /交互预览 · 示例数据/)
  assert.match(html, /href="https:\/\/github\.com\/raojinlin\/portway"[^>]+target="_blank"[^>]+rel="noopener noreferrer"/)
  assert.doesNotMatch(source, /\b(?:fetch|XMLHttpRequest|WebSocket)\s*\(/)
  assert.doesNotMatch(html, /(?:src|href)="https?:\/\/.*\.(?:js|css|woff)/)
  const css = readFileSync(new URL('style.css', root), 'utf8')
  assert.match(css, /prefers-reduced-motion/)
  assert.match(css, /max-width: 480px/)
  assert.match(css, /max-width: 360px/)
})

test('all forwarding modes update their route, command and tab accessibility', async () => {
  const h = setup()
  for (const [index, flag] of ['-L', '-R', '-D'].entries()) {
    await h.modes[index].fire('click')
    assert.ok(h.get('mode-command').textContent.includes(flag))
    assert.equal(h.modes[index].getAttribute('aria-selected'), 'true')
    assert.equal(h.get('mode-panel').getAttribute('aria-labelledby'), h.modes[index].id)
    assert.equal(h.modes.filter(node => node.tabIndex === 0).length, 1)
    assert.ok(h.get('route-from-address').textContent)
    assert.ok(h.get('route-to-address').textContent)
  }
})

test('tabs support arrow keys, Home and End with focus following selection', async () => {
  const h = setup()
  await h.modes[0].fire('keydown', { key: 'ArrowLeft' })
  assert.equal(h.modes[2].getAttribute('aria-selected'), 'true')
  assert.ok(h.modes[2].focused)
  await h.modes[2].fire('keydown', { key: 'ArrowRight' })
  assert.equal(h.modes[0].getAttribute('aria-selected'), 'true')
  await h.modes[0].fire('keydown', { key: 'End' })
  assert.equal(h.modes[2].getAttribute('aria-selected'), 'true')
  await h.modes[2].fire('keydown', { key: 'Home' })
  assert.equal(h.modes[0].getAttribute('aria-selected'), 'true')
})

test('demo stop and start keep count, status and action consistent', async () => {
  const h = setup(), button = h.get('proxy-toggle')
  await button.fire('click')
  assert.equal(h.get('running-count').textContent, '2')
  assert.match(button.textContent, /启动线路/)
  assert.ok(h.get('proxy-line').classList.contains('is-stopped'))
  assert.equal(h.get('proxy-dot').getAttribute('aria-label'), '已停止')
  await button.fire('click')
  assert.equal(h.get('running-count').textContent, '3')
  assert.match(button.textContent, /停止线路/)
  assert.equal(h.get('proxy-line').classList.contains('is-stopped'), false)
})

test('both connection links open the dialog; close and outside click dismiss it', async () => {
  const h = setup(), dialog = h.get('connections-dialog')
  for (const trigger of h.triggers) {
    await trigger.fire('click')
    assert.ok(dialog.open)
    await dialog.fire('click', { clientX: 20, clientY: 20 })
    assert.ok(dialog.open)
    await h.get('close-dialog').fire('click')
    assert.equal(dialog.open, false)
  }
  await h.triggers[0].fire('click')
  await dialog.fire('click', { clientX: 1, clientY: 1 })
  assert.equal(dialog.open, false)
})

test('copy emits only the documented sample command; denial exposes a manual fallback', async () => {
  let copied
  const h = setup({ clipboard: async value => { copied = value } })
  await h.get('copy-proxy').fire('click')
  assert.match(copied, /^export all_proxy='socks5h:\/\/127\.0\.0\.1:1080' http_proxy=/)
  assert.match(copied, /https_proxy=/)
  assert.doesNotMatch(copied, /ALL_PROXY/)
  assert.match(h.get('copy-feedback').textContent, /已复制示例/)
  assert.equal(h.get('copy-proxy').disabled, false)
  const denied = setup({ clipboard: async () => { throw new Error('denied') } })
  await denied.get('copy-proxy').fire('click')
  assert.match(denied.get('copy-feedback').textContent, /请手动复制：export/)
  assert.equal(denied.get('copy-proxy').disabled, false)
})

test('platform detection and manual override select installation instructions', async () => {
  for (const [platform, expected] of [['Win32', 'Windows'], ['Linux x86_64', 'Linux']]) {
    const h = setup({ platform })
    assert.equal(h.get('os-title').textContent, expected)
    await h.systems[0].fire('click')
    assert.equal(h.get('os-title').textContent, 'macOS')
    assert.equal(h.systems[0].getAttribute('aria-pressed'), 'true')
    assert.equal(h.systems.filter(node => node.getAttribute('aria-pressed') === 'true').length, 1)
  }
})

test('menubar can collapse, reopen and return focus to its trigger with Escape', async () => {
  const h = setup(), trigger = h.get('menubar-trigger'), panel = h.get('menubar-panel')
  await trigger.fire('click')
  assert.equal(panel.hidden, true)
  assert.equal(trigger.getAttribute('aria-expanded'), 'false')
  await trigger.fire('click')
  assert.equal(panel.hidden, false)
  await panel.fire('keydown', { key: 'Escape' })
  assert.equal(panel.hidden, true)
  assert.ok(trigger.focused)
})

test('menubar and hero share demo state; stopped SOCKS5 hides copy and clears its rate', async () => {
  const h = setup()
  await h.get('menubar-proxy-toggle').fire('click')
  assert.equal(h.get('menubar-copy-proxy').hidden, true)
  assert.equal(h.get('menubar-proxy-rate').textContent, '已停止')
  assert.equal(h.get('running-count').textContent, '2')
  assert.match(h.get('proxy-toggle').textContent, /启动线路/)
  await h.get('proxy-toggle').fire('click')
  assert.equal(h.get('menubar-copy-proxy').hidden, false)
  assert.equal(h.get('menubar-proxy-rate').textContent, '244 KB/s')
  assert.match(h.get('menubar-proxy-toggle').textContent, /停止线路/)
})

test('menubar copy uses the same proxy command and reports clipboard errors locally', async () => {
  let copied
  const h = setup({ clipboard: async value => { copied = value } })
  await h.get('menubar-copy-proxy').fire('click')
  assert.match(copied, /all_proxy=.*http_proxy=.*https_proxy=/)
  assert.match(h.get('menubar-feedback').textContent, /已复制示例/)
  const denied = setup({ clipboard: async () => { throw new Error('denied') } })
  await denied.get('menubar-copy-proxy').fire('click')
  assert.match(denied.get('menubar-feedback').textContent, /请手动复制/)
})
