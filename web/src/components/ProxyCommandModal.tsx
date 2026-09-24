import { useState } from 'react'
import { Alert, App, Button, Input, Modal, Segmented } from 'antd'
import { CopyOutlined } from '@ant-design/icons'
import { useI18n } from '../I18n'
import { copyText, proxyCommand, proxyURL, type ProxyShell } from '../proxyCommand'
import type { TunnelView } from '../types'

export default function ProxyCommandModal({ tunnel, onClose }: { tunnel: TunnelView; onClose: () => void }) {
  const { tr } = useI18n()
  const { message } = App.useApp()
  const [shell, setShell] = useState<ProxyShell>(() => /Win/i.test(navigator.platform) ? 'powershell' : 'posix')
  let url = ''
  try { url = proxyURL(tunnel.local_listen) } catch { /* Show a localized, actionable error. */ }
  const command = url ? proxyCommand(url, shell) : ''
  async function copy() {
    try {
      await copyText(command)
      message.success(tr('代理命令已复制'))
    } catch {
      message.error(tr('无法访问剪贴板，请手动复制下方命令'))
    }
  }
  return <Modal open title={tr('{name} · 复制代理命令', { name: tunnel.Name })} onCancel={onClose}
    footer={<><Button onClick={onClose}>{tr('关闭')}</Button><Button type="primary" icon={<CopyOutlined />} disabled={!command} onClick={copy}>{tr('复制')}</Button></>}>
    <Segmented value={shell} onChange={(value) => setShell(value as ProxyShell)} options={[
      { value: 'posix', label: 'Bash / Zsh' }, { value: 'powershell', label: 'PowerShell' }, { value: 'url', label: tr('代理 URL') },
    ]} />
    {!url && <Alert type="error" message={tr('监听地址无效，请先编辑线路的 SOCKS5 监听地址')} />}
    {tunnel.State !== 'running' && <Alert type="warning" message={tr('线路尚未运行，请启动后再使用代理')} />}
    <Input.TextArea className="proxy-command" value={command} readOnly autoSize={{ minRows: 3 }} aria-label={tr('代理命令')} onFocus={(event) => event.target.select()} />
    <p className="connection-note">{tr('设置当前终端的 ALL_PROXY、all_proxy、http_proxy 和 https_proxy，不修改系统代理。程序需支持 socks5h；域名通过代理解析，程序自身配置及 NO_PROXY 可能优先。')}</p>
    <p className="connection-note">{tr('请在 daemon 所在机器运行。若从其他机器使用，请替换为可访问的监听地址；不要将无认证的 SOCKS5 端口暴露到公网。')}</p>
  </Modal>
}
