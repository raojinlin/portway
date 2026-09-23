import { DesktopOutlined, MoonOutlined, PlusOutlined, SettingOutlined, SunOutlined } from '@ant-design/icons'
import { Button, Dropdown, Tooltip } from 'antd'
import type { TunnelView } from '../types'
import { useThemeMode } from '../ThemeMode'

interface Props {
  tunnels: TunnelView[]
  onAddClick: () => void
  onConfigClick: () => void
}

export default function Header({ tunnels, onAddClick, onConfigClick }: Props) {
  const { mode, preference, setPreference } = useThemeMode()
  const themeLabel = preference === 'system' ? '跟随系统' : preference === 'dark' ? '深色' : '浅色'
  const running = tunnels.filter((t) => t.State === 'running').length
  const errors = tunnels.filter((t) => t.State === 'error').length
  const connections = tunnels.reduce((sum, t) => sum + (t.ActiveConns || 0), 0)

  return (
    <header className="page-header">
      <div className="page-toolbar">
        <h1>Portway</h1>
        <div className="header-actions">
          <Tooltip title="daemon 配置与日志设置，保存到 YAML 文件">
            <Button icon={<SettingOutlined />} onClick={onConfigClick} aria-label="打开配置">配置</Button>
          </Tooltip>
          <Dropdown trigger={['click']} menu={{
            selectable: true,
            selectedKeys: [preference],
            items: [
              { key: 'system', label: '跟随系统', icon: <DesktopOutlined /> },
              { key: 'light', label: '浅色', icon: <SunOutlined /> },
              { key: 'dark', label: '深色', icon: <MoonOutlined /> },
            ],
            onClick: ({ key }) => {
              if (key === 'system' || key === 'light' || key === 'dark') setPreference(key)
            },
          }}>
            <Button
              className="theme-button"
              type="text"
              icon={preference === 'system' ? <DesktopOutlined /> : mode === 'dark' ? <MoonOutlined /> : <SunOutlined />}
              title={`主题：${themeLabel}`}
              aria-label={`选择主题，当前${themeLabel}`}
            />
          </Dropdown>
          <Button type="primary" icon={<PlusOutlined />} onClick={onAddClick}>新建线路</Button>
        </div>
      </div>

      <div className="status-summary" aria-label="线路概览">
        <span><strong>{running}</strong> 运行中</span>
        <span className={errors ? 'text-error' : undefined}><strong>{errors}</strong> 异常</span>
        <span><strong>{connections}</strong> 个活动连接</span>
      </div>
    </header>
  )
}
