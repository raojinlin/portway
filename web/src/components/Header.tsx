import { DesktopOutlined, GlobalOutlined, MoonOutlined, PlusOutlined, SettingOutlined, SunOutlined } from '@ant-design/icons'
import { Button, Dropdown, Tooltip } from 'antd'
import type { TunnelView } from '../types'
import { useThemeMode } from '../ThemeMode'
import { useI18n } from '../I18n'

interface Props {
  tunnels: TunnelView[]
  onAddClick: () => void
  onConfigClick: () => void
}

export default function Header({ tunnels, onAddClick, onConfigClick }: Props) {
  const { tr, preference: languagePreference, setPreference: setLanguagePreference } = useI18n()
  const { mode, preference, setPreference } = useThemeMode()
  const themeLabel = preference === 'system' ? tr("跟随系统") : preference === 'dark' ? tr("深色") : tr("浅色")
  const running = tunnels.filter((t) => t.State === 'running').length
  const errors = tunnels.filter((t) => t.State === 'error').length
  const connections = tunnels.reduce((sum, t) => sum + (t.ActiveConns || 0), 0)

  return (
    <header className="page-header">
      <div className="page-toolbar">
        <h1>Portway</h1>
        <div className="header-actions">
          <Dropdown trigger={['click']} menu={{
            selectable: true, selectedKeys: [languagePreference],
            items: [{ key: 'system', label: tr('自动检测') }, { key: 'zh', label: tr("简体中文") }, { key: 'en', label: 'English' }],
            onClick: ({ key }) => { if (key === 'system' || key === 'zh' || key === 'en') setLanguagePreference(key) },
          }}>
            <Button type="text" icon={<GlobalOutlined />} title={tr('切换语言')} aria-label={tr('切换语言')} />
          </Dropdown>
          <Dropdown trigger={['click']} menu={{
            selectable: true,
            selectedKeys: [preference],
            items: [
              { key: 'system', label: tr("跟随系统"), icon: <DesktopOutlined /> },
              { key: 'light', label: tr("浅色"), icon: <SunOutlined /> },
              { key: 'dark', label: tr("深色"), icon: <MoonOutlined /> },
            ],
            onClick: ({ key }) => {
              if (key === 'system' || key === 'light' || key === 'dark') setPreference(key)
            },
          }}>
            <Button
              className="theme-button"
              type="text"
              icon={preference === 'system' ? <DesktopOutlined /> : mode === 'dark' ? <MoonOutlined /> : <SunOutlined />}
              title={tr('主题：{theme}', { theme: themeLabel })}
              aria-label={tr('选择主题，当前{theme}', { theme: themeLabel })}
            />
          </Dropdown>
          <Tooltip title={tr("daemon 配置与日志设置，保存到 YAML 文件")}>
            <Button icon={<SettingOutlined />} onClick={onConfigClick} aria-label={tr("打开配置")}>{tr("配置")}</Button>
          </Tooltip>
          <Button type="primary" icon={<PlusOutlined />} onClick={onAddClick}>{tr("新建线路")}</Button>
        </div>
      </div>

      <div className="status-summary" aria-label={tr("线路概览")}>
        <span><strong>{running}</strong> {' '}{tr("运行中")}</span>
        <span className={errors ? 'text-error' : undefined}><strong>{errors}</strong> {' '}{tr("异常")}</span>
        <span><strong>{connections}</strong> {' '}{tr("个活动连接")}</span>
      </div>
    </header>
  )
}
