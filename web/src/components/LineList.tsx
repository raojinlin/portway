import { Empty, Spin } from 'antd'
import type { TunnelView } from '../types'
import LineRow from './LineRow'
import { useI18n } from '../I18n'

interface Props {
  tunnels: TunnelView[]
  loaded: boolean
  onChanged: () => void
  onEdit: (tunnel: TunnelView) => void
}

export default function LineList({ tunnels, loaded, onChanged, onEdit }: Props) {
  const { tr } = useI18n()
  return (
    <section className="line-section">
      <div className="section-heading">
        <h2>{tr("转发线路")}</h2>
        <span className="section-count">{tr('{count} 条', { count: tunnels.length })}</span>
      </div>

      {!loaded ? (
        <div className="empty-state"><Spin /><span>{tr("正在同步线路状态…")}</span></div>
      ) : tunnels.length === 0 ? (
        <div className="empty-state">
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={tr("还没有线路，创建第一条转发规则。")} />
        </div>
      ) : (
        <div className="line-grid">
          {tunnels.map((t) => (
            <LineRow key={t.Name} tunnel={t} onChanged={onChanged} onEdit={onEdit} />
          ))}
        </div>
      )}
    </section>
  )
}
