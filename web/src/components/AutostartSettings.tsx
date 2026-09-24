import { useEffect, useRef, useState } from 'react'
import { Alert, Button, Switch } from 'antd'
import { api } from '../api'
import { useI18n } from '../I18n'
import type { AutostartStatus } from '../types'

export default function AutostartSettings() {
  const { tr } = useI18n()
  const [status, setStatus] = useState<AutostartStatus | null>(null)
  const [busy, setBusy] = useState(true)
  const [error, setError] = useState('')
  const [retry, setRetry] = useState(0)
  const lifetime = useRef<AbortController | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    lifetime.current = controller
    setBusy(true)
    api.autostart(controller.signal).then((value) => {
      if (!controller.signal.aborted) { setStatus(value); setError('') }
    }).catch((err: Error) => {
      if (!controller.signal.aborted) setError(err.message)
    }).finally(() => {
      if (!controller.signal.aborted) setBusy(false)
    })
    return () => controller.abort()
  }, [retry])

  async function save(enabled: boolean) {
    const signal = lifetime.current?.signal
    if (busy || !signal || signal.aborted) return
    setBusy(true)
    try {
      const value = await api.setAutostart(enabled, signal)
      if (!signal.aborted) { setStatus(value); setError('') }
    } catch (err) {
      if (!signal.aborted) setError((err as Error).message)
    } finally {
      if (!signal.aborted) setBusy(false)
    }
  }

  return <section className="form-section autostart-settings">
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16 }}>
      <h3 className="form-section-heading" style={{ marginBottom: 0 }}>{tr('开机自启')}</h3>
      <Switch aria-label={tr('开机自启')} checked={status?.enabled ?? false} loading={busy}
        disabled={!status || busy || (!status.supported && !status.enabled)} onChange={save} />
    </div>
    <p className="connection-note">{tr('登录当前系统账户后自动启动，开关立即保存，无需重启。macOS / Windows 后台常驻，Linux 显示主窗口。')}</p>
    <p className="connection-note">{tr('此开关管理系统启动项，不写入 YAML。若被系统设置禁用，也需在系统中重新允许。')}</p>
    {status && !status.supported && <Alert type="info" showIcon message={status.reason === 'app_bundle_required'
      ? tr('请先将打包的 Portway.app 放到固定位置（建议应用程序目录），并从该位置打开。')
      : tr('当前平台或应用路径不支持开机自启。')} />}
    {status?.supported && status.needs_update && <Alert type="warning" showIcon message={tr('应用路径或启动项已变化，请更新启动项。')}
      action={<Button size="small" disabled={busy} onClick={() => save(true)}>{tr('更新启动项')}</Button>} />}
    {error && <Alert type="error" showIcon message={error}
      action={<Button size="small" disabled={busy} onClick={() => setRetry((n) => n + 1)}>{tr('重试')}</Button>} />}
  </section>
}
