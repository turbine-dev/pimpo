# Writing a connector

A connector gives Pimpo new capabilities: `tides.today`, `bank.balance`, `printer.print`. Routines and explorations can only reach the world through capabilities, so a connector decides what becomes possible. For that reason Pimpo holds connectors to what they declare.

There are two kinds:

- **A JSON connector**: just a `connector.json` that describes each capability as one HTTP request to a service's API. There is no program; Pimpo makes the requests itself. This covers most REST APIs. See [JSON connectors](#json-connectors) and [`examples/connectors/hnsearch`](../examples/connectors/hnsearch).
- **A program** in any language that speaks [MCP](https://modelcontextprotocol.io) over stdio: one JSON-RPC message per line on stdin and stdout. Next to it sits a `connector.json`. The smallest complete example is [`examples/connectors/tides`](../examples/connectors/tides): a Python file using only the standard library. Use this kind when a capability needs more than one request, local files, or logic.

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
- **env**: the only environment variables your process gets, besides `PATH`, `HOME` and `LANG`. The owner fills them in Connections, and they are stored in Pimpo's vault. Nothing else from Pimpo's environment reaches you.
- **contract**: test cases. Every `read` capability needs at least one. A case calls the capability and checks the result has these keys, either on the object or on the first item of a list. Cases may only call `read` capabilities.

## JSON connectors

Replace `command` with `http`, and give each capability a `request` (and optionally a `result`):

```json
{
  "name": "hnsearch",
  "description": "Hacker News search.",
  "env": ["EXAMPLE_KEY"],
  "http": {
    "base": "https://hn.algolia.com/api/v1",
    "headers": {"Authorization": "Bearer {{env.EXAMPLE_KEY}}"},
    "query": {}
  },
  "capabilities": [
    {
      "name": "hnsearch.stories",
      "risk": "read",
      "signature": "hnsearch.stories({query, min_points?, max?})",
      "returns": "[{title, url, points}]",
      "request": {"method": "GET", "path": "/search", "query": {"query": "{{query}}", "tags": "story", "numericFilters": "points>={{min_points}}", "hitsPerPage": "{{max}}"}},
      "result": {"path": "hits", "max": 50, "fields": {"title": "title", "url": "url", "points": "points"}}
    }
  ],
  "contract": [{"capability": "hnsearch.stories", "args": {"query": "rust"}, "keys": ["title", "points"]}]
}
```

- **http.base**: the service's address; `https` only (plain `http` only for this computer). `headers` and `query` go on every request.
- **request.method**: `GET`, `POST`, `PUT`, `PATCH` or `DELETE`. Anything but `GET` changes something, so its risk cannot be `read`.
- **request.path**: starts with `/` and is added to the base. `query` and `headers` add to the ones in `http`.
- **Placeholders**: `{{name}}` is an argument of the call; `{{env.NAME}}` is a key from `env`, filled in Connections and kept in the vault. Arguments in the path are URL-escaped, so a value cannot reach another endpoint. Keys may go in headers, the query or the body, never in the path.
- **Optional arguments**: a query parameter whose argument was not given is left out; so is a header whose key is empty. A path argument that is missing is an error.
- **request.body**: JSON sent as written. A string that is only `"{{text}}"` becomes the argument with its type (a number, a list); `{{…}}` inside a longer string is filled in as text; fields whose argument was not given are dropped. `"form": true` sends the body as form fields instead.
- **result**: `path` picks part of the answer (`data.items`); `fields` keeps only these, renamed as the keys and taken by dot path (`user.login`, and `labels.*.name` for a field of every item of a list); `max` caps a list. Without `result`, the whole JSON answer is returned. An answer that is not JSON comes back as `{text}`.

Pimpo also enforces, for JSON connectors:

- requests stay on the base's host, redirects too, and never reach private addresses (unless the base is this computer);
- 30 seconds per request and at most 5 MB of answer;
- errors say what happened: a refused key (401/403), too many requests (429), or the service's own message.

A `POST` that only reads (many APIs search that way) may be declared `read` with `"safe": true` in its request; it says so openly in the manifest.

### From an OpenAPI description

Most REST APIs publish an OpenAPI (Swagger) description, and Pimpo can write the `connector.json` from it:

- **Connections › From OpenAPI**: give the address of the description (`.json` or `.yaml`) or paste it. Pimpo lists the operations; choose the ones to include (at most 100) and each one's risk, fill in the keys, and install. Reads (`GET`) start as `read`; everything else starts as `irreversible`, so it asks first until you decide otherwise.
- Or from the terminal, which writes a folder you can review and edit before installing:

```bash
pimpo connector openapi https://petstore3.swagger.io/api/v3/openapi.json
pimpo connector openapi --only get https://petstore3.swagger.io/api/v3/openapi.json petstore ./petstore
pimpo connector check ./petstore
```

What the import does:

- OpenAPI 3.x and Swagger 2.0, JSON or YAML; local `$ref`s are followed. The service's address comes from `servers` (or `host` and `basePath`), resolved against the description's address when relative.
- Keys come from the security schemes: an API key in a header or the query, a bearer token (also for OAuth, where you paste a token; Pimpo does not run the sign-in) or Basic (`user:password` in base64). When the description declares none but the API needs one, **This API needs a key the description does not declare** (or `--header Authorization`) adds a header whose whole value is a key.
- Path, query and header parameters and the JSON or form body become the capability's arguments, with their schema and descriptions; `readOnly` fields are left out of the body.
- Operations that upload files or need cookies are listed as unsupported.
- The operation's summary and the shape of its answer (`[{id, name, status}]`) become what the model reads; a list answer is capped at 50 items.
- Contract cases are written only for reads whose required arguments have a default or a fixed set of values; Pimpo never invents an id.

## What Pimpo enforces

- The process must offer exactly the declared tools. One extra tool and the connector does not start.
- Names cannot shadow Pimpo's own capabilities.
- Calls time out after 30 seconds, and a process that stops answering is restarted on the next call.
- Every call passes the rules engine and lands in Receipts with its arguments, like any built-in capability.

## Checking and installing

```bash
TIDES_KEY=... pimpo connector check ./tides
```

No part of Pimpo needs recompiling, and it does not need a restart either. Two ways to install:

- In **Connections › Install connector (.json · .zip)**, send the `connector.json` itself (JSON connectors) or a zip with `connector.json` at its root (or inside one folder). Pimpo checks the manifest before anything is installed.
- Or copy the folder yourself and choose **Reload the connectors folder**:

```bash
cp -r ./tides ~/.pimpo/connectors/
```

The connector shows up in Connections with its capabilities, their risk, fields for the env vars it declares, and a **Test** button that runs the contract. Connectors travel with **Export and import everything**, like everything else.

## Adding an existing MCP server

Any MCP server works without writing a `connector.json`:

- **Connections › Explore** searches the [official MCP registry](https://registry.modelcontextprotocol.io). Servers published as npm or PyPI packages run on this computer through `npx` or `uvx`, pinned to the listed version; remote servers are reached over streamable HTTP, and only over https.
- **Connections › Add by hand** takes a command (`npx -y @company/server@1.2.3`) or an https address, with env vars or headers.

Pimpo connects, lists the tools and suggests a risk for each from the server's own hints (`readOnlyHint`, `destructiveHint`); a tool without hints counts as irreversible, so Pimpo asks before using it. The owner picks which tools to include and can change any risk before installing. Pimpo writes the `connector.json` itself (marked `imported`), keeps env values and headers in the vault, and from then on:

- each tool is a capability named `<name>.<tool>` that goes through rules and approvals like any other;
- tools the server adds later stay hidden, and a tool that disappears stops the connector until it is added again and reviewed;
- **Uninstall** removes the folder, the capabilities and the stored keys.

Registry servers are third-party code that Pimpo has not verified. Local ones run with a clean environment that holds only the variables you filled in.

## Built-in connectors

The catalog in Connections also has native connectors, written in Go with contract tests against fake servers in `internal/connector/services`:

| Connector | Capabilities |
|---|---|
| RSS and Atom | 🟢 `rss.read` (sites declared per routine) |
| Web search | 🟢 `web.search` (Brave Search API or a SearXNG instance) |
| GitHub | 🟢 `github.repo`, `github.issues`, `github.issue`, `github.pulls`, `github.pr`, `github.checks`, `github.sponsors` · 🟡 `github.issue_create`, `github.issue_edit`, `github.pr_create`, `github.pr_review` · 🔴 `github.comment`, `github.merge`, `github.release` |
| YouTube | 🟢 `youtube.stats` · 🔴 `youtube.publish` (uploads, always private, are Pimpo's own `youtube.upload`) |
| LinkedIn | posts through Pimpo's own `linkedin.post` 🔴, which adds the AI disclosure for company members |
| Stripe | 🟢 `stripe.balance`, `stripe.charges` (a restricted, read-only key) |
| Todoist | 🟢 `todoist.tasks` · 🟡 `todoist.add`, `todoist.close` |
| Notion | 🟢 `notion.search` · 🟡 `notion.append` |
| Obsidian | 🟢 `obsidian.search` · 🟡 `obsidian.append` (keeps the previous version) |
| Home Assistant | 🟢 `ha.states` · 🟡 `ha.call` · 🔴 `ha.critical` (locks, alarms, covers, valves; always asks) |
| Slack, Discord | 🔵 `slack.send`, `discord.send` (the owner's own channel, through a webhook) |

The core connectors are Google Calendar and iCal, Gmail and IMAP, SMTP, HTTP JSON with declared hosts, Telegram and WhatsApp.
