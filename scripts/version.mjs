import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const config = JSON.parse(fs.readFileSync(path.join(root, 'desktop/wails.json'), 'utf8'))

// Tags are the release source in CI; local builds use the checked-in app version.
const candidate = process.env.PORTWAY_VERSION ||
  (process.env.GITHUB_REF_TYPE === 'tag' ? process.env.GITHUB_REF_NAME : '') ||
  config.info.productVersion
const version = candidate.replace(/^v/, '')
if (!/^[0-9A-Za-z][0-9A-Za-z.-]*$/.test(version)) throw new Error(`Invalid version: ${candidate}`)

export { version }
