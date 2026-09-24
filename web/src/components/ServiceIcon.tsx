import { CodeOutlined, DatabaseOutlined, DesktopOutlined, GlobalOutlined, LinkOutlined, LockOutlined } from '@ant-design/icons'
import { serviceNames, type ServiceIconKind } from '../serviceIcon'

const tags: Partial<Record<ServiceIconKind, string>> = { mysql: 'MY', postgresql: 'PG', redis: 'R', mongodb: 'M' }

export default function ServiceIcon({ kind }: { kind: ServiceIconKind }) {
  const Icon = kind === 'ssh' ? CodeOutlined : kind === 'https' ? LockOutlined : kind === 'rdp' ? DesktopOutlined
    : ['mysql', 'postgresql', 'redis', 'mongodb'].includes(kind) ? DatabaseOutlined
    : kind === 'http' || kind === 'socks5' ? GlobalOutlined : LinkOutlined
  const tag = tags[kind]
  return <span className={`service-icon service-icon--${kind}`} role="img" aria-label={serviceNames[kind]}>
    <Icon aria-hidden="true" />{tag && <small aria-hidden="true">{tag}</small>}
  </span>
}
