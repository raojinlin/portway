---
name: portway
description: Manage Portway SSH tunnels through the Portway MCP server. Use when the user asks to inspect connections or create, update, start, stop, or remove Portway tunnels.
---

# Portway

Use the Portway MCP tools to manage the user's SSH forwarding lines.

## Workflow

- Inspect existing tunnels with `list_tunnels` or `get_tunnel` before changing them. Reuse the user's names, SSH config aliases, and address conventions.
- Choose `local` for a local listener forwarding to a fixed remote target, `remote` for an SSH-server listener forwarding back to a locally reachable target, and `dynamic` for a local SOCKS5 proxy.
- Creating a tunnel starts it and persists it as enabled. Stopping a tunnel preserves its definition; deleting removes it permanently.
- `update_tunnel` replaces the non-secret configuration. Start from `get_tunnel` and preserve every field the user did not ask to change.
- Treat missing tools as a server permission boundary. Do not work around a read-only or operate-only MCP configuration.
- Report the resulting tunnel state and any connection error after a mutation.

## Credentials And Safety

- Never request, submit, expose, or store an SSH password through MCP. Prefer an SSH config alias, a key path, or the user's running ssh-agent.
- Keep listeners on `127.0.0.1` unless the user explicitly asks to expose them more broadly and understands the network impact.
- Explain and obtain confirmation immediately before deleting a tunnel or disabling host-key verification.
- Do not retry a failed mutation more than once unless the user asks. Surface the concrete error and a recovery step.
