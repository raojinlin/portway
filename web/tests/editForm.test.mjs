import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { runInNewContext } from 'node:vm'
import ts from 'typescript'

const source = readFileSync(new URL('../src/components/AddLineModal.tsx', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2020, jsx: ts.JsxEmit.ReactJSX },
}).outputText
const component = {}
runInNewContext(compiled, { exports: component, require: () => ({}) })

test('edit form restores all persisted connection settings', () => {
  const tunnel = {
    Name: 'database',
    direction: 'local',
    service_icon: 'postgres',
    local_listen: '127.0.0.1:5432',
    ssh_address: 'gateway',
    remote_listen: '',
    forward_address: 'db.internal:5432',
    ssh_user: 'alice',
    ssh_key_path: '~/.ssh/work',
    ssh_config_path: '~/.ssh/config.work',
    known_hosts_path: '~/.ssh/known_hosts.work',
    keep_alive: '45s',
    reconnect_delay: '2s',
    trust_new_host_key: true,
    insecure_skip_host_key_check: false,
  }

  assert.deepEqual(JSON.parse(JSON.stringify(component.formValuesForTunnel(tunnel))), {
    name: 'database',
    direction: 'local',
    service_icon: 'postgres',
    local_listen: '127.0.0.1:5432',
    ssh_address: 'gateway',
    remote_listen: '',
    forward_address: 'db.internal:5432',
    ssh_user: 'alice',
    ssh_key_path: '~/.ssh/work',
    ssh_config_path: '~/.ssh/config.work',
    known_hosts_path: '~/.ssh/known_hosts.work',
    keep_alive: '45s',
    reconnect_delay: '2s',
    trust_new_host_key: true,
    insecure_skip_host_key_check: false,
  })
})
