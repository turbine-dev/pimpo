# Contributing

Thanks for helping. A few things keep Pimpo trustworthy:

- **Everything is tested.** A behavior without a test is a bug waiting. Run `make check` before every commit; `scripts/commit.sh` refuses to commit when it fails.
- **English in code and docs.** The app itself speaks ten languages: Português, English, Español, Français, Deutsch, Italiano, 日本語, 简体中文, 한국어 and Русский. Any text a person sees goes into all of them.
- **Read [ENGINEERING.md](ENGINEERING.md).** Code should read like the code around it: few comments, and only ones that say why.
- **Safety changes go through an RFC.** Anything touching the rules engine, approvals, the vault, the runtime's capabilities, or what counts as low trust needs one (see below).

## Where to start

- **Connectors** are the easiest way in, and most need no Go at all: a JSON file, an MCP server or an OpenAPI description. See [docs/CONNECTORS.md](docs/CONNECTORS.md).
- **Gallery routines**: see [docs/ROUTINES.md](docs/ROUTINES.md) and [gallery/README.md](gallery/README.md).
- **Protection list entries**: see [protection/README.md](protection/README.md).
- **Translations**: fixing a phrase that reads badly in your language is always welcome.
- Issues labeled `good first issue`.

## Setting up

You need Go (the version in `go.mod`; the toolchain downloads itself), Node 24 (see `.nvmrc`) and, for the desktop app, Rust stable with the [Tauri prerequisites](https://tauri.app/start/prerequisites/) for your system.

```bash
git clone https://github.com/turbine-dev/pimpo.git
cd pimpo
make build
./bin/pimpo serve --demo
```

`--demo` starts with sample data and fake connectors, so nothing reaches your real accounts. Open the address it prints.

| Command | What it does |
|---|---|
| `make build` | web app and server into `bin/pimpo` |
| `make check` | formatting, vet, type checks, UI tests, web build and Go tests with the race detector: run it before every commit |
| `make e2e` | browser flows and accessibility checks against the demo (Playwright) |
| `make proof` | the routine proof suite: explores and compiles real tasks with a model (uses your Claude Code subscription) |
| `make desktop` | the desktop app for this computer, with this computer's server inside |
| `make install` | on macOS, puts that app in /Applications, keeping the previous one |

Live tests that talk to real services or model CLIs sit behind the `live` build tag and never run in CI: `go test -tags live ./internal/llm/`.

## How the code is laid out

| Path | What lives there |
|---|---|
| `cmd/pimpo` | the command line and server entry point |
| `cmd/proof` | the proof harness that explores, compiles and checks routines |
| `internal/app` | the HTTP API and how the parts are wired together |
| `internal/explore`, `internal/compiler` | exploring a task with a model, and compiling it into a routine |
| `internal/runtime`, `internal/host`, `internal/routine`, `internal/trace` | running routines and checking them against their tests |
| `internal/capability`, `internal/policy`, `internal/approval`, `internal/undo` | what routines may do, the rules, approvals and undo |
| `internal/connector/...` | built-in connectors; `external` runs MCP, JSON and OpenAPI connectors |
| `internal/llm`, `internal/models`, `internal/judge` | model providers and CLIs, and the judgment model |
| `internal/vault`, `internal/event`, `internal/store` | secrets, the tamper-evident log and storage |
| `ui/` | the React web app, also used inside the desktop and phone apps |
| `desktop/` | the Tauri shell for macOS, Windows, Linux, iOS and Android |
| `gallery/`, `protection/`, `guard/` | the routine gallery, the protection list and the Guard plugins |
| `examples/connectors` | complete connectors to copy from |
| `docs/` | user and developer documentation, published on the website by `scripts/site_docs.py` (run it and open `site/docs/index.html` to preview; a new document needs a line in its `SECTIONS`, a new user-guide section a place in `GUIDE`) |

## RFCs

Copy `docs/rfcs/0000-template.md` to `docs/rfcs/NNNN-short-name.md` and open a pull request. Discussion stays open at least 7 days. Two maintainers approve, one of them outside the author's organization.

## Pull requests

- Keep each pull request to one change, with tests and a description of why.
- Two approvals for code under `internal/policy`, `internal/vault`, `internal/runtime`, `internal/host` or `internal/approval`; one elsewhere.
- Commits are signed off (`git commit -s`): you certify the [Developer Certificate of Origin](https://developercertificate.org), and your contribution is licensed under the project's [Apache License 2.0](LICENSE).
- CI runs on Linux, macOS and Windows. A pull request that touches `desktop/` also builds the desktop app for all three.

## Releases

Maintainers tag `vX.Y.Z`; CI builds the command-line binaries, the desktop installers and a draft release, which a maintainer checks and publishes. See [docs/RELEASES.md](docs/RELEASES.md).

## Conduct

Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md).
