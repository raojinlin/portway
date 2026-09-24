export type ProxyShell = 'posix' | 'powershell' | 'url'

export function proxyURL(listen: string): string {
  // Only accept host:port, never shell syntax or URL credentials from a saved address.
  const match = /^(\[[0-9a-fA-F:.]+\]|[a-zA-Z0-9._-]*):(\d+)$/.exec(listen.trim())
  if (!match || Number(match[2]) < 1 || Number(match[2]) > 65535) throw new Error('Invalid SOCKS5 listen address')
  let host = match[1]
  if (!host || host === '0.0.0.0') host = '127.0.0.1'
  if (host === '[::]') host = '[::1]'
  return `socks5h://${host}:${Number(match[2])}`
}

export function proxyCommand(url: string, shell: ProxyShell): string {
  if (shell === 'url') return url
  if (shell === 'powershell') return `$env:all_proxy='${url.split("'").join("''")}'; $env:http_proxy=$env:all_proxy; $env:https_proxy=$env:all_proxy`
  const quoted = url.split("'").join("'\\''")
  return `export all_proxy='${quoted}' http_proxy='${quoted}' https_proxy='${quoted}'`
}

export async function copyText(text: string): Promise<void> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return
    }
  } catch { /* HTTP dashboards and WebViews may require the legacy copy path. */ }
  const focused = document.activeElement as HTMLElement | null
  const input = document.createElement('textarea')
  input.value = text
  input.style.position = 'fixed'
  input.style.opacity = '0'
  document.body.appendChild(input)
  try {
    input.select()
    if (!document.execCommand('copy')) throw new Error('Clipboard unavailable')
  } finally {
    input.remove()
    focused?.focus()
  }
}
