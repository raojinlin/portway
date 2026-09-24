import { useEffect, type ReactNode } from 'react'
import { QuestionCircleOutlined, SafetyCertificateOutlined, SettingOutlined } from '@ant-design/icons'
import { Alert, App as AntApp, Checkbox, Collapse, Form, Input, Modal, Segmented, Tooltip } from 'antd'
import { api } from '../api'
import type { Direction, TunnelRequest, TunnelView } from '../types'
import { useI18n } from '../I18n'
import type { Translate } from '../locale'

interface Props {
  open: boolean
  onClose: () => void
  onSaved: () => void
  tunnel?: TunnelView | null
}

interface FormValues {
  name: string
  direction: Direction
  local_listen?: string
  ssh_address: string
  remote_listen?: string
  forward_address?: string
  ssh_user?: string
  ssh_key_path?: string
  ssh_password?: string
  ssh_config_path?: string
  known_hosts_path?: string
  keep_alive?: string
  reconnect_delay?: string
  trust_new_host_key?: boolean
  insecure_skip_host_key_check?: boolean
}

function FieldLabel({ children, tip }: { children: ReactNode; tip: ReactNode }) {
  const { tr } = useI18n()
  return (
    <span className="field-label">
      {children}
      <Tooltip title={tip} placement="top" trigger={['hover', 'focus', 'click']}><QuestionCircleOutlined tabIndex={0} aria-label={tr('参数说明')} /></Tooltip>
    </span>
  )
}

const directionOptions = (tr: Translate) => [
  {
    value: 'local' as Direction,
    label: <Tooltip title={tr("在本机开放端口，经 SSH 服务器访问远端服务。等价于 ssh -L。")}>{tr("本地转发")}</Tooltip>,
  },
  {
    value: 'remote' as Direction,
    label: <Tooltip title={tr("在 SSH 服务器开放端口，把流量转回本机可访问的服务。等价于 ssh -R。")}>{tr("远程转发")}</Tooltip>,
  },
  {
    value: 'dynamic' as Direction,
    label: <Tooltip title={tr("在本机创建 SOCKS5 代理，由客户端为每个连接指定目标。等价于 ssh -D。")}>{tr("动态代理")}</Tooltip>,
  },
]

export default function AddLineModal({ open, onClose, onSaved, tunnel }: Props) {
  const { tr } = useI18n()
  const { message } = AntApp.useApp()
  const [form] = Form.useForm<FormValues>()
  const direction = Form.useWatch('direction', form) ?? 'local'
  const skipHostKey = Form.useWatch('insecure_skip_host_key_check', form) ?? false

  useEffect(() => {
    if (!open) return
    form.resetFields()
    if (tunnel) {
      form.setFieldsValue({
        name: tunnel.Name,
        direction: tunnel.direction,
        local_listen: tunnel.local_listen,
        ssh_address: tunnel.ssh_address,
        remote_listen: tunnel.remote_listen,
        forward_address: tunnel.forward_address,
        ssh_user: tunnel.ssh_user,
        ssh_key_path: tunnel.ssh_key_path,
        ssh_config_path: tunnel.ssh_config_path,
        known_hosts_path: tunnel.known_hosts_path,
        keep_alive: tunnel.keep_alive,
        reconnect_delay: tunnel.reconnect_delay,
        trust_new_host_key: tunnel.trust_new_host_key,
        insecure_skip_host_key_check: tunnel.insecure_skip_host_key_check,
      })
    }
  }, [open, form, tunnel])

  const showLocal = direction !== 'remote'
  const showRemote = direction === 'remote'
  const showForward = direction !== 'dynamic'

  async function handleFinish(values: FormValues) {
    const payload: TunnelRequest = {
      name: values.name,
      direction: values.direction,
      local_listen: showLocal ? values.local_listen : undefined,
      ssh_address: values.ssh_address,
      remote_listen: showRemote ? values.remote_listen : undefined,
      forward_address: showForward ? values.forward_address : undefined,
      ssh_user: values.ssh_user,
      ssh_key_path: values.ssh_key_path,
      ssh_password: values.ssh_password,
      ssh_config_path: values.ssh_config_path,
      known_hosts_path: values.known_hosts_path,
      keep_alive: values.keep_alive,
      reconnect_delay: values.reconnect_delay,
      trust_new_host_key: values.trust_new_host_key,
      insecure_skip_host_key_check: values.insecure_skip_host_key_check,
    }
    try {
      if (tunnel) await api.update(tunnel.Name, payload)
      else await api.create(payload)
      message.success(tr(tunnel ? '已更新线路 “{name}”' : '已添加线路 “{name}”', { name: values.name }))
      onClose()
      onSaved()
    } catch (err) {
      message.error(tr(tunnel ? '更新失败：{error}' : '添加失败：{error}', { error: (err as Error).message }))
    }
  }

  return (
    <Modal
      className="line-modal"
      title={tunnel ? tr('编辑 {name}', { name: tunnel.Name }) : tr("新建转发线路")}
      open={open}
      onCancel={onClose}
      onOk={() => form.submit()}
      okText={tunnel ? tr("保存并应用") : tr("创建并启动")}
      cancelText={tr("取消")}
      destroyOnClose
      centered
      width={720}
    >
      <Form<FormValues>
        form={form}
        layout="vertical"
        onFinish={handleFinish}
        initialValues={{ direction: 'local', trust_new_host_key: false, insecure_skip_host_key_check: false }}
      >
        <div className="form-section">
          <Form.Item
            name="direction"
            label={<FieldLabel tip={tr("本地转发适合访问内网服务；远程转发适合对外暴露本机服务；动态代理提供通用 SOCKS5 出口。")}>{tr("转发类型")}</FieldLabel>}
          >
            <Segmented className="direction-selector" options={directionOptions(tr)} block />
          </Form.Item>
        </div>

        <div className="form-section">
          <h3 className="form-section-heading">{tr("线路与端点")}</h3>
          <div className="form-grid">
            <Form.Item
              name="name"
              label={<FieldLabel tip={tr("线路的唯一标识，用于列表展示和 CLI 操作；创建后不可修改。")}>{tr("线路名称")}</FieldLabel>}
              rules={[{ required: true, message: tr("请输入线路名称") }]}
            >
              <Input placeholder={tr("例如：drop-web")} disabled={Boolean(tunnel)} />
            </Form.Item>
            <Form.Item
              name="ssh_address"
              label={<FieldLabel tip={tr("SSH 服务器的 host:port，或 ~/.ssh/config 中的 Host 别名，例如 drop。")}>{tr("SSH 目标")}</FieldLabel>}
              rules={[{ required: true, message: tr("请输入 SSH 目标") }]}
            >
              <Input placeholder={tr("drop 或 10.0.1.6:22")} />
            </Form.Item>

            {showLocal && (
              <Form.Item
                name="local_listen"
                label={<FieldLabel tip={direction === 'dynamic' ? tr("SOCKS5 客户端连接的本机地址。建议绑定 127.0.0.1，避免代理暴露给局域网。") : tr("本机程序连接的监听地址。使用 127.0.0.1 仅允许本机访问，0.0.0.0 会对局域网开放。")}>{direction === 'dynamic' ? tr("SOCKS5 本地监听") : tr("本地监听")}</FieldLabel>}
                rules={[{ required: true, message: tr("请输入本地监听地址") }]}
              >
                <Input placeholder={direction === 'dynamic' ? '127.0.0.1:1080' : '127.0.0.1:8080'} />
              </Form.Item>
            )}
            {showRemote && (
              <Form.Item
                name="remote_listen"
                label={<FieldLabel tip={tr("由 SSH 服务器监听的地址。127.0.0.1 仅服务器本机可访问；0.0.0.0 还需要服务器允许 GatewayPorts。")}>{tr("远程监听")}</FieldLabel>}
                rules={[{ required: true, message: tr("请输入远程监听地址") }]}
              >
                <Input placeholder="0.0.0.0:9000" />
              </Form.Item>
            )}
            {showForward && (
              <Form.Item
                name="forward_address"
                label={<FieldLabel tip={direction === 'remote' ? tr("SSH 流量回到本机后连接的目标地址，例如本机 Web 服务 127.0.0.1:3000。") : tr("SSH 服务器一侧最终访问的目标服务地址，例如数据库 10.0.0.20:5432。")}>{tr("转发目标")}</FieldLabel>}
                rules={[{ required: true, message: tr("请输入转发目标地址") }]}
              >
                <Input placeholder={direction === 'remote' ? '127.0.0.1:3000' : '10.0.0.20:80'} />
              </Form.Item>
            )}
          </div>
        </div>

        <Collapse
          className="advanced-collapse"
          ghost
          items={[
            {
              key: 'auth',
              label: <span className="collapse-title"><SafetyCertificateOutlined /><span><strong>{tr("认证与主机校验")}</strong><small>{tr("用户、密钥、known_hosts")}</small></span></span>,
              children: (
                <div className="form-grid compact-grid">
                  <Form.Item name="ssh_user" label={<FieldLabel tip={tr("登录目标 SSH 服务器的用户名；留空时读取 ssh config 的 User，再回退到 daemon 的本机用户名。跳板使用自己的配置。")}>{tr("SSH 用户")}</FieldLabel>}>
                    <Input placeholder={tr("留空读取 ssh config")} />
                  </Form.Item>
                  <Form.Item name="ssh_key_path" label={<FieldLabel tip={tr("daemon 所在机器上的私钥路径，支持 ~/。留空时读取 IdentityFile，未配置时尝试默认 id_ed25519、id_ecdsa、id_rsa，并可使用 ssh-agent。加密密钥需预先在 agent 中解锁，不调用系统 ssh。")}>{tr("私钥路径")}</FieldLabel>}>
                    <Input placeholder="~/.ssh/id_ed25519" />
                  </Form.Item>
                  <Form.Item name="ssh_password" label={<FieldLabel tip={tr("SSH 登录密码。建议优先使用密钥；编辑时留空会保留已保存的密码。")}>{tr("SSH 密码")}</FieldLabel>}>
                    <Input.Password placeholder={tunnel ? tr("留空保留原密码") : tr("不推荐使用密码")} />
                  </Form.Item>
                  <Form.Item name="ssh_config_path" label={<FieldLabel tip={tr("留空使用 ~/.ssh/config。支持 Host、User、Port、IdentityFile、IdentityAgent 和原生 ProxyJump（含多跳）；常见 ProxyCommand ssh -W %h:%p drop 也原生处理。自定义代理命令仍需对应外部程序。")}>SSH config</FieldLabel>}>
                    <Input placeholder="~/.ssh/config" />
                  </Form.Item>
                  <Form.Item className="span-2" name="known_hosts_path" label={<FieldLabel tip={tr("保存可信服务器公钥的文件；留空时依次读取 ssh config 的 UserKnownHostsFile 和 ~/.ssh/known_hosts。")}>{tr("known_hosts 路径")}</FieldLabel>}>
                    <Input placeholder={tr("留空使用默认路径")} />
                  </Form.Item>
                  <Form.Item className="span-2 checkbox-item" name="trust_new_host_key" valuePropName="checked">
                    <Checkbox disabled={skipHostKey}><FieldLabel tip={tr("仅在主机从未被记录时接受并写入密钥；已有记录发生变化时仍会拒绝，防止中间人攻击。")}>{tr("首次连接自动信任新主机（TOFU）")}</FieldLabel></Checkbox>
                  </Form.Item>
                  <Form.Item className="span-2 checkbox-item danger-option" name="insecure_skip_host_key_check" valuePropName="checked">
                    <Checkbox><FieldLabel tip={tr("不验证目标服务器身份，也会接受变化的密钥，仅限临时诊断。此选项不跳过原生跳板的校验；跳板仍按自己的 known_hosts 验证。")}>{tr("跳过 Host Key 校验")}</FieldLabel></Checkbox>
                  </Form.Item>
                  {skipHostKey && <Alert className="span-2 host-key-warning" type="warning" showIcon message={tr("服务器身份校验已关闭")} description={tr("连接将接受任何主机密钥，包括已发生变化的密钥。请仅在风险可控时使用。")} />}
                </div>
              ),
            },
            {
              key: 'connection',
              label: <span className="collapse-title"><SettingOutlined /><span><strong>{tr("连接策略")}</strong><small>{tr("保活与断线重试")}</small></span></span>,
              children: (
                <div className="form-grid compact-grid">
                  <Form.Item name="keep_alive" label={<FieldLabel tip={tr("向 SSH 服务器发送保活请求的间隔，Go duration 格式，例如 30s、1m；设为 0s 可关闭。")}>{tr("Keepalive 间隔")}</FieldLabel>}>
                    <Input placeholder="30s" />
                  </Form.Item>
                  <Form.Item name="reconnect_delay" label={<FieldLabel tip={tr("连接失败或断开后再次尝试的等待时间，Go duration 格式，例如 3s、500ms。")}>{tr("重连间隔")}</FieldLabel>}>
                    <Input placeholder="3s" />
                  </Form.Item>
                </div>
              ),
            },
          ]}
        />
      </Form>
    </Modal>
  )
}
