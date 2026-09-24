export const serviceIcons = ['generic', 'ssh', 'mysql', 'postgresql', 'http', 'https', 'redis', 'mongodb', 'rdp', 'socks5'] as const
export type ServiceIconKind = typeof serviceIcons[number]

export const serviceNames: Record<ServiceIconKind, string> = {
  generic: 'TCP', ssh: 'SSH', mysql: 'MySQL', postgresql: 'PostgreSQL', http: 'HTTP', https: 'HTTPS',
  redis: 'Redis', mongodb: 'MongoDB', rdp: 'RDP', socks5: 'SOCKS5',
}

export function resolveServiceIcon(preference: string | undefined, direction: string, target: string): ServiceIconKind {
  if (serviceIcons.includes(preference as ServiceIconKind)) return preference as ServiceIconKind
  if (direction === 'dynamic') return 'socks5'
  const match = /^(?:\[[^\]]+\]|[^:]+):(\d+)$/.exec(target)
  const ports: Record<number, ServiceIconKind> = {
    22: 'ssh', 3306: 'mysql', 5432: 'postgresql', 80: 'http', 8080: 'http', 8000: 'http', 3000: 'http',
    443: 'https', 8443: 'https', 6379: 'redis', 27017: 'mongodb', 3389: 'rdp',
  }
  return match ? ports[Number(match[1])] ?? 'generic' : 'generic'
}
