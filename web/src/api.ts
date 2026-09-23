import type { ApiError, ConnectionHistory, DaemonConfig, DaemonConfigView, TunnelConnection, TunnelRequest, TunnelView } from './types'

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

export const api = {
  config: (signal?: AbortSignal) => request<DaemonConfigView>('/api/config', { signal, cache: 'no-store' }),

  saveConfig: (body: DaemonConfig) => request<DaemonConfigView>('/api/config', {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  }),

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
