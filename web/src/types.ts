// Mirrors internal/daemon/api.go's JSON wire format exactly. tunnel.Status
// has no json tags, so its fields keep their Go (capitalized) names on the
// wire; TunnelView's own fields and TunnelRequest are snake_case. Field
// names here must stay in sync with the Go structs.

export type Direction = 'local' | 'remote' | 'dynamic'

export type TunnelState = 'starting' | 'running' | 'stopped' | 'error'

export interface TunnelView {
  service_icon?: string
  resolved_service_icon?: string
  // from tunnel.Status (embedded, no json tags -> Go field names)
  Name: string
  State: TunnelState
  LastError: string
  BytesIn: number
  BytesOut: number
  ActiveConns: number
  StartedAt: string

  // TunnelView's own fields
  enabled: boolean
  direction: Direction
  local_listen: string
  ssh_address: string
  ssh_user: string
  ssh_key_path: string
  ssh_config_path: string
  remote_listen: string
  forward_address: string
  known_hosts_path: string
  trust_new_host_key: boolean
  insecure_skip_host_key_check: boolean
  keep_alive: string
  reconnect_delay: string
}

export interface TunnelRequest {
  service_icon?: string
  name: string
  direction?: Direction
  local_listen?: string
  ssh_address: string
  ssh_user?: string
  ssh_key_path?: string
  ssh_password?: string
  ssh_config_path?: string
  remote_listen?: string
  forward_address?: string
  known_hosts_path?: string
  trust_new_host_key?: boolean
  insecure_skip_host_key_check?: boolean
  keep_alive?: string
  reconnect_delay?: string
}

export interface ApiError {
  error: string
}

export interface TunnelConnection {
  id: string
  source: string
  target: string
  state: 'connecting' | 'connected' | 'closed' | 'failed'
  started_at: string
  age_seconds: number
  bytes_in: number
  bytes_out: number
  ended_at?: string
  error?: string
}

export interface ConnectionHistory {
  connections: TunnelConnection[]
  limit: number
  persisted: boolean
  persistence_error?: string
}

export interface RuntimeLogEntry {
  id: string
  time: string
  level: string
  message: string
  tunnel: string
  raw: string
}

export interface RuntimeLogs {
  entries: RuntimeLogEntry[]
  enabled: boolean
  path: string
  limit: number
  truncated: boolean
}

export interface ActivityConnection extends TunnelConnection { tunnel: string }

export interface ConnectionsView {
  connections: ActivityConnection[]
  tunnels: string[]
  total: number
  limit: number
  persisted: boolean
  persistence_error?: string
}

export interface DaemonConfig {
  addr: string
  state_file: string
  log_level: 'debug' | 'info' | 'warn' | 'error'
  log_format: 'text' | 'json'
  log_file: string
  connection_log: string
  log_max_size_mb: number
  log_max_backups: number
  mcp: MCPConfig
}

export interface MCPConfig {
  enabled: boolean
  addr: string
  access: 'read' | 'operate' | 'manage'
  auth: 'token' | 'oauth' | 'both'
  token: string
}

export interface MCPStatus {
  enabled: boolean
  running: boolean
  addr: string
  url: string
  access: string
  auth: string
  error?: string
}

export interface DaemonConfigView {
  desktop: boolean
  config: DaemonConfig
  effective: DaemonConfig
  path: string
  exists: boolean
  restart_required: boolean
  mcp_status: MCPStatus
}
export interface AutostartStatus {
  supported: boolean
  enabled: boolean
  needs_update: boolean
  reason?: string
}
