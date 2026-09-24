# Vigia

A personal AI agent that turns what you ask into software.

The first time you ask for something ("every morning, send me my agenda and the emails that matter"), Vigia does it with a language model, while you watch, simulating any change. When it works, Vigia compiles the task into a **routine**: readable JavaScript with tests and a list of the only things it may touch. From then on the routine runs on schedule without a language model. A small judgment model answers the few subjective questions ("is this email important?").

- **Reliable:** a routine is code with tests, not a prompt re-interpreted every time.
- **Cheap:** routines cost cents, not dollars a night.
- **Safe by construction:** routines reach the world only through declared capabilities. Every action passes your rules and lands in a tamper-evident log, and anything irreversible waits for you.

## Everywhere you are

| | |
|---|---|
| **Web** | the full app at `http://127.0.0.1:7788`, installable as a PWA |
| **Desktop** | macOS, Windows and Linux (Tauri): menu bar, starts at login, system notifications |
| **Phone** | iOS and Android companion, paired by QR code, focused on approvals and receipts |
| **Chat** | Telegram, WhatsApp (official API), email, voice notes, Slack, Discord, or any bridge through the channel API |

## What it does

- **Routines** you ask for in plain words, with a gallery of signed, audited routines to install.
- **Email triage**, agenda, bills, weather, news, tasks, notes, smart home, GitHub, and more connectors.
- **A whole house:** people with roles, their own memory and accounts, and approvals sent to the person responsible.
- **Small business:** clients, quotes, invoices as PDF, payment links, and reminders that always ask first.
- **Community protection:** a signed list of exfiltration domains and dangerous patterns, which also guards OpenClaw and Hermes through [Guard plugins](guard/).
- **Your data, portable:** export everything to one file and import it anywhere; import from OpenClaw or Hermes.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/denerFernandes/vigia/main/scripts/install.sh | sh
```

Or download the desktop app from the releases page. Then run `vigia serve` and open the link it prints. Exploration and compilation use Claude Code (`claude`) with your own subscription; the rest runs locally.

## Build from source

```bash
make build
./bin/vigia serve --demo
```

Requires Go 1.25+ (the toolchain pins 1.26.8) and Node 24. `make desktop` builds the desktop app, and `make check` runs every check.

## Documentation

- [User guide](docs/USER_GUIDE.md)
- [Extending Vigia](docs/SDK.md): connectors, channels, judges, routines, and the Guard, none of which needs recompiling
- [Threat model](docs/THREAT_MODEL.md) · [Security](SECURITY.md) · [Releases](docs/RELEASES.md)
- [Contributing](CONTRIBUTING.md) · [Governance](GOVERNANCE.md) · [Code of Conduct](CODE_OF_CONDUCT.md)
- [Plan](docs/PLANNING.md) · [Status](docs/STATUS.md)

Free and open source, with no telemetry.
