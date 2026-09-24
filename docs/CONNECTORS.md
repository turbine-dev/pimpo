# Writing a connector

A connector gives Vigia new capabilities: `tides.today`, `bank.balance`, `printer.print`. Routines and explorations can only reach the world through capabilities, so a connector decides what becomes possible. For that reason Vigia holds connectors to what they declare.

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
- **env**: the only environment variables your process gets, besides `PATH`, `HOME` and `LANG`. The owner fills them in Connections, and they are stored in Vigia's vault. Nothing else from Vigia's environment reaches you.
- **contract**: test cases. Every `read` capability needs at least one. A case calls the capability and checks the result has these keys, either on the object or on the first item of a list. Cases may only call `read` capabilities.

## What Vigia enforces

- The process must offer exactly the declared tools. One extra tool and the connector does not start.
- Names cannot shadow Vigia's own capabilities.
- Calls time out after 30 seconds, and a process that stops answering is restarted on the next call.
- Every call passes the rules engine and lands in Receipts with its arguments, like any built-in capability.

## Checking and installing

```bash
TIDES_KEY=... vigia connector check ./tides
```

```bash
cp -r ./tides ~/.vigia/connectors/
```

Restart Vigia and the connector shows up in Connections, with its capabilities, their risk and a **Testar** button that runs the contract.

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
