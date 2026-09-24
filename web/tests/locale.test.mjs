import assert from 'node:assert/strict'
import { readFileSync, readdirSync } from 'node:fs'
import test from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

const english = JSON.parse(readFileSync(new URL('../src/locales/en.json', import.meta.url), 'utf8'))
const source = readFileSync(new URL('../src/locale.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020 } }).outputText
function setup(saved) {
  const api = {}
  const window = { localStorage: { getItem: () => saved } }
  runInNewContext(compiled, { exports: api, require: () => ({ default: english }), window })
  return { api, window }
}

test('auto-detects supported preferred languages, falls back to English, respects manual choice', () => {
  const { api } = setup()
  for (const [languages, expected] of [
    [['zh-CN', 'en-US'], 'zh'], [['zh-TW'], 'zh'], [['zh_HK'], 'zh'], [['en-GB', 'zh'], 'en'],
    [['fr-FR', 'zh-Hans'], 'zh'], [['fr'], 'en'], [[], 'en'], [['EN-us'], 'en'],
  ]) assert.equal(api.resolveLanguage('system', languages), expected)
  assert.equal(api.resolveLanguage('en', ['zh-CN']), 'en')
  assert.equal(api.resolveLanguage('zh', ['en-US']), 'zh')
})

test('restores language preferences and tolerates unavailable storage', () => {
  for (const saved of ['zh', 'en', 'system', null, 'invalid']) {
    assert.equal(setup(saved).api.loadLanguagePreference(), ['zh', 'en'].includes(saved) ? saved : 'system')
  }
  const { api, window } = setup('zh')
  Object.defineProperty(window, 'localStorage', { get() { throw new Error('blocked') } })
  assert.equal(api.loadLanguagePreference(), 'system')
})

test('both languages preserve every interpolation parameter and raw user values', () => {
  const { api } = setup()
  const placeholders = text => [...text.matchAll(/\{(\w+)\}/g)].map(match => match[1]).sort()
  for (const [key, value] of Object.entries(english)) {
    assert.ok(value.trim(), `Empty translation: ${key}`)
    assert.deepEqual(placeholders(value), placeholders(key), key)
    assert.equal(api.translator('zh')(key), key)
    assert.equal(api.translator('en')(key), value)
  }
  assert.equal(api.translator('en')('编辑 {name}', { name: '<test>{name}' }), 'Edit <test>{name}')
  assert.equal(api.translator('en')('{count} 条', { count: 0 }), '0 total')
})

test('visible Chinese strings in components must go through the translation catalog', () => {
  const directory = new URL('../src/components/', import.meta.url)
  for (const file of readdirSync(directory).filter(name => name.endsWith('.tsx'))) {
    const text = readFileSync(new URL(file, directory), 'utf8')
    const ast = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
    const visit = (node, translated = false) => {
      if (ts.isCallExpression(node) && node.expression.getText(ast) === 'tr') translated = true
      if ((ts.isStringLiteral(node) || ts.isJsxText(node) || ts.isTemplateLiteralToken(node)) && /\p{Script=Han}/u.test(node.text)) {
        assert.ok(translated, `Untranslated text in ${file}: ${node.text}`)
        if (ts.isStringLiteral(node)) assert.ok(Object.hasOwn(english, node.text), `Missing translation: ${node.text}`)
      }
      ts.forEachChild(node, child => visit(child, translated))
    }
    visit(ast)
  }
})
