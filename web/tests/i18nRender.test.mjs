import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import { runInNewContext } from 'node:vm'
import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { App as AntApp } from 'antd'
import ts from 'typescript'

const root = fileURLToPath(new URL('../src/', import.meta.url))
const require = createRequire(import.meta.url)
function ui(language, queryState) {
  const cache = new Map()
  function load(filename) {
    if (filename.endsWith('/useLiveQuery.ts') && queryState) return { useLiveQuery: () => queryState }
    if (cache.has(filename)) return cache.get(filename)
    if (filename.endsWith('.json')) return { default: JSON.parse(readFileSync(filename, 'utf8')) }
    const exports = {}
    cache.set(filename, exports)
    const code = ts.transpileModule(readFileSync(filename, 'utf8'), {
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
    }).outputText
    runInNewContext(code, {
      exports,
      require: id => {
        if (!id.startsWith('.')) return require(id)
        const resolved = path.resolve(path.dirname(filename), id)
        if (path.extname(resolved)) return load(resolved)
        return load(resolved + (existsSync(resolved + '.tsx') ? '.tsx' : '.ts'))
      },
      window: { localStorage: { getItem: () => language }, matchMedia: () => ({ matches: false }) },
      navigator: { languages: [language], language, platform: 'MacIntel' },
    }, { filename })
    return exports
  }
  const { I18nProvider } = load(path.join(root, 'I18n.tsx'))
  const { ThemeModeProvider } = load(path.join(root, 'ThemeMode.tsx'))
  return (component, props = {}) => renderToStaticMarkup(React.createElement(I18nProvider, null,
    React.createElement(ThemeModeProvider, null, React.createElement(AntApp, null,
      React.createElement(load(path.join(root, component)).default, props)))))
}

test('Chinese and English pages render translated buttons and SOCKS routes without a daemon', () => {
  for (const language of ['zh', 'en']) {
    const render = ui(language)
    const header = render('components/Header.tsx', { tunnels: [], onAddClick() {}, onConfigClick() {} })
    assert.ok(header.includes(language === 'en' ? 'New tunnel' : '新建线路'))
    assert.ok(header.includes(language === 'en' ? 'Settings' : '配置'))
    const languageIndex = header.indexOf(language === 'en' ? 'Language' : '切换语言')
    const themeIndex = header.indexOf(language === 'en' ? 'Choose theme' : '选择主题')
    const configIndex = header.indexOf(language === 'en' ? 'Open settings' : '打开配置')
    assert.ok(languageIndex < themeIndex && themeIndex < configIndex, 'theme must immediately follow language, before settings')
    const line = render('components/LineRow.tsx', {
      tunnel: { Name: 'example', State: 'running', direction: 'dynamic', local_listen: '127.0.0.1:1080', ssh_address: 'gateway', enabled: true, BytesIn: 1024, BytesOut: 512 },
      onChanged() {}, onEdit() {},
    })
    assert.ok(line.includes(language === 'en' ? 'Copy proxy command' : '复制代理命令'))
    assert.ok(line.includes(language === 'en' ? 'Specified by client' : '由客户端指定'))
    assert.ok(line.includes(language === 'en' ? 'Running' : '运行中'))
    if (language === 'en') assert.doesNotMatch(line, /[\p{Script=Han}]/u)
  }
})

test('proxy copy action is hidden for stopped lines and non-SOCKS routes', () => {
  for (const language of ['zh', 'en']) {
    const render = ui(language)
    for (const direction of ['local', 'remote', 'dynamic']) {
      for (const state of ['running', 'stopped', 'starting', 'error', 'unknown']) {
        const html = render('components/LineRow.tsx', {
          tunnel: { Name: 'example', State: state, direction, local_listen: '127.0.0.1:1080', ssh_address: 'gateway', enabled: state !== 'stopped', BytesIn: 0, BytesOut: 0 },
          onChanged() {}, onEdit() {},
        })
        const expected = direction === 'dynamic' && state !== 'stopped' && state !== 'unknown'
        assert.equal(html.includes(language === 'en' ? 'Copy proxy command' : '复制代理命令'), expected, `${language}/${direction}/${state}`)
      }
    }
  }
})

test('autostart setting renders localized label and immediate-save guidance', () => {
  for (const language of ['zh', 'en']) {
    const html = ui(language)('components/AutostartSettings.tsx')
    assert.ok(html.includes(language === 'en' ? 'Start at login' : '开机自启'))
    assert.ok(html.includes(language === 'en' ? 'Changes save immediately' : '开关立即保存'))
    assert.ok(html.includes('role="switch"'))
    assert.ok(html.includes('aria-checked="false"'))
    if (language === 'en') assert.doesNotMatch(html, /[\p{Script=Han}]/u)
  }
})

test('line service badges render automatic and manual choices', () => {
  const render = ui('en')
  for (const [preference, target, expected] of [['auto', 'localhost:5432', 'PostgreSQL'], ['mysql', 'localhost:9999', 'MySQL'], ['', 'localhost:22', 'SSH']]) {
    const html = render('components/LineRow.tsx', {
      tunnel: { Name: 'service', State: 'stopped', direction: 'local', service_icon: preference, forward_address: target, local_listen: 'localhost:8080', ssh_address: 'gateway', enabled: false, BytesIn: 0, BytesOut: 0 },
      onChanged() {}, onEdit() {},
    })
    assert.ok(html.includes(`aria-label="${expected}"`))
  }
})

test('activity page renders translated log filters, file warnings and escaped messages', () => {
  const snapshot = { loading: false, error: '', updatedAt: new Date('2026-09-24T00:00:00Z'), refresh() {}, data: { kind: 'logs', result: {
    enabled: true, path: '/logs/daemon.log', limit: 500, truncated: true,
    entries: [{ id: '1', time: '2026-09-24T00:00:00Z', level: 'WARN', tunnel: 'socks', message: '<script>bad()</script>', raw: 'level=WARN msg=test' }],
  } } }
  for (const language of ['zh', 'en']) {
    const html = ui(language, snapshot)('components/ActivityPage.tsx', { tunnels: [], onConfigClick() {} })
    assert.ok(html.includes(language === 'en' ? 'Runtime logs' : '运行日志'))
    assert.ok(html.includes(language === 'en' ? 'SOCKS5 history' : 'SOCKS5 历史'))
    assert.ok(html.includes('/logs/daemon.log'))
    assert.ok(html.includes('&lt;script&gt;bad()&lt;/script&gt;'))
    assert.ok(!html.includes('<script>bad()</script>'))
    if (language === 'en') assert.doesNotMatch(html, /[\p{Script=Han}]/u)
  }
  snapshot.data.result.enabled = false
  const html = ui('en', snapshot)('components/ActivityPage.tsx', { tunnels: [], onConfigClick() {} })
  assert.ok(html.includes('Logging to stderr only'))
  assert.ok(html.includes('Open settings'))
})
