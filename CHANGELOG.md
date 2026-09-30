# Changelog

Notable changes to Pimpo. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [semantic versioning](https://semver.org) as described in [docs/RELEASES.md](docs/RELEASES.md).

## Unreleased

No version has been tagged yet. The first release will include everything built so far: routines compiled from explorations, rules and approvals, receipts, the gallery, the Guard, the desktop, web and phone apps, the chat channels, and the changes below.

### Added

- **For this routine** on approvals: a routine repeats exactly the approved action (same recipients, hosts, amounts up to the limit) without asking, until its code changes; each person revokes theirs in **Routines**.
- Settings history (**Settings › History**): every change to rules, budget, connections, models, people and settings, with who and when, secrets never kept, and undo; a member sees their own accounts' history in **Account**.
- Database integrity: Pimpo checks its database at start and before every snapshot; a damaged one is moved to a quarantine folder untouched, and a recovery page offers the newest snapshot that passes its check, a fresh start, or the damaged file as a zip.
- Dashboards in tabs with widgets of seven kinds (numbers, goals, status, lists, tables, and line, area, bar and donut charts), ready-made widgets, and sharing with the house.
- **Turn into a widget** on a routine's page, and requests to see or track something end in a widget.
- Search your past conversations by words (ignoring case and accents) or an exact phrase in quotes, each person only in their own.
- On Discord, Slack and Signal, replying to a notice with a number answers that notice, even after newer ones.
- A routine's question can be answered in words, in the app or on any channel: the answer is checked against its options (number, name or the start of one), and one that is none of them gets the options again. On WhatsApp, more than three options come as a list.
- JSON connectors: a `connector.json` that describes HTTP requests, with no program, installable as a single file.
- OpenAPI import (**Connections › From OpenAPI** and `pimpo connector openapi`): turns a REST API's description into a JSON connector.
- Desktop builds for macOS (Apple Silicon and Intel), Windows and Linux (x64 and arm64) on every release, and on demand.
- Webhooks that start a routine, routines that ask the owner and wait for the answer, and reminders.
- Google Sheets, Apple (Reminders, Notes, Calendar, Music…), Spotify, Perplexity search and a web page reader.
- Audio: routines can send audio (for example a news podcast), with cloud voices (OpenAI, ElevenLabs) or local ones (Kokoro, Piper).
- Local models downloaded by Pimpo itself from a pinned catalog, with progress: voices, Whisper transcription and Ollama models.
- **Listen** in chat streams sentence by sentence and keeps a cache.
- Automatic model choice and thinking level per chat, job and routine; Codex, opencode and subscription plans counted apart from money spent.
- Setup without Claude Code: the welcome screen sets every job up from what the computer has (Claude Code, Codex, opencode, Ollama, LM Studio) or from one API key, and tests it.
- `pimpo report`: how routines did in real use, including silent failures, late runs and time Pimpo was off.
- The proof suite runs on any model (`-model`); DeepSeek V4 Pro through opencode passes 5 of 5.
- Releases can be started from Actions with a version.
- Signed releases: `checksums.txt.sig` is checked against a release key built into Pimpo by `pimpo update` (which refuses a missing or wrong signature) and by `scripts/install.sh` when OpenSSL 3 is available. The gallery's list of authors is signed by a gallery root key built in (`pimpo gallery sign-authors`).

### Fixed

- The **liberal** safety level let irreversible actions through without asking (a GitHub comment, clearing a sheet, imported tools). It now asks before anything irreversible except deleting email, which moves it to the trash, and installs that chose it are upgraded.
- Repairs are told the error of the last failed run.
- Routines started only by a webhook no longer log a false "invalid schedule" failure.
- An answer to a question no longer runs a paused routine; **Run now** keeps a paused routine paused.
- The time a run waits for approval no longer counts toward its 15 minutes.
- Saving settings with only some fields keeps the others; the judge uses the configured Ollama address; the home network follows the port of `--addr`.
- opencode runs that start together no longer fail on a locked database.
- Windows builds (free disk space was read with a Unix-only call).
- A data race in how the language of messages was chosen.
