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

const POLL_INTERVAL_MS = 2000

function Board() {
  const { message } = AntApp.useApp()
  const [tunnels, setTunnels] = useState<TunnelView[]>([])
  const [loaded, setLoaded] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [configOpen, setConfigOpen] = useState(false)
  const [editingTunnel, setEditingTunnel] = useState<TunnelView | null>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const refresh = useCallback(async () => {
    try {
      const list = await api.list()
      setTunnels(list)
      setLoaded(true)
    } catch (err) {
      message.error(`刷新失败：${(err as Error).message}`)
    }
  }, [message])

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
        <Header tunnels={tunnels} onAddClick={() => { setEditingTunnel(null); setModalOpen(true) }} onConfigClick={() => setConfigOpen(true)} />
        <LineList
          tunnels={tunnels}
          loaded={loaded}
          onChanged={refresh}
          onEdit={(tunnel) => { setEditingTunnel(tunnel); setModalOpen(true) }}
        />
      </div>

      {configOpen && <ConfigModal onClose={() => setConfigOpen(false)} />}
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
  const { mode } = useThemeMode()
  const antdTheme = useMemo(() => buildAntdTheme(mode), [mode])

  return (
    <ConfigProvider theme={antdTheme}>
      <AntApp>
        <Board />
      </AntApp>
    </ConfigProvider>
  )
}

export default function App() {
  return (
    <ThemeModeProvider>
      <ThemedApp />
    </ThemeModeProvider>
  )
}
