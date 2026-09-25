# Writing a connector

A connector gives Zodim new capabilities: `tides.today`, `bank.balance`, `printer.print`. Routines and explorations can only reach the world through capabilities, so a connector decides what becomes possible. For that reason Zodim holds connectors to what they declare.

A connector is a program in any language that speaks [MCP](https://modelcontextprotocol.io) over stdio: one JSON-RPC message per line on stdin and stdout. Next to it sits a `connector.json`. The smallest complete example is [`examples/connectors/tides`](../examples/connectors/tides): a Python file using only the standard library.

## connector.json

```json
{
  "name": "tides",
  "description": "Tide tables for a port.",
  "command": "python3",
  "args": ["server.py"],
  "env": ["TIDES_KEY"],
  "capabilities": [
    {
      "name": "tides.today",
      "risk": "read",
      "signature": "tides.today({port})",
      "returns": "[{port, time, height_m, kind}]",
      "schema": {"type": "object", "required": ["port"], "properties": {"port": {"type": "string"}}}
    }
  ],
  "contract": [
    {"capability": "tides.today", "args": {"port": "Santos"}, "keys": ["port", "time", "height_m", "kind"]}
  ]
}
```

- **name**: lowercase letters and digits. Every capability is `<name>.<method>`, and its MCP tool is `<name>_<method>`.
- **risk**: this is what the rules engine uses, so declare it honestly:

  | Risk | Meaning | Default behavior |
  |---|---|---|
  | `read` | no side effects | allowed |
  | `notify` | a message to the owner only | allowed |
  | `reversible` | a change that can be undone | allowed, kept undoable |
  | `irreversible` | anything else, including messages to other people | asks the owner |

- **signature / returns / schema**: what the explorer and the compiler see. Write them for a reader who has never seen your service.
- **env**: the only environment variables your process gets, besides `PATH`, `HOME` and `LANG`. The owner fills them in Connections, and they are stored in Zodim's vault. Nothing else from Zodim's environment reaches you.
- **contract**: test cases. Every `read` capability needs at least one. A case calls the capability and checks the result has these keys, either on the object or on the first item of a list. Cases may only call `read` capabilities.

## What Zodim enforces

- The process must offer exactly the declared tools. One extra tool and the connector does not start.
- Names cannot shadow Zodim's own capabilities.
- Calls time out after 30 seconds, and a process that stops answering is restarted on the next call.
- Every call passes the rules engine and lands in Receipts with its arguments, like any built-in capability.

## Checking and installing

```bash
TIDES_KEY=... zodim connector check ./tides
```

No part of Zodim needs recompiling, and it does not need a restart either. Two ways to install:

- In **Conexões › Instalar conector (.zip)**, send a zip with `connector.json` at its root (or inside one folder). Zodim checks the manifest before anything is installed.
- Or copy the folder yourself and choose **Recarregar a pasta de conectores**:

```bash
cp -r ./tides ~/.zodim/connectors/
```

The connector shows up in Connections with its capabilities, their risk, fields for the env vars it declares, and a **Testar** button that runs the contract. Connectors travel with **Exportar tudo**, like everything else.

## Adding an existing MCP server

Any MCP server works without writing a `connector.json`:

- **Conexões › Explorar** searches the [official MCP registry](https://registry.modelcontextprotocol.io). Servers published as npm or PyPI packages run on this computer through `npx` or `uvx`, pinned to the listed version; remote servers are reached over streamable HTTP, and only over https.
- **Conexões › Adicionar manualmente** takes a command (`npx -y @company/server@1.2.3`) or an https address, with env vars or headers.

Zodim connects, lists the tools and suggests a risk for each from the server's own hints (`readOnlyHint`, `destructiveHint`); a tool without hints counts as irreversible, so Zodim asks before using it. The owner picks which tools to include and can change any risk before installing. Zodim writes the `connector.json` itself (marked `imported`), keeps env values and headers in the vault, and from then on:

- each tool is a capability named `<name>.<tool>` that goes through rules and approvals like any other;
- tools the server adds later stay hidden, and a tool that disappears stops the connector until it is added again and reviewed;
- **Desinstalar** removes the folder, the capabilities and the stored keys.

Registry servers are third-party code that Zodim has not verified. Local ones run with a clean environment that holds only the variables you filled in.

## Built-in connectors

The catalog in Connections also has native connectors, written in Go with contract tests against fake servers in `internal/connector/services`:

| Connector | Capabilities |
|---|---|
| RSS and Atom | 🟢 `rss.read` (sites declared per routine) |
| GitHub | 🟢 `github.issues` · 🔴 `github.comment` |
| Todoist | 🟢 `todoist.tasks` · 🟡 `todoist.add`, `todoist.close` |
| Notion | 🟢 `notion.search` · 🟡 `notion.append` |
| Obsidian | 🟢 `obsidian.search` · 🟡 `obsidian.append` (keeps the previous version) |
| Home Assistant | 🟢 `ha.states` · 🟡 `ha.call` · 🔴 `ha.critical` (locks, alarms, covers, valves; always asks) |
| Slack, Discord | 🔵 `slack.send`, `discord.send` (the owner's own channel, through a webhook) |

The core connectors are Google Calendar and iCal, Gmail and IMAP, SMTP, HTTP JSON with declared hosts, Telegram and WhatsApp.
