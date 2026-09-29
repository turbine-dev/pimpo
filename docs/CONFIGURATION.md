# Configuration

This page is the reference for configuring Pimpo, for people who run it themselves or want to know where each setting lives. Most people never need it: the desktop app and the web app cover everything a day-to-day user changes, and the [user guide](USER_GUIDE.md) explains those screens. Here you find the command line, the environment variables, the data folder, every stored setting, the model backends, the channels, network access, local models, backups and the settings that matter for safety. The source is at [github.com/turbine-dev/pimpo](https://github.com/turbine-dev/pimpo).

## Contents

- [Running Pimpo](#running-pimpo)
- [Environment variables](#environment-variables)
- [The data folder](#the-data-folder)
- [Settings](#settings)
- [Models](#models)
- [Channels](#channels)
- [Network and access](#network-and-access)
- [Local models](#local-models)
- [Backups and moving](#backups-and-moving)
- [Safety settings](#safety-settings)

## Running Pimpo

`pimpo` with no command runs `serve`. Flags come after the command and before any other argument (`pimpo routines import --active FOLDER`, not `pimpo routines import FOLDER --active`).

| Command | Flags | What it does |
|---|---|---|
| `pimpo serve` | `--addr` (default `127.0.0.1:7788`), `--data DIR`, `--demo` | Runs the server, the web app and the channels. Prints the login link. |
| `pimpo update` | `--check`, `--beta`, `--version vX.Y.Z`, `--rollback` | Replaces this binary with the latest release after checking it against the release's checksums, keeping the old one as `pimpo.previous`; `--rollback` puts it back. Restart Pimpo afterwards: its first start keeps a snapshot of the data (`before-VERSION`). The desktop app updates itself instead. |
| `pimpo version` | | Prints the version. |
| `pimpo export FILE.pimpo` | `--data DIR` | Writes everything to one file, secrets sealed with a passphrase. Refuses to overwrite an existing file. |
| `pimpo import FILE.pimpo` | `--data DIR` | Replaces the data with a backup. Pimpo must be stopped. What was there is kept aside. |
| `pimpo snapshots` | `--data DIR` | Lists the local snapshots, with date and size. |
| `pimpo snapshot [LABEL…]` | `--data DIR` | Takes a snapshot now. |
| `pimpo restore NAME` | `--data DIR` | Goes back to a snapshot. Pimpo must be stopped. The current state is snapshotted first. |
| `pimpo routines import FOLDER` | `--data DIR`, `--active` | Installs routines from a folder in the repository layout (`routines/<id>/routine.json`) after checking their manifest, tests and audit. They arrive paused unless `--active`. |
| `pimpo report` | `--data DIR`, `--days N` (default 21), `--json` | How the routines did in real use: runs, failures, silent failures (scheduled runs that never happened while Pimpo ran), late runs, times Pimpo was off, repairs, approvals and cost. Also `GET /api/report?days=N`. See [VALIDATION.md](VALIDATION.md). |
| `pimpo local list` | `--data DIR` | Lists the local model catalog; `✓` marks what is installed. |
| `pimpo local install ID…` | `--data DIR` | Downloads catalog items, with the same checks as the app. |
| `pimpo local remove ID…` | `--data DIR` | Removes installed items. |
| `pimpo connector check DIR` | | Loads a connector, lists its capabilities and runs its contract. Keys come from environment variables named as the connector's `env`. |
| `pimpo connector openapi SPEC [NAME DIR]` | `--only op1,op2` or `--only get`, `--header NAME` | Reads an OpenAPI description (file or URL). Without `--only` and `NAME DIR`, lists the operations; with them, writes `DIR/connector.json`. |
| `pimpo migrate openclaw\|hermes` | `--home DIR`, `--data DIR`, `--apply`, `--secrets`, `--trust` | Shows what would come over from OpenClaw or Hermes; `--apply` imports it (Pimpo must be stopped), `--secrets` also brings the Telegram token and mail password, `--trust` treats imported memories and rules as your own words. Takes a snapshot first. |
| `pimpo gallery keygen` | `--out KEYFILE` | Creates a signing key for publishing routines. |
| `pimpo gallery build DIR` | `--key KEYFILE`, `--author ID`, `--name NAME` | Signs `DIR/routines/*.json` into `DIR/index.json`, then verifies it. |
| `pimpo gallery verify INDEX` | | Verifies a gallery index (file or URL). Fails on any problem. |
| `pimpo protect suggest` | `--domain D` or `--pattern REGEX`, `--capability NAME`, `--reason TEXT` | Prints a protection-list entry to propose by pull request. |
| `pimpo protect sign DIR` | `--key KEYFILE` | Maintainers: signs `DIR/entries.json` into `DIR/list.json`. |
| `pimpo protect verify FILE` | | Checks a signed protection list. |

`serve` does a few things before it listens: it finishes an import started in the web app, applies a snapshot restore chosen in the web app, and takes a snapshot when the version changed since the last start. While it runs it takes a snapshot every 24 hours.

```bash
# Start with the defaults: ~/.pimpo, loopback only, port 7788
pimpo serve

# Try it with a sample mailbox and calendar, no accounts, no model costs
pimpo serve --demo

# A second, separate install
pimpo serve --data ~/pimpo-test --addr 127.0.0.1:7799
```

```bash
# Move to another computer
pimpo export ~/Desktop/pimpo.pimpo
pimpo import ~/Desktop/pimpo.pimpo

# Go back to an automatic snapshot
pimpo snapshots
pimpo restore 20260928-101500-daily
```

```bash
# Local models from the terminal
pimpo local list
pimpo local install kokoro-multi

# Routines from a folder, paused until you review them
pimpo routines import ~/src/my-routines
```

```bash
# Check a connector before copying it into ~/.pimpo/connectors/
EXAMPLE_KEY=... pimpo connector check examples/connectors/hnsearch
pimpo connector openapi --only get https://api.example.com/openapi.json example ./example
```

`--demo` without `--data` uses a separate folder (`~/.pimpo-demo`), so a demo never touches your real data. The demo does not start Tailscale or the home-network address.

## Environment variables

Pimpo reads only these. None of them is needed for normal use.

| Variable | Read by | What it does |
|---|---|---|
| `PIMPO_HOME` | every command | The data folder, when `--data` is not given. |
| `ZODIM_HOME` | every command | Same as `PIMPO_HOME`, from when Pimpo was called Zodim. `PIMPO_HOME` wins. |
| `PIMPO_BACKUP_PASSPHRASE` | `export`, `import` | The backup passphrase, instead of asking on the terminal. At least 8 characters. |
| `PIMPO_TOKEN` | `serve` | Fixes the login token instead of the stored one, and stores it. The desktop app and browser tests use it. |
| `PIMPO_TELEGRAM_API` | `serve` | The address of a self-hosted Telegram Bot API server. Empty uses Telegram's. |
| `PIMPO_DESKTOP_NOTIFY` | `serve` | Any value: show notices as system notifications, and complete `PATH` from the usual install folders and the login shell (so the `claude`, `codex` and `opencode` CLIs are found when started from the Finder). Set by the desktop app. |
| `PIMPO_EXIT_WITH_PARENT` | `serve` | Any value: stop when the process that started Pimpo exits. Set by the desktop app. |
| `OPENCLAW_STATE_DIR`, `OPENCLAW_CONFIG_PATH`, `OPENCLAW_WORKSPACE_DIR` | `migrate openclaw` | Where OpenClaw keeps its data, config and workspace, when not the defaults (`~/.openclaw`). |
| `HERMES_HOME` | `migrate hermes` | Where Hermes keeps its data, when not `~/.hermes`. |
| Any name a connector declares | `connector check` | The connector's keys for the contract run. |

`PATH`, `HOME` and `SHELL` are used as usual to find programs. An external connector process gets only `PATH`, `HOME`, `LANG` and the keys it declares; nothing else from Pimpo's environment reaches it (see [CONNECTORS.md](CONNECTORS.md)).

The desktop app starts `pimpo serve --addr 127.0.0.1:<a free port>` itself, with a fresh `PIMPO_TOKEN`, `PIMPO_EXIT_WITH_PARENT=1` and `PIMPO_DESKTOP_NOTIFY=1`.

## The data folder

Everything Pimpo keeps is in one folder: `~/.pimpo` by default. Change it with `--data DIR` on any command, or with `PIMPO_HOME`. The folder is created with owner-only permissions (`0700`).

Pimpo was called Zodim, and Vigia before that. When `~/.pimpo` does not exist yet and neither `--data` nor `PIMPO_HOME` is set, Pimpo moves `~/.zodim` (or else `~/.vigia`) to `~/.pimpo` and renames its database files, snapshots included. It refuses while the old app is still running, and asks you to quit it first.

| Path | What it holds |
|---|---|
| `pimpo.db` (with `-wal`, `-shm`) | The SQLite database: the event log, settings, rules, routines and their runs, people, paired devices, receipts, costs, and the vault's encrypted secrets. |
| `vault.key` | The vault key, only when the system keychain is not available. See [Safety settings](#safety-settings). |
| `memory/` | The memory: Markdown files, one per topic, versioned with git. |
| `connectors/<name>/` | Installed external connectors, each with its `connector.json`. Connectors added from **Conexões** (JSON, OpenAPI, MCP) are written here too. |
| `local/` | Downloaded voices, the voice engine and speech-to-text models. A `catalog.json` here replaces the built-in catalog (see [Local models](#local-models)). |
| `models/ggml-base.bin` | The whisper.cpp model used for voice notes when no local speech-to-text model is installed. You put it there yourself. |
| `media/` | Recent audio Pimpo made for you (the last 60). |
| `cache/speech/` | Cached audio for **Ouvir** in the chat (the last 400). |
| `skills/` | Installed skills, one folder each (`skills/<id>/SKILL.md` and its files); `.staging/` holds skills being reviewed. |
| `tailscale/` | The state of the built-in Tailscale node, when **De qualquer lugar** is on. |
| `snapshots/` | Local snapshots: a copy of `pimpo.db` and `memory/` each. The last 10 are kept. |
| `restore-pending` | A snapshot chosen in the web app, applied at the next start. |
| `import-pending/` | A backup imported from the web app, applied at the next start. Its secrets wait there only until then. |
| `before-import-<date>/` | What was here before an import. Delete it when you no longer need it. |
| `pimpo.pid` | The running server's process id. `import`, `restore` and `migrate --apply` use it to refuse while Pimpo runs. |

Routines are not files in the data folder: they live in the database. To keep them as files, use **Rotinas › Repositório** (a folder you choose) or see [ROUTINES.md](ROUTINES.md).

Outside the data folder, the desktop app caches the shell's `PATH` in the user cache folder (`pimpo/shell-path`), and the vault key is in the system keychain under the service `pimpo`.

## Settings

Settings are stored in the database and changed in **Ajustes** in the app. Almost every change takes effect at once, without a restart. The web app reads and writes them with `GET /api/settings` and `PUT /api/settings`; a `PUT` changes only the fields it carries, so a client can send just what changed. Both need a login (see [Network and access](#network-and-access)).

**Ajustes** has these sections: **Geral**, **Celular**, **Modelos**, **Notificações**, **Backup**, **Privacidade**, **Laboratório** and **Trazer de outro agente**. Preferences wait in a bar at the bottom until you press save; cards with their own button (models, backups, pairing) save at once.

### Language and time

| Setting | Default | Where | What it does |
|---|---|---|---|
| `locale` | `pt-BR` | **Ajustes › Geral › Idioma da tela e das mensagens** | The language of the app, the channels, approvals and what routines write. Also the language for voice-note transcription. |
| `zone` | the computer's time zone | **Ajustes › Geral › Fuso horário** | An IANA time zone (`America/Sao_Paulo`). Schedules, "today" in the daily limit and dates use it. An unknown zone is refused. |

### Daily spending limit

| Setting | Default | Where | What it does |
|---|---|---|---|
| Daily limit, in US dollars | `1` | **Ajustes › Geral › Limite de gasto por dia**, and the welcome screen | Checked before every paid model call and every paid voice. A call that could pass it is refused. `0` means no limit. |

The limit is stored apart from the other settings and changed with `PUT /api/budget {"daily_usd": 5}`. The day follows `zone`. Calls paid by a subscription (below) do not count. After 80% of the limit, the automatic choice stops using the strong model and the high thinking level.

### Models per job

The welcome screen sets all three at once (**Qual modelo pensa pelo Pimpo**): it offers what this computer already has (Claude Code, Codex, opencode, Ollama or LM Studio models) or one API key. With a key, Pimpo lists the provider's models, picks its current main model for exploring and compiling and a light one for judgments, requires a known price for both, tests the main one with a single word, and only then saves. The same happens with `POST /api/setup/model` and `{"kind": "provider", "provider": "openai", "key": "…"}` (or `kind` `claude_code`, `codex`, `opencode`, `ollama`, `lmstudio` with a `model`). A job left on Claude Code on a computer without it says so and points to **Ajustes › Modelos**.

Pimpo has three jobs, each with its own model. A model is named in one of these ways: `sonnet`, `opus`, `haiku` or `claude-…` (Claude Code); `codex` or `codex:<model>` (Codex); `opencode:<provider>/<model>` (opencode); or `<provider>:<model>` for a provider's API (`anthropic:…`, `openrouter:…`, `ollama:…`). See [Models](#models).

| Setting | Default | Label in **Ajustes › Modelos › Modelos em uso** | Used for |
|---|---|---|---|
| `explore_model` | `sonnet`, or what the welcome screen chose | **Fazer tarefas e conversar** | Explorations (doing a task the first time) and chats. |
| `compile_model` | `sonnet` | **Escrever rotinas** | Turning a task into a routine with tests. |
| `judge_model` | `haiku` | **Julgamentos e textos curtos** | The routines' yes-or-no questions and short texts, when the judgment backend is **Seu modelo** or as the last step of the others. |
| `models` | none | **Provedores** and **Neste computador** | Your API models, each with its price in USD per million tokens in and out (0 to 1000). A provider model must be in this list, with its price, before any job may use it. opencode models carry no price: opencode reports each call's cost. |
| `fallbacks` | none | **Se falhar:** next to each job | Up to four models per job (`explore`, `compile`, `judge`), tried in order when the model fails for a provider reason: a refused key, no credits, a rate or usage limit, an outage, a missing model or CLI. Your own daily limit and a cancelled task are never retried. You are told once when a job falls back and once when its model answers again. |
| `efforts` | the model's own | **Raciocínio** next to each job | The default thinking level per job: `low`, `medium`, `high` or `max`. |

A routine can override the judgment model and thinking level for itself, in its own settings (**Modelo dos julgamentos e textos**).

### Automatic model choice in chats

| Setting | Default | Where | What it does |
|---|---|---|---|
| `auto_off` | `false` | **Ajustes › Modelos › Escolha automática nas conversas** | When on, every chat request stays on `explore_model`. |
| `auto_light` | Claude Code's `haiku`, or your cheapest priced model | **Modelo leve** | Where simple requests go. |
| `auto_strong` | Claude Code's `opus`, or your dearest priced model | **Modelo forte** | Where hard requests go, while under 80% of the daily limit. |

Each request is weighed as simple, normal or hard. With a Jev key, Jev weighs it and only a probability of 60% or more moves it; otherwise a few plain rules do (short plain questions are simple, long or planning requests are hard). Normal requests, and any doubt, stay on `explore_model`. The Haiku and Opus defaults apply only when `explore_model` is a Claude Code model and the `claude` CLI is installed. A model equal to `explore_model` is not used as light or strong.

The thinking level follows the same weighing: `low` for simple requests, `high` for hard ones (not after 80% of the limit), and the job's `efforts` otherwise. In a chat, the pill under the message box fixes a model or a level for that conversation. On channels, `/modelo` and `/pensar` do the same; see the [user guide](USER_GUIDE.md#models).

### Thinking levels

`low`, `medium`, `high` and `max`, or empty for the model's own default. Each backend maps them to its own scale: Codex calls `max` "xhigh"; the OpenAI-compatible providers stop at `high`, so `max` is sent as `high`; OpenRouter gets it as `reasoning.effort`; DeepSeek, Mistral and Qwen (DashScope) get no level, because they choose thinking by model. If a provider refuses the level, the call is repeated without it.

### Judgments

| Setting | Default | Where | What it does |
|---|---|---|---|
| `judge_backend` | `local` | **Ajustes › Modelos › Quem responde os julgamentos** | `local` (**Modelo local**), `jev` (**Jev**) or `llm` (**Seu modelo**). |
| `local_judge_url` | `http://127.0.0.1:11500` | **Endereço do modelo local** | Pimpo's small judgment model (`tools/judge/serve.py`), or any server speaking the [Judge API](SDK.md#judge-api). |
| `ollama_model` | `qwen3:1.7b` | not in the app | The Ollama model the local backend tries when the local judge does not answer. |

With `local`, the local judge answers first (then Ollama, at `ollama_url`); answers it is unsure about go on to Jev, when its key is set, and then to your model. With `jev` or `llm`, that backend goes first and the others follow. The Jev key is set in **Conexões** (**Jev (julgamentos calibrados, opcional)**).

### Voices

| Setting | Where | What it does |
|---|---|---|
| `voice` | **Ajustes › Modelos › Voz dos áudios › Rotinas** | What reads the routines' audio: `auto` (a downloaded voice for the language, else the system's), `local` (a downloaded voice only), `system`, `openai` or `elevenlabs`. |
| `voice_model`, `voice_name` | same | For OpenAI, the model (`tts-1` or `tts-1-hd`) and the voice; for ElevenLabs, the voice. |
| `chat_voice` | **Ajustes › Modelos › Voz dos áudios › Ouvir no chat** | What reads answers aloud with **Ouvir**. Empty uses the routines' voice. It also accepts `browser`, the browser's own voice, which only the chat may use. |
| `chat_voice_model`, `chat_voice_name` | same | As above, for the chat. |

The OpenAI voice uses your OpenAI provider key and is billed per character, checked against the daily limit. ElevenLabs uses its own key, set on the same card. Local voices are downloaded in **Ajustes › Modelos › Baixar modelos**.

### Notifications, email requests, labs and more

| Setting | Default | Where | What it does |
|---|---|---|---|
| `suggest_off` | `false` | **Ajustes › Notificações › Sugestões** | Stops the daily suggestions. When on, once a day after 9:00 (only on Mondays after three rounds nobody took up) the judgment model sees metadata only (senders and subjects of the last 21 days' email, titles of the next 14 days' events, the routines and those that failed) and may offer up to two routines, within the daily limit and at most $0.02 a round. |
| `mute` | none | **Ajustes › Notificações** | Kinds of notices kept off Telegram, WhatsApp and system notifications: `task` (**Resultado de tarefas**), `failure` (**Rotinas que falharam**), `backup` (**Problemas no backup**). They stay in **Precisa de você**. Approval requests cannot be silenced. |
| `email_channel` | `false` | **Ajustes › Notificações › Pedir por e-mail** | Checks your mailbox every minute for messages you sent to yourself with `Pimpo:` in the subject, and treats each as a request. Only your own address counts. |
| `labs_off` | none | **Ajustes › Laboratório** | Turns off newer features: `memory_organize` (**Organizar a memória toda noite**), `meaning_search` (**Buscar na memória pelo sentido**), `mcp_registry` (**Explorar o registro MCP**). All are on by default. |
| `protection_network` | `true` | **Ajustes › Privacidade › Baixar a lista de proteção** | Downloads the signed community protection list every day. Only the list is downloaded; nothing of yours is sent. |
| `protection_url` | the project's list | not in the app | Where the protection list comes from. |
| `gallery_url` | the project's index | not in the app | The routine gallery index. A local path works too. |
| `ollama_url`, `lmstudio_url` | `http://127.0.0.1:11434`, `http://127.0.0.1:1234` | **Ajustes › Modelos › Endereços dos servidores locais** | Where Ollama and LM Studio answer, without `/v1`. |
| `custom_url` | none | **Provedores › Servidor compatível com OpenAI** | The address of your own OpenAI-compatible server, ending in `/v1`. |

## Models

**Ajustes › Modelos** (also reached from **Conexões › Inteligência**) finds what is on this computer and lets you add providers. Every model answers one real word, capped at one cent, before it is added.

### On this computer

| Backend | How Pimpo finds it | Model names |
|---|---|---|
| Claude Code | the `claude` command on `PATH` | `sonnet`, `opus`, `haiku`, `claude-…` |
| Codex | `codex` on `PATH`, or inside the ChatGPT app (`/Applications/ChatGPT.app` or `~/Applications/ChatGPT.app`) | `codex`, `codex:<model>` |
| opencode | `opencode` on `PATH`, or `~/.opencode/bin/opencode` | `opencode:<provider>/<model>`, chosen under **opencode › Escolher** |
| Ollama | a server at `ollama_url` | `ollama:<model>` |
| LM Studio | a server at `lmstudio_url` | `lmstudio:<model>` |

Claude Code is the default for every job. Each CLI runs with its own tools off, so the model reaches the world only through Pimpo's tools, where rules and approvals apply. Codex runs with your Codex config ignored and a read-only sandbox. opencode runs with your project config, Claude Code integration, external skills, auto-update and sharing turned off, every opencode tool denied, and its session deleted after each run. The [user guide](USER_GUIDE.md#models) says what was checked against each real CLI.

### Providers

Paste a key in **Ajustes › Modelos › Provedores** and choose among the provider's models. Keys are kept in the vault.

| Provider | Id | Address |
|---|---|---|
| Anthropic | `anthropic` | `https://api.anthropic.com/v1` (Messages API) |
| OpenAI | `openai` | `https://api.openai.com/v1` |
| Google Gemini | `google` | `https://generativelanguage.googleapis.com/v1beta/openai` |
| OpenRouter | `openrouter` | `https://openrouter.ai/api/v1` |
| Qwen (Alibaba DashScope) | `dashscope` | `https://dashscope-intl.aliyuncs.com/compatible-mode/v1` |
| DeepSeek | `deepseek` | `https://api.deepseek.com/v1` |
| Groq | `groq` | `https://api.groq.com/openai/v1` |
| Mistral | `mistral` | `https://api.mistral.ai/v1` |
| xAI Grok | `xai` | `https://api.x.ai/v1` |
| Ollama | `ollama` | `ollama_url`, no key |
| LM Studio | `lmstudio` | `lmstudio_url`, no key |
| OpenAI-compatible | `custom` | `custom_url`; a key only if the server asks |

All but Anthropic use OpenAI's chat completions API. Prices come from OpenRouter's public catalog of list prices. A model the catalog does not know needs its price typed in: without a price the model does not run, because the daily limit counts every call. A call also stops if it would cost more than it is allowed.

### Subscription or money

Some backends are paid by a subscription you already have. Pimpo asks each CLI how it is signed in, at most every ten minutes:

- **Claude Code** is a subscription when `claude auth status` reports a claude.ai sign-in.
- **Codex** is a subscription when `codex login status` mentions ChatGPT.
- **opencode**: GitHub Copilot, OpenCode Go and any provider signed in with OAuth (from `opencode auth list`) count as subscriptions.

A subscription call spends no money and does not count toward the daily limit. What the same work would cost on the API is recorded apart and shown under **Custo › Pela assinatura**. Everything else, including Claude Code signed in with an API key, is real spending.

## Channels

Channels are set up in **Conexões**; each card says what to create and what to paste. Tokens go to the vault. This is only what matters for configuration; the [user guide](USER_GUIDE.md#other-chat-channels) covers daily use.

| Channel | What Pimpo needs | Notes |
|---|---|---|
| Telegram | a bot token from @BotFather | Checked with Telegram before it is saved; takes effect without a restart. `PIMPO_TELEGRAM_API` points at a self-hosted Bot API server. More bots for routine notices: **Conexões › Bots extras do Telegram**. |
| WhatsApp | the Cloud API access token, the phone number id and the app secret | Meta calls Pimpo at `/webhook/whatsapp`, so Pimpo needs a public https address (for example Tailscale with Funnel, below). Pimpo makes the verify token; every call must carry Meta's signature. |
| Email requests | the mail connection (Gmail or IMAP) and `email_channel` on | See [Settings](#notifications-email-requests-labs-and-more). |
| Slack | a bot token (`xoxb-`) and an app token (`xapp-`), Socket Mode on | Direct messages. Pair by sending `pimpo` and the pairing code shown in **Conexões**. |
| Discord | a bot token | Private messages, paired the same way. |
| Signal | the address of a running `signal-cli` daemon (`http://127.0.0.1:8080`) and, optionally, its number | Paired the same way. Messages are end-to-end encrypted up to the daemon. |
| Slack or Discord notices only | an incoming webhook of a channel of your own | Messages to you only; no conversation. |
| Your own bridge | the channel API | Notices go out as signed webhooks; requests and button taps come back through the API. See [SDK.md](SDK.md#channel-api). |

Only the paired owner is answered on Slack, Discord and Signal; strangers get nothing. A channel that keeps failing for three minutes is reported on the others.

## Network and access

For a server (Docker or systemd) and reaching it safely from elsewhere, see [SELF_HOSTING.md](SELF_HOSTING.md).

### Listening address

`pimpo serve` listens on `127.0.0.1:7788`, this computer only. `--addr` changes it. The server speaks plain http, so an address other than loopback sends your login token in the clear over the network; use the two ways below instead of `--addr 0.0.0.0:…`.

### Logging in

At start, Pimpo prints a link: `http://127.0.0.1:7788/auth?token=…`. Opening it sets an http-only, same-site session cookie for a year. The token is created once and kept in the database, so the link stays valid across restarts; `PIMPO_TOKEN` replaces it. Every API call needs the cookie or `Authorization: Bearer <token>`, with the session token or a device token. The web app's files, `/api/health` and routes that check their own credential (the WhatsApp webhook, routine webhooks at `/hook/…`, the Google and Spotify sign-in callbacks, the per-exploration MCP endpoint) are the only ones reachable without it.

### Phones and other devices

**Ajustes › Celular** (**Abrir no celular**) turns on one or both ways in. Both are remembered across restarts.

- **Em casa**: Pimpo also listens on this computer's private address on the home network, on the same port as `--addr` (`http://192.168.x.x:7788` by default). Only private addresses are used. With `--addr 0.0.0.0:PORT` the server already answers on the home network, and this option only shows its address.
- **De qualquer lugar**: Tailscale runs inside Pimpo, as a node named `pimpo` (installs from before the rename keep `zodim`), and serves Pimpo with Tailscale Funnel on port 443, giving an `https://pimpo.<your-network>.ts.net` link. Funnel makes that link reachable from the internet, protected by the login token. The first time, you sign in to Tailscale in the browser; your tailnet must allow HTTPS and Funnel.
- **Usar outro endereço**: an address of your own (a reverse proxy, `tailscale serve`). It must be https, or http on a private address.

**Gerar código** pairs a device: it gets its own token, shown once as a link and QR code, and kept only as a hash. Revoke one device with the trash icon next to it; the others keep working. Device tokens also authenticate bridges and scripts that use the [API](SDK.md#authentication).

## Local models

**Ajustes › Modelos › Baixar modelos** downloads voices, speech-to-text models and, through Ollama, language models; `pimpo local` does the same from the terminal (see [Running Pimpo](#running-pimpo)).

- The list comes from a catalog built into Pimpo (`internal/local/catalog.json`). A `catalog.json` in `~/.pimpo/local` replaces it. Every entry needs an `https` address, its size and its SHA-256 (64 lowercase hex characters); items made of several files need them for each file. Ids may not contain `/`, `\` or `.`. An invalid catalog is refused as a whole: the app then keeps the built-in one and logs the error, and `pimpo local` stops with the error.
- Every file is checked against its SHA-256 before it is unpacked, and unpacks only inside its own folder. Installing needs about three times the size free, plus 2 GB.
- Voices and local transcription need a Mac: the voice engine (sherpa-onnx) is built for macOS only.
- Language models come from Ollama, which must be installed and running at `ollama_url`. Once downloaded, choose them under **Neste computador › Ollama**.
- Downloaded models are not included in backups.

Without a local speech-to-text model, voice notes use whisper.cpp (`whisper-cli` or `whisper-cpp` on `PATH`), `ffmpeg`, and the model at `~/.pimpo/models/ggml-base.bin`.

## Backups and moving

There are three kinds of copies. The [user guide](USER_GUIDE.md#moving-and-backups) has the details.

- **Local snapshots** (**Ajustes › Backup › Cópias locais**, `pimpo snapshots`, `pimpo restore`): the database and memory, every day, before each update, before an import and before a migration. The last 10 are kept in `~/.pimpo/snapshots`. A restore chosen in the app takes effect at the next start. A snapshot brings back data, not the old version of the app.
- **Export and import** (**Ajustes › Backup › Exportar e importar tudo**, `pimpo export`, `pimpo import`): one `.pimpo` file with the database, memory, installed connectors and the vault's secrets. The secrets are sealed with a passphrase of at least 8 characters; the rest of the file is not encrypted, so keep it somewhere private. Importing keeps what was there in `before-import-<date>/`.
- **Cloud backup** (**Ajustes › Backup › Backup automático na nuvem**): the same export, encrypted as a whole on this computer (database and memory included), sent daily (default) or weekly to Amazon S3 or a compatible service (R2, B2, MinIO, Wasabi) or to Google Drive. It keeps the newest copies, 7 by default, from 1 to 90. S3 needs the bucket, region, service address (empty for Amazon), an optional folder, and an access key that can read, write, list and delete there. Drive needs Google connected with Drive allowed.

Keep the passphrase outside Pimpo: without it no one can open a backup.

## Safety settings

The [threat model](THREAT_MODEL.md) says what Pimpo defends against. These are the settings that shape it.

### The vault

Secrets (tokens, API keys, passwords, the backup passphrase) are encrypted in the database. The key is 32 random bytes kept in the system keychain (service `pimpo`, account `data-key`); keys kept under the old names `zodim` and `vigia` are carried over. On a system without a keychain, the key is a `vault.key` file in the data folder with owner-only permissions: then anyone who can read the data folder can read the secrets, so protect that folder. Code outside the vault holds only a secret's name; the value is read when a connector makes a request. An export removes the encrypted secrets from its copy of the database and carries them re-sealed with your passphrase instead, so the file never depends on this computer's vault key.

### Rules and approvals

- **Safety level**, chosen on the welcome screen: conservative (asks before any change, even reversible ones), balanced (asks before anything that cannot be undone) or liberal (asks before sending anything to other people and before anything else that cannot be undone, but deleting email moves it to the trash without asking). Installs that chose liberal before 2026-09-29 had a rule that asked only before sending email; it is replaced the next time Pimpo starts, unless you had edited it.
- **Regras**: your own rules, in plain words, turned into checks that run without a model. A rule can match capabilities, a minimum risk, a routine or explorations, words in the arguments, web hosts, and people or roles. When several match, the strictest wins, and a block always wins.
- Without a matching rule, the risk decides: reversible changes go through and stay undoable, and everything else goes through. It is the safety level's rules that make changes ask, so do not delete them unless you mean it.
- The first time an exploration reaches a new web host, it asks; a host you refused stays blocked.
- An entry of the protection list blocks the call, whatever the rules say, unless you contest that entry.
- **Sempre** on an approval creates a rule for that routine and capability.
- Some capabilities always ask, whatever the rules say: WhatsApp messages to other people, and locks, alarms, covers and valves.
- A guest's request never changes anything without the person responsible for them (**Pessoas**).

### Spending

The daily limit (**Ajustes › Geral**) is checked before every paid call, so it cannot be passed by more than one call. Fallbacks never work around it. See [Daily spending limit](#daily-spending-limit).

### What leaves the computer

- The protection list download (`protection_network`) and the gallery index (`gallery_url`) are plain downloads.
- Connectors reach only what they declare, and JSON connectors never reach private addresses unless their base is this computer ([CONNECTORS.md](CONNECTORS.md)).
- Local models, local voices and local transcription keep text and audio on this computer.
