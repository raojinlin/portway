import { useEffect, useState } from 'react'
import { Alert, Button, Modal, Table, Tabs, Tooltip } from 'antd'
import type { TableColumnsType } from 'antd'
import { api } from '../api'
import { humanBytes } from '../format'
import type { TunnelConnection } from '../types'
import { useI18n } from '../I18n'
import type { Translate } from '../locale'

function duration(seconds: number, tr: Translate): string {
  const s = Math.max(0, seconds)
  if (s < 60) return tr('{seconds} 秒', { seconds: s })
  if (s < 3600) return tr('{minutes} 分 {seconds} 秒', { minutes: Math.floor(s / 60), seconds: s % 60 })
  return tr('{hours} 小时 {minutes} 分', { hours: Math.floor(s / 3600), minutes: Math.floor(s % 3600 / 60) })
}

export const connectionColumns = <T extends TunnelConnection = TunnelConnection,>(tr: Translate, locale: string): TableColumnsType<T> => [
  { title: tr("来源地址"), dataIndex: 'source', width: 210, fixed: 'left', render: (value: string) => <span className="connection-address">{value}</span> },
  { title: tr("目标地址"), dataIndex: 'target', width: 210, render: (value: string, row) => <span className="connection-address">{value || (row.ended_at ? tr("未取得目标地址") : tr("等待 SOCKS5 请求"))}</span> },
  { title: tr("状态"), dataIndex: 'state', width: 105, render: (value: TunnelConnection['state']) => <span className={`connection-state connection-state--${value}`}>{{ connected: tr("已连接"), connecting: tr("连接中"), closed: tr("已结束"), failed: tr("失败") }[value]}</span> },
  { title: tr("接入时间"), dataIndex: 'started_at', width: 180, render: (value: string) => new Date(value).toLocaleString(locale, { hour12: false }) },
  { title: <Tooltip title={tr("从接受客户端连接开始计时，包含等待目标连接的时间。")}>{tr("已持续")}</Tooltip>, dataIndex: 'age_seconds', width: 140, render: (seconds: number) => duration(seconds, tr) },
  { title: tr("累计接收"), dataIndex: 'bytes_in', width: 115, align: 'right', render: (value: number) => `↓ ${humanBytes(value)}` },
  { title: tr("累计发送"), dataIndex: 'bytes_out', width: 115, align: 'right', render: (value: number) => `↑ ${humanBytes(value)}` },
]

export const historyColumns = <T extends TunnelConnection = TunnelConnection,>(tr: Translate, locale: string): TableColumnsType<T> => [
  ...connectionColumns<T>(tr, locale).slice(0, 4),
  { title: tr("结束时间"), dataIndex: 'ended_at', width: 180, render: (value: string) => new Date(value).toLocaleString(locale, { hour12: false }) },
  ...connectionColumns<T>(tr, locale).slice(4),
  { title: tr("失败原因"), dataIndex: 'error', width: 300, render: (value: string) => value ? <span className="connection-failure">{value}</span> : '—' },
]

function ConnectionTable({ name, history, showHistory }: { name: string; history: boolean; showHistory: boolean }) {
  const { tr, locale } = useI18n()
  const [rows, setRows] = useState<TunnelConnection[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)
  const [retry, setRetry] = useState(0)
  const [limit, setLimit] = useState<number | null>(null)
  const [persisted, setPersisted] = useState(false)
  const [persistenceError, setPersistenceError] = useState('')
  const [page, setPage] = useState(1)

  useEffect(() => {
    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined
    setLoading(true)
    async function refresh() {
      try {
        const result = history
          ? await api.connectionHistory(name, controller.signal)
          : { connections: await api.connections(name, controller.signal), limit: null, persisted: false, persistence_error: '' }
        if (controller.signal.aborted) return
        setRows(result.connections)
        setLimit(result.limit)
        setPersisted(result.persisted)
        setPersistenceError(result.persistence_error || '')
        setPage((page) => Math.min(page, Math.max(1, Math.ceil(result.connections.length / 20))))
        setError('')
        setUpdatedAt(new Date())
      } catch (err) {
        if (controller.signal.aborted) return
        setError((err as Error).message)
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false)
          // Schedule after completion so slow requests never overlap.
          timer = setTimeout(refresh, 2000)
        }
      }
    }
    void refresh()
    return () => {
      controller.abort()
      clearTimeout(timer)
    }
  }, [name, history, retry])

  return (
    <>
      <div className="connection-toolbar">
        <strong>{updatedAt ? tr(error ? '{count} 条连接（上次数据）' : '{count} 条连接', { count: rows.length }) : tr('正在获取连接')}</strong>
        <span>{updatedAt ? tr('更新于 {time} · ', { time: updatedAt.toLocaleTimeString(locale, { hour12: false }) }) : ''}{tr('每 2 秒自动刷新')}</span>
      </div>
      {error && <Alert type="warning" showIcon message={tr('连接信息刷新失败：{error}', { error })} description={tr("显示的数据可能已过期，将自动重试。")}
        action={<Button size="small" onClick={() => setRetry((value) => value + 1)}>{tr("重试")}</Button>} />}
      {persistenceError && <Alert type="error" showIcon message={tr("部分连接历史未能写入日志")} description={tr('{error}。当前仍显示内存记录，请检查磁盘空间与文件权限；未写入的记录重启后会丢失。', { error: persistenceError })} />}
      <Table<TunnelConnection> size="small" columns={history ? historyColumns(tr, locale) : connectionColumns(tr, locale)} dataSource={rows} rowKey={(row) => `${row.started_at}-${row.id}`}
        loading={loading} scroll={{ x: history ? 1655 : 1175 }} pagination={{ current: page, onChange: setPage, pageSize: 20, showSizeChanger: false, hideOnSinglePage: true }}
        locale={{ emptyText: error ? tr("暂时无法获取连接信息") : loading ? tr("正在加载…") : history ? tr("暂无历史连接，SOCKS5 请求结束后会显示在这里") : tr("暂无活动连接") }} />
      <p className="connection-note">{history ? <>
        {tr('展示最近已结束的 SOCKS5 请求（最多 {limit} 条），最新记录在前。', { limit: limit ?? '—' })}
        {tr(persisted ? '连接记录追加到日志文件，停止或重启线路不会清空；重启 daemon 后从保留的日志恢复。日志路径及轮转策略可在页面配置中修改。' : '当前未启用日志持久化，记录仅在本次线路运行期间保留。')}
        {tr('失败仅代表该请求失败，不代表线路异常。')}
      </> : tr(showHistory ? '仅展示当前转发连接，断开后移入历史连接，不包含 SSH 控制连接。' : '仅展示当前转发连接，断开后自动移除，不包含 SSH 控制连接。')}
        {tr('收发以 daemon 所在机器为准，仅统计业务数据。')}</p>
    </>
  )
}

export default function ConnectionModal({ name, showHistory, onClose }: { name: string; showHistory: boolean; onClose: () => void }) {
  const { tr } = useI18n()
  const [tab, setTab] = useState('active')
  const history = showHistory && tab === 'history'
  return (
    <Modal open title={tr('{name} · 连接详情', { name })} onCancel={onClose} width={1120} className="line-modal connection-modal"
      footer={<Button onClick={onClose}>{tr("关闭")}</Button>}>
      {showHistory && <Tabs activeKey={tab} onChange={setTab} items={[
        { key: 'active', label: tr("当前连接") },
        { key: 'history', label: tr("历史连接") },
      ]} />}
      <ConnectionTable key={`${name}-${history}`} name={name} history={history} showHistory={showHistory} />
    </Modal>
  )
}
