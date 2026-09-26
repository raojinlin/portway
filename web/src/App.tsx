import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { App as AntApp, ConfigProvider } from 'antd'
import { api } from './api'
import type { TunnelView } from './types'
import { buildAntdTheme } from './theme'
import { ThemeModeProvider, useThemeMode } from './ThemeMode'
import Header from './components/Header'
import LineList from './components/LineList'
import AddLineModal from './components/AddLineModal'
import ConfigModal from './components/ConfigModal'
import MCPModal from './components/MCPModal'
import { I18nProvider, useI18n } from './I18n'
import zhCN from 'antd/locale/zh_CN'
import enUS from 'antd/locale/en_US'
import ActivityPage from './components/ActivityPage'
import { FileTextOutlined, SwapOutlined } from '@ant-design/icons'
import { listenForDesktopNavigation, type ConnectionNavigation } from './desktopNavigation'
import ConnectionModal from './components/ConnectionModal'

const POLL_INTERVAL_MS = 2000

function Board() {
  const { tr } = useI18n()
  const { message } = AntApp.useApp()
  const [tunnels, setTunnels] = useState<TunnelView[]>([])
  const [loaded, setLoaded] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [configOpen, setConfigOpen] = useState(false)
  const [mcpOpen, setMCPOpen] = useState(false)
  const [page, setPage] = useState<'tunnels' | 'activity'>('tunnels')
  const [activitySession, setActivitySession] = useState(0)
  const [lineSession, setLineSession] = useState(0)
  const [connectionTarget, setConnectionTarget] = useState<ConnectionNavigation | null>(null)
  const [editingTunnel, setEditingTunnel] = useState<TunnelView | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => listenForDesktopNavigation(() => {
    setConfigOpen(false)
    setMCPOpen(false)
    setModalOpen(false)
    setEditingTunnel(null)
    setConnectionTarget(null)
    setPage('activity')
    setActivitySession(value => value + 1)
  }, target => {
    setConfigOpen(false)
    setMCPOpen(false)
    setModalOpen(false)
    setEditingTunnel(null)
    setPage('tunnels')
    setLineSession(value => value + 1)
    setConnectionTarget(target)
  }, () => {
    setConfigOpen(false)
    setModalOpen(false)
    setEditingTunnel(null)
    setConnectionTarget(null)
    setMCPOpen(true)
  }), [])

  const refresh = useCallback(async () => {
    try {
      const list = await api.list()
      setTunnels(list)
      setLoaded(true)
    } catch (err) {
      message.error(tr('刷新失败：{error}', { error: (err as Error).message }))
    }
  }, [message, tr])

  useEffect(() => {
    refresh()
    pollRef.current = setInterval(refresh, POLL_INTERVAL_MS)
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [refresh])

  return (
    <main className="app-shell">
      <div className="app-frame">
        <Header tunnels={tunnels} onAddClick={() => { setEditingTunnel(null); setModalOpen(true) }}
          onMCPClick={() => setMCPOpen(true)} onConfigClick={() => setConfigOpen(true)} />
        <nav className="page-navigation" aria-label={tr('页面导航')}>
          <button type="button" aria-current={page === 'tunnels' ? 'page' : undefined} onClick={() => setPage('tunnels')}><SwapOutlined />{tr('转发线路')}</button>
          <button type="button" aria-current={page === 'activity' ? 'page' : undefined} onClick={() => setPage('activity')}><FileTextOutlined />{tr('日志与连接')}</button>
        </nav>
        {page === 'activity' ? <ActivityPage key={activitySession} tunnels={tunnels} onConfigClick={() => setConfigOpen(true)} /> : <LineList
          key={lineSession}
          tunnels={tunnels}
          loaded={loaded}
          onChanged={refresh}
          onEdit={(tunnel) => { setEditingTunnel(tunnel); setModalOpen(true) }}
        />}
      </div>

      {configOpen && <ConfigModal onClose={() => setConfigOpen(false)} />}
      {mcpOpen && <MCPModal onClose={() => setMCPOpen(false)} />}
      {connectionTarget && <ConnectionModal key={`${connectionTarget.name}:${lineSession}`} name={connectionTarget.name} showHistory={connectionTarget.showHistory} onClose={() => setConnectionTarget(null)} />}
      <AddLineModal
        open={modalOpen}
        tunnel={editingTunnel}
        onClose={() => { setModalOpen(false); setEditingTunnel(null) }}
        onSaved={refresh}
      />
    </main>
  )
}

function ThemedApp() {
  const { language } = useI18n()
  const { mode } = useThemeMode()
  const antdTheme = useMemo(() => buildAntdTheme(mode), [mode])

  return (
    <ConfigProvider theme={antdTheme} locale={language === 'zh' ? zhCN : enUS}>
      <AntApp>
        <Board />
      </AntApp>
    </ConfigProvider>
  )
}

export default function App() {
  return (
    <I18nProvider>
      <ThemeModeProvider>
        <ThemedApp />
      </ThemeModeProvider>
    </I18nProvider>
  )
}
