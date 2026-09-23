import { useEffect, useRef, useState } from 'react'
import {
  CaretRightOutlined,
  DeleteOutlined,
  EditOutlined,
  MoreOutlined,
  PauseOutlined,
  RightOutlined,
  WarningOutlined,
} from '@ant-design/icons'
import { App as AntApp, Button, Dropdown, Tooltip, type MenuProps } from 'antd'
import { api } from '../api'
import type { TunnelState, TunnelView } from '../types'
import { humanBytes } from '../format'
import ConnectionModal from './ConnectionModal'

const STATUS_LABEL: Record<TunnelState, string> = {
  running: '运行中',
  starting: '连接中',
  error: '异常',
  stopped: '已停止',
}

const DIRECTION_LABEL = { local: '本地转发', remote: '远程转发', dynamic: '动态代理' }

function routeParts(t: TunnelView): Array<{ label: string; value: string }> {
  if (t.direction === 'remote') {
    return [
      { label: '远程监听', value: t.remote_listen || '?' },
      { label: 'SSH 服务', value: t.ssh_address },
      { label: '本机目标', value: t.forward_address },
    ]
  }
  if (t.direction === 'dynamic') {
    return [
      { label: 'SOCKS5 监听', value: t.local_listen },
      { label: 'SSH 跳板', value: t.ssh_address },
      { label: '目标', value: '由客户端指定' },
    ]
  }
  return [
    { label: '本地监听', value: t.local_listen },
    { label: 'SSH 跳板', value: t.ssh_address },
    { label: '目标服务', value: t.forward_address },
  ]
}

interface Props {
  tunnel: TunnelView
  onChanged: () => void
  onEdit: (tunnel: TunnelView) => void
}

export default function LineRow({ tunnel: t, onChanged, onEdit }: Props) {
  const { message, modal } = AntApp.useApp()
  const [busy, setBusy] = useState(false)
  const [errorOpen, setErrorOpen] = useState(false)
  const [connectionsOpen, setConnectionsOpen] = useState(false)
  const [hasTraffic, setHasTraffic] = useState(false)
  const previousSample = useRef<TunnelView | null>(null)
  const status: TunnelState = STATUS_LABEL[t.State] ? t.State : 'stopped'

  useEffect(() => {
    const previous = previousSample.current
    previousSample.current = t
    // Compare each poll, not connection count: an open connection may be idle.
    const active = Boolean(
      previous && t.enabled && t.State === 'running' &&
      t.Name === previous.Name && t.StartedAt === previous.StartedAt &&
      t.BytesIn >= previous.BytesIn && t.BytesOut >= previous.BytesOut &&
      (t.BytesIn > previous.BytesIn || t.BytesOut > previous.BytesOut),
    )
    setHasTraffic(active)
    if (!active) return
    // Expire stale activity if the next 2-second poll fails or is delayed.
    const timeout = setTimeout(() => setHasTraffic(false), 3000)
    return () => clearTimeout(timeout)
  }, [t])

  const trafficActive = hasTraffic && t.enabled && status === 'running'

  async function run(action: () => Promise<unknown>, failMsg: string) {
    setBusy(true)
    try {
      await action()
      onChanged()
    } catch (err) {
      message.error(`${failMsg}：${(err as Error).message}`)
    } finally {
      setBusy(false)
    }
  }

  const menuItems: MenuProps['items'] = [
    { key: 'delete', icon: <DeleteOutlined />, danger: true, label: '删除线路' },
  ]

  function onMenuClick({ key }: { key: string }) {
    if (key === 'delete') {
      modal.confirm({
        title: `删除线路 “${t.Name}”？`,
        content: '保存的配置也会一并删除，此操作无法撤销。',
        okText: '删除',
        cancelText: '取消',
        okButtonProps: { danger: true },
        onOk: () => run(() => api.remove(t.Name), '删除失败'),
      })
    }
  }

  return (
    <article className="line-row">
      <div className="line-card__header">
        <div className="line-card__heading">
          <div className="line-identity">
            <div>
              <div className="line-title-row">
                <h3>{t.Name}</h3>
                <span className="direction-chip">{DIRECTION_LABEL[t.direction] ?? DIRECTION_LABEL.local}</span>
              </div>
              <span
                className={`status-pill status-pill--${status}${trafficActive ? ' status-pill--traffic' : ''}`}
                title={trafficActive ? '有数据传输' : STATUS_LABEL[status]}
                aria-label={trafficActive ? '运行中，有数据传输' : STATUS_LABEL[status]}
              ><i aria-hidden="true" />{STATUS_LABEL[status]}</span>
            </div>
          </div>

          <div className="route-strip" aria-label="转发路径">
            {routeParts(t).map((part, i, parts) => (
              <div className="route-fragment" key={`${part.label}-${i}`}>
                <Tooltip title={`${part.label}：${part.value || '—'}`} trigger={['hover', 'focus', 'click']}>
                  <strong className="route-node" tabIndex={0} aria-label={`${part.label}：${part.value || '—'}`}>
                    {part.value || '—'}
                  </strong>
                </Tooltip>
                {i < parts.length - 1 && <RightOutlined className="route-arrow" aria-hidden="true" />}
              </div>
            ))}
          </div>
        </div>

        <div className="line-actions">
          <Button type="text" icon={<EditOutlined />} disabled={busy} onClick={() => onEdit(t)}>编辑</Button>
          {t.enabled ? (
            <Button icon={<PauseOutlined />} loading={busy} onClick={() => run(() => api.stop(t.Name), '停止失败')}>停止</Button>
          ) : (
            <Button type="primary" icon={<CaretRightOutlined />} loading={busy} onClick={() => run(() => api.start(t.Name), '启动失败')}>启动</Button>
          )}
          <Dropdown menu={{ items: menuItems, onClick: onMenuClick }} trigger={['click']} placement="bottomRight">
            <Button icon={<MoreOutlined />} disabled={busy} aria-label="更多操作" />
          </Dropdown>
        </div>
      </div>

      <div className="line-card__footer">
        <Tooltip title="本次启动以来，daemon 所在机器通过隧道收到的累计业务数据，不含 SSH 协议开销。每 2 秒刷新；停止或重新启动后清零。" trigger={['hover', 'focus', 'click']}>
          <div className="metric" tabIndex={0}><span>累计接收</span><strong>↓ {humanBytes(t.BytesIn)}</strong></div>
        </Tooltip>
        <Tooltip title="本次启动以来，daemon 所在机器通过隧道发出的累计业务数据，不是实时速度。每 2 秒刷新；停止或重新启动后清零。" trigger={['hover', 'focus', 'click']}>
          <div className="metric" tabIndex={0}><span>累计发送</span><strong>↑ {humanBytes(t.BytesOut)}</strong></div>
        </Tooltip>
        <Tooltip title="点击查看每条连接的来源、目标和流量。包含正在连接目标的请求，不包含 SSH 控制连接。" trigger={['hover', 'focus']}>
          <button className="metric metric--connections" type="button" aria-haspopup="dialog" onClick={() => setConnectionsOpen(true)}>
            <span>当前连接</span><strong>{t.ActiveConns || 0}</strong><RightOutlined aria-hidden="true" />
          </button>
        </Tooltip>
        {t.LastError && (
          <button className="error-summary" type="button" aria-expanded={errorOpen} onClick={() => setErrorOpen((open) => !open)}>
            <WarningOutlined />
            <span>{t.LastError}</span>
            <RightOutlined className={errorOpen ? 'is-open' : ''} />
          </button>
        )}
      </div>
      {t.LastError && errorOpen && <div className="error-detail">{t.LastError}</div>}
      {connectionsOpen && <ConnectionModal name={t.Name} showHistory={t.direction === 'dynamic'} onClose={() => setConnectionsOpen(false)} />}
    </article>
  )
}
