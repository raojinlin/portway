import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const root = new URL('../site/', import.meta.url)
const routes = new Map([
  ['/', ['index.html', 'text/html; charset=utf-8']],
  ['/index.html', ['index.html', 'text/html; charset=utf-8']],
  ['/style.css', ['style.css', 'text/css; charset=utf-8']],
  ['/main.js', ['main.js', 'text/javascript; charset=utf-8']],
  ['/theme.js', ['theme.js', 'text/javascript; charset=utf-8']],
  ['/mark.svg', ['mark.svg', 'image/svg+xml']],
])
const port = Number(process.env.PORTWAY_SITE_PORT || 4173)
if (!Number.isInteger(port) || port < 1 || port > 65535) throw new Error('PORTWAY_SITE_PORT must be between 1 and 65535')
const server = createServer(async (request, response) => {
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    response.writeHead(405, { Allow: 'GET, HEAD' }).end()
    return
  }
  const route = routes.get(new URL(request.url, 'http://localhost').pathname)
  if (!route) { response.writeHead(404).end('Not found'); return }
  try {
    const data = await readFile(new URL(route[0], root))
    response.writeHead(200, { 'Content-Type': route[1], 'Cache-Control': 'no-store', 'X-Content-Type-Options': 'nosniff' })
    response.end(request.method === 'HEAD' ? undefined : data)
  } catch (error) {
    console.error(error.message)
    response.writeHead(500).end('Unable to read page')
  }
})
server.on('error', (error) => { console.error(error.message); process.exitCode = 1 })
server.listen(port, '127.0.0.1', () => console.log(`Portway landing page: http://127.0.0.1:${port}\nServing ${fileURLToPath(root)}`))
