import { useCallback, useEffect, useState } from 'react'
import { Alert, Button, Input, Select, Table, Tabs, Tooltip, type TableColumnsType } from 'antd'
import { PauseOutlined, CaretRightOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import { api } from '../api'
import { useI18n } from '../I18n'
import { useLiveQuery } from '../useLiveQuery'
import type { ActivityConnection, ConnectionsView, RuntimeLogEntry, RuntimeLogs, TunnelView } from '../types'
import { connectionColumns, historyColumns } from './ConnectionModal'

type Kind = 'logs' | 'active' | 'history'
type ActivityData = { kind: 'logs'; result: RuntimeLogs } | { kind: 'active' | 'history'; result: ConnectionsView }

export default function ActivityPage({ tunnels, onConfigClick }: { tunnels: TunnelView[]; onConfigClick: () => void }) {
  const { tr, locale } = useI18n()
  const [kind, setKind] = useState<Kind>('logs')
  const [tunnel, setTunnel] = useState('')
  const [filter, setFilter] = useState('')
  const [query, setQuery] = useState('')
  const [draft, setDraft] = useState('')
  const [live, setLive] = useState(true)
  const [page, setPage] = useState(1)
  const [expanded, setExpanded] = useState<React.Key[]>([])
  const load = useCallback(async (signal: AbortSignal): Promise<ActivityData> => {
    const params = new URLSearchParams({ limit: '500', tunnel, q: query })
    if (kind === 'logs') {
      params.set('level', filter)
      return { kind, result: await api.logs(params, signal) }
    }
    params.set('kind', kind)
    params.set('state', filter)
    return { kind, result: await api.allConnections(params, signal) }
  }, [kind, tunnel, filter, query])
  const { data, loading, error, updatedAt, refresh } = useLiveQuery(load, live)
  useEffect(() => { setPage(1); setExpanded([]) }, [load])
  const logs = data?.kind === 'logs' ? data.result : null
  const connections = data && data.kind !== 'logs' ? data.result : null
  const count = logs?.entries.length ?? connections?.connections.length ?? 0
  useEffect(() => setPage(value => Math.min(value, Math.max(1, Math.ceil(count / 50)))), [count])
  const names = [...new Set([...tunnels.map(item => item.Name), ...(connections?.tunnels ?? []), ...(logs?.entries.map(entry => entry.tunnel).filter(Boolean) ?? []), ...(tunnel ? [tunnel] : [])])].sort()
  const logColumns: TableColumnsType<RuntimeLogEntry> = [
    { title: tr('时间'), dataIndex: 'time', width: 185, render: value => value && !Number.isNaN(Date.parse(value)) ? new Date(value).toLocaleString(locale, { hour12: false }) : '—' },
    { title: tr('级别'), dataIndex: 'level', width: 85, render: value => <span className={`log-level log-level--${String(value).toLowerCase()}`}>{value || '—'}</span> },
    { title: tr('线路名称'), dataIndex: 'tunnel', width: 135, ellipsis: true, render: value => value || '—' },
    { title: tr('日志消息'), dataIndex: 'message', render: value => <span className="log-message">{value}</span> },
  ]
  const columns = [...(kind === 'history' ? historyColumns<ActivityConnection>(tr, locale) : connectionColumns<ActivityConnection>(tr, locale))]
  columns.splice(1, 0, { title: tr('线路名称'), dataIndex: 'tunnel', width: 140, ellipsis: true })
  const pagination = { current: page, onChange: setPage, pageSize: 50, showSizeChanger: false, hideOnSinglePage: true }

  return <section className="activity-page">
    <div className="activity-heading">
      <div><h2>{tr('日志与连接')}</h2><p>{tr('排查连接问题，查看运行事件与访问记录。')}</p></div>
      <div className="activity-controls">
        <span className={`live-indicator${live ? ' is-live' : ''}`}><i />{tr(live ? '每 3 秒刷新' : '已暂停刷新')}</span>
        <Button icon={live ? <PauseOutlined /> : <CaretRightOutlined />} onClick={() => setLive(value => !value)}>{tr(live ? '暂停' : '继续')}</Button>
        <Tooltip title={tr('立即刷新')}><Button aria-label={tr('立即刷新')} icon={<ReloadOutlined spin={loading} />} onClick={refresh} disabled={loading} /></Tooltip>
      </div>
    </div>
    <div className="activity-panel">
      <Tabs activeKey={kind} onChange={value => { setKind(value as Kind); setFilter('') }} items={[
        { key: 'logs', label: tr('运行日志') }, { key: 'active', label: tr('当前连接') }, { key: 'history', label: tr('SOCKS5 历史') },
      ]} />
      <div className="activity-filters">
        <Select className="activity-tunnel-filter" aria-label={tr('筛选线路')} value={tunnel} onChange={setTunnel} showSearch optionFilterProp="label"
          options={[{ value: '', label: tr('全部线路') }, ...names.map(value => ({ value, label: value }))]} />
        <Select className="activity-level-filter" aria-label={tr(kind === 'logs' ? '级别' : '状态')} value={filter} onChange={setFilter}
          options={kind === 'logs' ? [{ value: '', label: tr('全部级别') }, ...['DEBUG', 'INFO', 'WARN', 'ERROR'].map(value => ({ value, label: value }))]
            : [{ value: '', label: tr('全部状态') }, ...(kind === 'active' ? [{ value: 'connecting', label: tr('连接中') }, { value: 'connected', label: tr('已连接') }]
              : [{ value: 'closed', label: tr('已结束') }, { value: 'failed', label: tr('失败') }])]} />
        <Input.Search value={draft} onChange={event => setDraft(event.target.value)} onSearch={setQuery} allowClear
          prefix={<SearchOutlined />} placeholder={tr('搜索消息、地址或错误，按回车查询')} aria-label={tr('搜索日志与连接')} />
      </div>
      <div className="activity-meta">
        <span>{tr('显示 {count} 条记录', { count })}{connections && connections.total > count ? tr('（匹配 {total} 条，仅显示最新 500 条）', { total: connections.total }) : ''}</span>
        <span>{updatedAt ? tr('更新于 {time} · ', { time: updatedAt.toLocaleTimeString(locale, { hour12: false }) }) : ''}{tr('最新记录在前')}</span>
      </div>
      {error && <Alert type="warning" showIcon message={tr('刷新失败：{error}', { error })} description={tr('当前数据可能已过期，可手动刷新重试。')} />}
      {logs && !logs.enabled && <Alert type="info" showIcon message={tr('当前仅输出到 stderr，未启用日志文件')} action={<Button size="small" onClick={onConfigClick}>{tr('打开配置')}</Button>} />}
      {logs?.truncated && <Alert type="info" showIcon message={tr('已达到读取上限：最多扫描最近 2 MiB 日志，展示最新 500 条匹配记录。更早记录请查看日志文件。')} />}
      {connections?.persistence_error && <Alert type="error" showIcon message={tr('部分连接历史未能写入日志')} description={connections.persistence_error} />}
      {kind === 'logs' ? <Table<RuntimeLogEntry> size="small" className="log-table" dataSource={logs?.entries ?? []} columns={logColumns} rowKey="id"
        loading={loading && !data} pagination={pagination} scroll={{ x: 800 }} locale={{ emptyText: tr(error ? '读取失败，请重试' : '没有匹配的日志记录') }}
        expandable={{ expandRowByClick: true, expandedRowKeys: expanded, onExpandedRowsChange: keys => setExpanded([...keys]),
          expandedRowRender: row => <pre className="activity-raw">{row.raw}</pre> }} />
        : <Table<ActivityConnection> size="small" dataSource={connections?.connections ?? []} columns={columns} rowKey={row => JSON.stringify([row.tunnel, row.started_at, row.id])}
          loading={loading && !data} pagination={pagination} scroll={{ x: kind === 'history' ? 1795 : 1315 }} locale={{ emptyText: tr(error ? '读取失败，请重试' : '没有匹配的连接记录') }} />}
      <div className="activity-footnote">
        {kind === 'logs' ? <><span>{tr('点击日志行展开原始内容。仅查看日志，不会修改文件。')}</span>{logs?.path && <code>{logs.path}</code>}</>
          : <span>{tr(kind === 'history' ? '历史仅包含 SOCKS5 请求，每条线路保留最近 500 条缓存；单次请求失败不代表线路异常。' : '当前连接包含所有转发类型，不包含 SSH 控制连接；来源地址固定在左侧。')}
            {kind === 'history' && connections && !connections.persisted && tr('当前未启用日志持久化，记录仅在本次线路运行期间保留。')}</span>}
      </div>
    </div>
  </section>
}
