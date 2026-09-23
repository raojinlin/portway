import { useEffect, useState } from 'react'
import { Alert, App, Button, Collapse, Form, Input, InputNumber, Modal, Select, Spin } from 'antd'
import { api } from '../api'
import type { DaemonConfig, DaemonConfigView } from '../types'

export default function ConfigModal({ onClose }: { onClose: () => void }) {
  const { message } = App.useApp()
  const [form] = Form.useForm<DaemonConfig>()
  const [view, setView] = useState<DaemonConfigView | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [retry, setRetry] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    api.config(controller.signal).then((result) => {
      if (controller.signal.aborted) return
      setView(result)
      form.setFieldsValue(result.config)
      setError('')
    }).catch((err: Error) => {
      if (!controller.signal.aborted) setError(err.message)
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [form, retry])

  async function save(values: DaemonConfig) {
    setSaving(true)
    try {
      const result = await api.saveConfig({ ...values, log_file: values.log_file || '' })
      setView(result)
      setError('')
      message.success(result.restart_required ? `已保存到 YAML 文件，${result.desktop ? '退出并重新打开桌面应用' : '重启 daemon'}后生效` : '配置已保存到 YAML 文件')
    } catch (err) {
      setError(`保存失败：${(err as Error).message}`)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal open title={view?.desktop ? '应用配置' : 'daemon 配置'} className="line-modal config-modal" width={720} onCancel={onClose}
      maskClosable={!saving} keyboard={!saving} closable={!saving}
      footer={<><Button onClick={onClose} disabled={saving}>关闭</Button><Button type="primary" onClick={() => form.submit()} disabled={loading || !view} loading={saving}>保存到配置文件</Button></>}>
      {error && <Alert type="error" showIcon message={error} action={!view && <Button size="small" onClick={() => setRetry((n) => n + 1)}>重试</Button>} />}
      <Spin spinning={loading}>
        {view && <>
          <p className="connection-note">配置文件：<code style={{ overflowWrap: 'anywhere' }}>{view.path}</code>{!view.exists && '（保存时创建）'}</p>
          <Alert className="config-restart-note" type={view.restart_required ? 'warning' : 'info'} showIcon
            message={view.restart_required ? '文件配置与当前运行配置不同' : `保存后${view.desktop ? '退出并重新打开应用' : '重启 daemon'}生效`}
            description={view.desktop ? '桌面应用直接托管后端，不开放 HTTP 端口。保存不会中断线路；macOS / Windows 关闭窗口仅隐藏，请从应用菜单或托盘菜单选择退出后再打开。' : '保存不会中断线路或自动重启。启动参数优先于 YAML；要使用文件中的值，请移除对应的命令行覆盖参数。手动编辑 YAML 后同样需要重启。'} />
        </>}
        <Form form={form} layout="vertical" onFinish={save} disabled={loading || saving || !view}>
          <div className="form-section">
            <h3 className="form-section-heading">服务</h3>
            <div className="form-grid">
              <Form.Item name="addr" label="监听地址" tooltip="Web 页面与 API 的监听地址，格式 host:port。没有登录认证，建议只绑定 127.0.0.1；修改后用新地址访问。" rules={[{ required: true, message: '请输入 host:port' }]}>
                <Input placeholder="127.0.0.1:7777" disabled={view?.desktop} />
              </Form.Item>
              <Form.Item name="state_file" label="线路状态文件" tooltip="线路定义仍以 JSON 保存。更换路径不会迁移旧线路；请先复制原文件。路径位于 daemon 所在机器。" rules={[{ required: true, message: '请输入状态文件路径' }]}>
                <Input />
              </Form.Item>
            </div>
          </div>
          <div className="form-section">
            <h3 className="form-section-heading">日志与连接历史</h3>
            <div className="form-grid">
              <Form.Item name="log_level" label="运行日志级别" tooltip="仅影响 daemon 运行日志；SOCKS5 连接历史始终记录，不受日志级别影响。" rules={[{ required: true }]}>
                <Select options={['debug', 'info', 'warn', 'error'].map((value) => ({ value, label: value }))} />
              </Form.Item>
              <Form.Item name="log_format" label="运行日志格式" tooltip="text 适合直接阅读；json 便于日志采集。连接历史始终采用 JSONL。" rules={[{ required: true }]}>
                <Select options={[{ value: 'text', label: 'Text' }, { value: 'json', label: 'JSON' }]} />
              </Form.Item>
              <Form.Item className="span-2" name="log_file" label="运行日志文件" tooltip="同时输出到 stderr 和此文件；留空则仅 stderr。支持 ~/；相对路径以 YAML 文件目录为基准。">
                <Input placeholder="留空仅输出到 stderr" />
              </Form.Item>
              <Form.Item className="span-2" name="connection_log" label="连接历史日志文件" tooltip="SOCKS5 请求结束时追加一行 JSON，包含来源、目标、时间、流量和结果。启动时从当前文件及轮转文件恢复每条线路最近 500 条。修改路径不会迁移旧日志。" rules={[{ required: true, message: '请输入连接日志路径' }]}>
                <Input placeholder="logs/connections.jsonl" />
              </Form.Item>
              <Form.Item name="log_max_size_mb" label="单文件大小上限（MiB）" tooltip="对运行日志和连接日志分别生效。达到上限后轮转，不拆分单条记录。" rules={[{ required: true }]}>
                <InputNumber min={1} max={1024} precision={0} style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item name="log_max_backups" label="保留轮转文件数" tooltip="不含当前文件；.1 最新。超过数量的最旧文件被覆盖。两个日志各自保留此数量。" rules={[{ required: true }]}>
                <InputNumber min={1} max={20} precision={0} style={{ width: '100%' }} />
              </Form.Item>
            </div>
            <p className="connection-note">日志保留目标地址等访问元数据，不记录业务内容或 SSH 密码。文件权限为仅当前用户可读写。异常退出时尚未结束的连接没有完整历史记录。</p>
          </div>
        </Form>
        {view && <Collapse ghost items={[{ key: 'effective', label: '查看当前运行配置', children: <pre style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{JSON.stringify(view.effective, null, 2)}</pre> }]} />}
      </Spin>
    </Modal>
  )
}
