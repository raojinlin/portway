import { CopyOutlined } from '@ant-design/icons'
import { Button, Tabs } from 'antd'
import { useI18n } from '../I18n'

interface Props {
  auth: string
  token: string
  url: string
  onCopy: (value: string) => void
}

interface Example {
  title: string
  code: string
  displayCode?: string
  note: string
  requiresToken?: boolean
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, "'\\''")}'`
}

export default function MCPConnectionExamples({ auth, token, url, onCopy }: Props) {
  const { tr } = useI18n()
  const tokenEnabled = auth === 'token' || auth === 'both'
  const oauthEnabled = auth === 'oauth' || auth === 'both'
  const placeholder = '<PORTWAY_TOKEN>'
  const credential = token || placeholder
  const genericConfig = (oauth: boolean, value = credential) => JSON.stringify({
    mcpServers: {
      portway: oauth
        ? { type: 'http', url }
        : { type: 'http', url, headers: { Authorization: `Bearer ${value}` } },
    },
  }, null, 2)

  const codex: Example[] = []
  if (oauthEnabled) codex.push({
    title: 'OAuth 2.1',
    code: `codex mcp add portway --url ${url} --oauth-client-registration dcr`,
    note: tr('添加时会打开 Portway 授权页；服务器已存在时运行 codex mcp login portway --oauth-client-registration dcr。'),
  })
  if (tokenEnabled) codex.push({
    title: 'Token',
    code: `export PORTWAY_MCP_TOKEN=${shellQuote(credential)}\ncodex mcp add portway --url ${url} --bearer-token-env-var PORTWAY_MCP_TOKEN`,
    displayCode: `export PORTWAY_MCP_TOKEN=${shellQuote(placeholder)}\ncodex mcp add portway --url ${url} --bearer-token-env-var PORTWAY_MCP_TOKEN`,
    note: tr('环境变量需在启动 Codex 时仍然可用；不要把 Token 提交到 shell 配置或代码仓库。'),
    requiresToken: true,
  })

  const claude: Example[] = []
  if (oauthEnabled) claude.push({
    title: 'OAuth 2.1',
    code: `claude mcp add --transport http --scope user portway ${shellQuote(url)}`,
    note: tr('添加后在 Claude Code 中运行 /mcp，并按提示完成 OAuth 授权。'),
  })
  if (tokenEnabled) claude.push({
    title: 'Token',
    code: `claude mcp add --transport http --scope user --header ${shellQuote(`Authorization: Bearer ${credential}`)} portway ${shellQuote(url)}`,
    displayCode: `claude mcp add --transport http --scope user --header ${shellQuote(`Authorization: Bearer ${placeholder}`)} portway ${shellQuote(url)}`,
    note: tr('此命令会把认证 Header 写入 Claude Code 的 MCP 配置，请保护该配置文件。'),
    requiresToken: true,
  })

  const generic: Example[] = []
  if (oauthEnabled) generic.push({
    title: tr('通用 JSON · OAuth'),
    code: genericConfig(true),
    note: tr('适用于支持 Streamable HTTP、OAuth Discovery、DCR 和 PKCE 的 MCP 客户端。'),
  })
  if (tokenEnabled) generic.push({
    title: tr('通用 JSON · Token'),
    code: genericConfig(false),
    displayCode: genericConfig(false, placeholder),
    note: tr('不同客户端可能使用 type: "streamable-http"；请按其配置格式调整字段名。'),
    requiresToken: true,
  })

  function examples(items: Example[]) {
    return <div className="mcp-example-list">
      {items.map((item) => <article className="mcp-example" key={item.title}>
        <div className="mcp-example__heading">
          <strong>{item.title}</strong>
          <Button size="small" icon={<CopyOutlined />} disabled={item.requiresToken && !token}
            onClick={() => onCopy(item.code)}>{tr(item.requiresToken ? '复制（含 Token）' : '复制')}</Button>
        </div>
        <pre><code>{item.displayCode ?? item.code}</code></pre>
        <p>{item.note}</p>
      </article>)}
    </div>
  }

  return <section className="mcp-client-examples">
    <div className="mcp-client-examples__heading">
      <strong>{tr('连接示例')}</strong>
      <span>{tr('示例会跟随当前地址和授权方式更新；修改后请先保存 MCP 配置。')}</span>
    </div>
    <Tabs size="small" items={[
      { key: 'codex', label: 'Codex', forceRender: true, children: examples(codex) },
      { key: 'claude', label: 'Claude Code', forceRender: true, children: examples(claude) },
      { key: 'generic', label: tr('其他客户端'), forceRender: true, children: examples(generic) },
    ]} />
  </section>
}
