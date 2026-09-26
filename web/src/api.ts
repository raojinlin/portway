import type { ApiError, ConnectionHistory, ConnectionsView, RuntimeLogs, DaemonConfig, DaemonConfigView, TunnelConnection, TunnelRequest, TunnelView } from './types'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    let msg = res.statusText
    try {
      const body = (await res.json()) as ApiError
      if (body.error) msg = body.error
    } catch {
      // response wasn't JSON; fall back to statusText
    }
    throw new Error(msg)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

async function download(path: string): Promise<Blob> {
  const res = await fetch(path, { cache: 'no-store' })
  if (!res.ok) throw new Error(res.statusText)
  return res.blob()
}

export const api = {
  autostart: (signal?: AbortSignal) => request<import('./types').AutostartStatus>('/api/desktop/autostart', { signal, cache: 'no-store' }),
  setAutostart: (enabled: boolean, signal?: AbortSignal) => request<import('./types').AutostartStatus>('/api/desktop/autostart', {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ enabled }), signal,
  }),
  logs: (query: URLSearchParams, signal?: AbortSignal) => request<RuntimeLogs>(`/api/logs?${query}`, { signal, cache: 'no-store' }),

  allConnections: (query: URLSearchParams, signal?: AbortSignal) => request<ConnectionsView>(`/api/connections?${query}`, { signal, cache: 'no-store' }),
  config: (signal?: AbortSignal) => request<DaemonConfigView>('/api/config', { signal, cache: 'no-store' }),

  saveConfig: (body: DaemonConfig) => request<DaemonConfigView>('/api/config', {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }),

  downloadSkill: () => download('/api/skills/portway/download'),
  saveSkillDesktop: () => request<{ path: string } | undefined>('/api/desktop/skills/portway/save', { method: 'POST' }),

  list: () => request<TunnelView[]>('/api/tunnels'),

  get: (name: string) => request<TunnelView>(`/api/tunnels/${encodeURIComponent(name)}`),

  connections: (name: string, signal?: AbortSignal) =>
    request<TunnelConnection[]>(`/api/tunnels/${encodeURIComponent(name)}/connections`, { signal, cache: 'no-store' }),

  connectionHistory: (name: string, signal?: AbortSignal) =>
    request<ConnectionHistory>(`/api/tunnels/${encodeURIComponent(name)}/connections/history`, { signal, cache: 'no-store' }),

  create: (body: TunnelRequest) =>
    request<TunnelView>('/api/tunnels', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),

  update: (name: string, body: TunnelRequest) =>
    request<TunnelView>(`/api/tunnels/${encodeURIComponent(name)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }),

  remove: (name: string) =>
    request<void>(`/api/tunnels/${encodeURIComponent(name)}`, { method: 'DELETE' }),

  start: (name: string) =>
    request<TunnelView>(`/api/tunnels/${encodeURIComponent(name)}/start`, { method: 'POST' }),

  stop: (name: string) =>
    request<TunnelView>(`/api/tunnels/${encodeURIComponent(name)}/stop`, { method: 'POST' }),
}
