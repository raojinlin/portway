import { useEffect, useRef, useState } from 'react'
import {
  CaretRightOutlined,
  CopyOutlined,
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
import { useI18n } from '../I18n'
import type { Translate } from '../locale'
import ProxyCommandModal from './ProxyCommandModal'
import ServiceIcon from './ServiceIcon'
import { resolveServiceIcon, serviceNames } from '../serviceIcon'

const statusLabels = (tr: Translate): Record<TunnelState, string> => ({
  running: tr("运行中"),
  starting: tr("连接中"),
  error: tr("异常"),
  stopped: tr("已停止"),
})

const directionLabels = (tr: Translate) => ({ local: tr("本地转发"), remote: tr("远程转发"), dynamic: tr("动态代理") })

function routeParts(t: TunnelView, tr: Translate): Array<{ label: string; value: string }> {
  if (t.direction === 'remote') {
    return [
      { label: tr("远程监听"), value: t.remote_listen || '?' },
      { label: tr("SSH 服务"), value: t.ssh_address },
      { label: tr("本机目标"), value: t.forward_address },
    ]
  }
  if (t.direction === 'dynamic') {
    return [
      { label: tr("SOCKS5 监听"), value: t.local_listen },
      { label: tr("SSH 跳板"), value: t.ssh_address },
      { label: tr("目标"), value: tr("由客户端指定") },
    ]
  }
  return [
    { label: tr("本地监听"), value: t.local_listen },
    { label: tr("SSH 跳板"), value: t.ssh_address },
    { label: tr("目标服务"), value: t.forward_address },
  ]
}

interface Props {
  tunnel: TunnelView
  onChanged: () => void
  onEdit: (tunnel: TunnelView) => void
}

export default function LineRow({ tunnel: t, onChanged, onEdit }: Props) {
  const { tr } = useI18n()
  const STATUS_LABEL = statusLabels(tr)
  const service = resolveServiceIcon(t.service_icon, t.direction, t.forward_address || '')
  const DIRECTION_LABEL = directionLabels(tr)
  const [proxyOpen, setProxyOpen] = useState(false)
  const { message, modal } = AntApp.useApp()
  const [busy, setBusy] = useState(false)
  const [errorOpen, setErrorOpen] = useState(false)
  const [connectionsOpen, setConnectionsOpen] = useState(false)
  const [hasTraffic, setHasTraffic] = useState(false)
  const previousSample = useRef<TunnelView | null>(null)
  const status: TunnelState = STATUS_LABEL[t.State] ? t.State : 'stopped'
  const canCopyProxy = t.direction === 'dynamic' && status !== 'stopped'

  useEffect(() => {
    if (!canCopyProxy) setProxyOpen(false)
  }, [canCopyProxy])

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
    { key: 'delete', icon: <DeleteOutlined />, danger: true, label: tr("删除线路") },
  ]

  function onMenuClick({ key }: { key: string }) {
    if (key === 'delete') {
      modal.confirm({
        title: tr('删除线路 “{name}”？', { name: t.Name }),
        content: tr("保存的配置也会一并删除，此操作无法撤销。"),
        okText: tr("删除"),
        cancelText: tr("取消"),
        okButtonProps: { danger: true },
        onOk: () => run(() => api.remove(t.Name), tr("删除失败")),
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
                <Tooltip title={serviceNames[service]}><span><ServiceIcon kind={service} /></span></Tooltip>
                <h3>{t.Name}</h3>
                <span className="direction-chip">{DIRECTION_LABEL[t.direction] ?? DIRECTION_LABEL.local}</span>
              </div>
              <span
                className={`status-pill status-pill--${status}${trafficActive ? ' status-pill--traffic' : ''}`}
                title={trafficActive ? tr("有数据传输") : STATUS_LABEL[status]}
                aria-label={trafficActive ? tr("运行中，有数据传输") : STATUS_LABEL[status]}
              ><i aria-hidden="true" />{STATUS_LABEL[status]}</span>
            </div>
          </div>

          <div className="route-strip" aria-label={tr("转发路径")}>
            {routeParts(t, tr).map((part, i, parts) => (
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
          {canCopyProxy && <Tooltip title={tr('复制代理命令')}>
            <Button type="text" icon={<CopyOutlined />} aria-label={tr('复制代理命令')} onClick={() => setProxyOpen(true)} />
          </Tooltip>}
          <Button type="text" icon={<EditOutlined />} disabled={busy} onClick={() => onEdit(t)}>{tr("编辑")}</Button>
          {t.enabled ? (
            <Button icon={<PauseOutlined />} loading={busy} onClick={() => run(() => api.stop(t.Name), tr("停止失败"))}>{tr("停止")}</Button>
          ) : (
            <Button type="primary" icon={<CaretRightOutlined />} loading={busy} onClick={() => run(() => api.start(t.Name), tr("启动失败"))}>{tr("启动")}</Button>
          )}
          <Dropdown menu={{ items: menuItems, onClick: onMenuClick }} trigger={['click']} placement="bottomRight">
            <Button icon={<MoreOutlined />} disabled={busy} aria-label={tr("更多操作")} />
          </Dropdown>
        </div>
      </div>

      <div className="line-card__footer">
        <Tooltip title={tr("本次启动以来，daemon 所在机器通过隧道收到的累计业务数据，不含 SSH 协议开销。每 2 秒刷新；停止或重新启动后清零。")} trigger={['hover', 'focus', 'click']}>
          <div className="metric" tabIndex={0}><span>{tr("累计接收")}</span><strong>↓ {humanBytes(t.BytesIn)}</strong></div>
        </Tooltip>
        <Tooltip title={tr("本次启动以来，daemon 所在机器通过隧道发出的累计业务数据，不是实时速度。每 2 秒刷新；停止或重新启动后清零。")} trigger={['hover', 'focus', 'click']}>
          <div className="metric" tabIndex={0}><span>{tr("累计发送")}</span><strong>↑ {humanBytes(t.BytesOut)}</strong></div>
        </Tooltip>
        <Tooltip title={tr("点击查看每条连接的来源、目标和流量。包含正在连接目标的请求，不包含 SSH 控制连接。")} trigger={['hover', 'focus']}>
          <button className="metric metric--connections" type="button" aria-haspopup="dialog" onClick={() => setConnectionsOpen(true)}>
            <span>{tr("当前连接")}</span><strong>{t.ActiveConns || 0}</strong><RightOutlined aria-hidden="true" />
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
      {canCopyProxy && proxyOpen && <ProxyCommandModal tunnel={t} onClose={() => setProxyOpen(false)} />}
    </article>
  )
}
