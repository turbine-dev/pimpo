# Changelog

Notable changes to Pimpo. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [semantic versioning](https://semver.org) as described in [docs/RELEASES.md](docs/RELEASES.md).

## Unreleased

No version has been tagged yet. The first release will include everything built so far: routines compiled from explorations, rules and approvals, receipts, the gallery, the Guard, the desktop, web and phone apps, the chat channels, and the changes below.

### Added

- JSON connectors: a `connector.json` that describes HTTP requests, with no program, installable as a single file.
- OpenAPI import (**Conexões › Por OpenAPI** and `pimpo connector openapi`): turns a REST API's description into a JSON connector.
- Desktop builds for macOS (Apple Silicon and Intel), Windows and Linux (x64 and arm64) on every release, and on demand.
- Webhooks that start a routine, routines that ask the owner and wait for the answer, and reminders.
- Google Sheets, Apple (Reminders, Notes, Calendar, Music…), Spotify, Perplexity search and a web page reader.
- Audio: routines can send audio (for example a news podcast), with cloud voices (OpenAI, ElevenLabs) or local ones (Kokoro, Piper).
- Local models downloaded by Pimpo itself from a pinned catalog, with progress: voices, Whisper transcription and Ollama models.
- **Ouvir** in chat streams sentence by sentence and keeps a cache.
- Automatic model choice and thinking level per chat, job and routine; Codex, opencode and subscription plans counted apart from money spent.

### Fixed

- Windows builds (free disk space was read with a Unix-only call).
- A data race in how the language of messages was chosen.
