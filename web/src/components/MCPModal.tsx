import { useEffect, useState } from 'react'
import { ApiOutlined, CopyOutlined, DownloadOutlined } from '@ant-design/icons'
import { Alert, App, Button, Form, Input, Modal, Select, Space, Spin, Switch } from 'antd'
import { api } from '../api'
import type { DaemonConfigView, MCPConfig } from '../types'
import { useI18n } from '../I18n'
import { copyText as writeClipboard } from '../proxyCommand'
import MCPConnectionExamples from './MCPConnectionExamples'

export default function MCPModal({ onClose }: { onClose: () => void }) {
  const { tr } = useI18n()
  const { message } = App.useApp()
  const [form] = Form.useForm<MCPConfig>()
  const [view, setView] = useState<DaemonConfigView | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [retry, setRetry] = useState(0)
  const enabled = Form.useWatch('enabled', form) ?? false
  const addr = Form.useWatch('addr', form) ?? '127.0.0.1:7778'
  const auth = Form.useWatch('auth', form) ?? 'token'
  const token = Form.useWatch('token', form) ?? ''

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    api.config(controller.signal).then((result) => {
      if (controller.signal.aborted) return
      setView(result)
      form.setFieldsValue(result.config.mcp)
      setError('')
    }).catch((err: Error) => {
      if (!controller.signal.aborted) setError(err.message)
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [form, retry])

  async function save(mcp: MCPConfig) {
    if (!view) return
    setSaving(true)
    try {
      const result = await api.saveConfig({ ...view.config, mcp })
      setView(result)
      form.setFieldsValue(result.config.mcp)
      setError('')
      message.success(tr('MCP 配置已保存并立即生效'))
    } catch (err) {
      setError(tr('保存失败：{error}', { error: (err as Error).message }))
    } finally {
      setSaving(false)
    }
  }

  async function copyText(value: string, success: string) {
    try {
      await writeClipboard(value)
      message.success(success)
    } catch (err) {
      message.error(tr('复制失败：{error}', { error: (err as Error).message }))
    }
  }

  async function downloadSkill() {
    try {
      if (view?.desktop) {
        const saved = await api.saveSkillDesktop()
        if (saved) message.success(tr('Portway Skill 已保存到 {path}', { path: saved.path }))
        return
      }
      const blob = await api.downloadSkill()
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = 'portway-skill.zip'
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 0)
      message.success(tr('Portway Skill 已下载'))
    } catch (err) {
      message.error(tr('下载失败：{error}', { error: (err as Error).message }))
    }
  }

  const mcpURL = `http://${addr}/mcp`
  const skillURL = `http://${addr}/skills/portway.zip`
  const skillAvailable = Boolean(view?.mcp_status.running && view.mcp_status.addr === addr)
  const installPrompt = tr('请从 {url} 下载并安装 Portway Skill 到你的技能目录；安装后检查 SKILL.md，并告诉我安装位置。', { url: skillURL })
  const statusAuth = view?.mcp_status.auth === 'both' ? tr('访问令牌 + OAuth 2.1') : view?.mcp_status.auth === 'oauth' ? 'OAuth 2.1' : tr('访问令牌')
  const statusAccess = view?.mcp_status.access === 'manage' ? tr('完整管理') : view?.mcp_status.access === 'read' ? tr('只读') : tr('查看与启停')

  return (
    <Modal open title={<span className="modal-title-with-icon"><ApiOutlined />{tr('MCP 服务器')}</span>}
      className="line-modal mcp-modal" width={720} onCancel={onClose}
      maskClosable={!saving} keyboard={!saving} closable={!saving}
      footer={<><Button onClick={onClose} disabled={saving}>{tr('关闭')}</Button><Button type="primary" onClick={() => form.submit()} disabled={loading || !view} loading={saving}>{tr('保存 MCP 配置')}</Button></>}>
      {error && <Alert type="error" showIcon message={error} action={!view && <Button size="small" onClick={() => setRetry((n) => n + 1)}>{tr('重试')}</Button>} />}
      <Spin spinning={loading}>
        {view && <section className="mcp-runtime-card" aria-live="polite">
          <div>
            <span className={`mcp-runtime-dot${view.mcp_status.running ? ' is-running' : view.mcp_status.error ? ' is-error' : ''}`} aria-hidden="true" />
            <strong>{view.mcp_status.running ? tr('运行中') : view.mcp_status.error ? tr('启动失败') : tr('未启用')}</strong>
          </div>
          <span>{view.mcp_status.addr} · {statusAuth} · {statusAccess}</span>
          {view.mcp_status.error && <p className="text-error">{view.mcp_status.error}</p>}
        </section>}

        <Form form={form} layout="vertical" onFinish={save} disabled={loading || saving || !view}>
          <div className="mcp-settings__heading">
            <div>
              <h3 className="form-section-heading">{tr('本机 MCP 服务')}</h3>
              <p className="connection-note">{tr('允许本机 Agent 通过 MCP 查看和管理 Portway 线路。')}</p>
            </div>
            <Form.Item name="enabled" valuePropName="checked" noStyle>
              <Switch checkedChildren={tr('开启')} unCheckedChildren={tr('停用')} aria-label={tr('开启 MCP 服务器')} />
            </Form.Item>
          </div>

          {enabled && <>
            <Alert type="info" showIcon message={tr(auth === 'both'
              ? '静态 Token 和 OAuth 同时可用；OAuth 授权码只输入 Portway 页面，不提供给 Agent。'
              : auth === 'oauth'
                ? 'MCP 仅监听本机地址；OAuth 使用浏览器确认和 PKCE，授权码只输入 Portway 页面，不提供给 Agent。'
                : 'MCP 仅监听本机地址并要求访问令牌；保存后立即生效，不会中断现有线路。')} />
            <div className="form-grid mcp-fields">
              <Form.Item name="addr" label={tr('MCP 监听地址')} tooltip={tr('仅支持数字形式的回环地址，例如 127.0.0.1:7778。')} rules={[{ required: true, message: tr('请输入 MCP 监听地址') }]}>
                <Input placeholder="127.0.0.1:7778" />
              </Form.Item>
              <Form.Item name="auth" label={tr('授权方式')} rules={[{ required: true }]}>
                <Select options={[
                  { value: 'token', label: tr('访问令牌') },
                  { value: 'oauth', label: 'OAuth 2.1' },
                  { value: 'both', label: tr('访问令牌 + OAuth 2.1') },
                ]} />
              </Form.Item>
              <Form.Item name="access" label={tr('Agent 权限')} tooltip={tr('未授权的工具不会出现在 Agent 的工具列表中。')} rules={[{ required: true }]}>
                <Select options={[
                  { value: 'read', label: tr('只读') },
                  { value: 'operate', label: tr('查看与启停') },
                  { value: 'manage', label: tr('完整管理') },
                ]} />
              </Form.Item>
              <Form.Item className="span-2" label={tr(auth === 'both' ? '访问令牌 / OAuth 授权码' : auth === 'oauth' ? 'OAuth 授权码' : '访问令牌')}
                tooltip={tr(auth === 'both'
                  ? '此值既可作为静态 Bearer Token，也用于 OAuth 授权页确认；点击眼睛可查看。'
                  : auth === 'oauth'
                    ? 'Codex 登录时在 Portway 授权页输入此码。它不会发送给 Agent；点击眼睛可查看。'
                    : '已保存的令牌会回填到输入框并默认隐藏；点击眼睛可查看。留空保存会生成新的高强度令牌。')}>
                <Space.Compact block>
                  <Form.Item name="token" noStyle rules={[{ validator: (_, value) => !value || value.length >= 32 ? Promise.resolve() : Promise.reject(new Error(tr(auth === 'oauth' ? 'OAuth 授权码至少需要 32 个字符' : '访问令牌至少需要 32 个字符'))) }]}>
                    <Input.Password placeholder={tr('留空自动生成')} autoComplete="new-password" />
                  </Form.Item>
                  <Button icon={<CopyOutlined />} disabled={!token} onClick={() => copyText(token, tr(auth === 'both' ? 'MCP 凭据已复制' : auth === 'oauth' ? 'OAuth 授权码已复制' : '访问令牌已复制'))}>{tr('复制')}</Button>
                </Space.Compact>
              </Form.Item>
            </div>

            <div className="mcp-endpoint">
              <span>{tr('服务端点')}</span><code>{mcpURL}</code>
              <Button size="small" icon={<CopyOutlined />} onClick={() => copyText(mcpURL, tr('MCP 地址已复制'))}>{tr('复制地址')}</Button>
            </div>
            <MCPConnectionExamples auth={auth} token={token} url={mcpURL}
              onCopy={(value) => copyText(value, tr('连接示例已复制'))} />
          </>}

          <section className="mcp-skill-panel">
            <div className="mcp-skill-download">
              <div><strong>{tr('Portway Skill')}</strong><span>{tr('用户可以下载 ZIP，也可以把安装指令交给 Agent 自行下载安装。')}</span></div>
              <Button icon={<DownloadOutlined />} onClick={downloadSkill}>{tr('下载 Skill')}</Button>
            </div>
            <div className="mcp-install-command">
              <code>{installPrompt}</code>
              <Button icon={<CopyOutlined />} disabled={!skillAvailable}
                onClick={() => copyText(installPrompt, tr('Agent 安装指令已复制'))}>{tr('复制 Agent 安装指令')}</Button>
            </div>
            {!skillAvailable && <p className="connection-note">{tr('启用并保存 MCP 服务器后，Agent 才能访问这个本机下载地址。')}</p>}
          </section>
        </Form>
      </Spin>
    </Modal>
  )
}
