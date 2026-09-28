import assert from 'node:assert/strict'
import test from 'node:test'

test('version helper falls back to the desktop package version', async () => {
  const module = await import('../version.mjs')
  assert.equal(module.version, '0.1.0')
})
