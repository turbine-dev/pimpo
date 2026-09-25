# Status

What is built, what each phase gate needs, and which gates wait on people or time rather than code. Every "automated" gate runs in `make check` or `make e2e` on each commit.

| Phase | Built | Gate | Gate status |
|---|---|---|---|
| F0 Foundation and compiler proof | Runtime, compiler, event log, vault, server, UI shell | ≥ 8/10 recorded tasks compile on the first attempt and pass a holdout | **Passed: 8/10** ([proof](proof/README.md)) |
| F1 The compiled routine | iCal, IMAP, HTTP and Telegram connectors; exploration over MCP; scheduler; judgments (Jev, LLM, Ollama); daily budget; owner channel; UI with real data | 30 consecutive daily runs of the morning brief, ≥ 29 successful, ≤ $0.01 per run, every failure alerted within 5 minutes | **Waiting on time:** needs 30 days of real use. The live test with Claude passes (`go test -tags live -run Live ./internal/app`) |
| F2 Secure by construction | Rules engine, approvals that pause the run, reversible forms, undo, receipts, repairs that keep old tests, cost view | 200-email scenario stopped and reversible; 50 injections leak nothing; runaway loop stops within budget | **Passed, automated** (`go test -run Gate ./internal/app`) |
| F3 The UI that tells the story | Demo mode, first-run guide, compile moment, decisions shown in explorations, version diffs, palette, PWA, WCAG AA palette | 4 of 5 new users finish four tasks unaided, each in under 10 minutes | **Waiting on people:** usability sessions. The four tasks pass as browser tests, plus WCAG AA on every screen in both themes |
| F4 Three perfect uses | Email triage (drafts, archive, trash, unsubscribe), versioned memory, snapshots and safe upgrades, local judge (Qwen3-0.6B + LoRA on MLX) with a cascade to Jev | 30 days of the three routines at ≥ 97% success and < 2% undone | **Waiting on time.** Judge: 51% → 85% tuned, 92% with the cascade at 22% escalation ([results](../tools/judge/results/README.md)) |
| F5 Install and migrate → v1.0 | Desktop app (Tauri, menu bar, sidecar, starts at login), one-line installer with checksums and systemd, goreleaser for Linux/macOS/Windows/RPi, CI matrix, OpenClaw and Hermes migration with a report per skill, Google sign-in | 3 non-technical people install in ≤ 15 min; 3 real installs migrate with ≥ 80% of skills working | **Waiting on people** (and on the end of the stand-by). Installer, migration and bundles are tested; the macOS DMG builds |
| F6 Memory with provenance, family, WhatsApp | Provenance and trust on every fact, people with roles and responsible persons, per-person memory, accounts and chats, per-person rules, WhatsApp Cloud API with signed webhooks | 50 memory poisoning attacks 100% blocked; isolation tests 100% | **Passed, automated** (`TestGatePoisonedMemoryNeverBecomesInstruction`, `TestGateFamilyIsolation`) |
| F7 Gallery | Ed25519-signed, content-addressed routines; audit run with every capability reachable; revocation; install review; publishing; 10 starter routines shipped in the binary | 50 community routines with zero divergences; confirmed reports removed within 72 h | **Waiting on community.** The verifier runs on every index in CI and on every install |
| F8 More channels and connectors | Connector catalog (RSS, GitHub, Todoist, Notion, Obsidian, Home Assistant, Slack, Discord), external connectors over MCP with contracts (installed from a zip or folder, no restart), voice notes (whisper.cpp), email channel, generic channel API | 15 connectors with green contract tests against the real APIs for 4 consecutive weeks | **Waiting on time:** 15 connectors exist with contract tests against fake servers; live probes (`Testar`) need 4 weeks of real credentials |
| F9 Small business | **Removed from the roadmap** (2026-09-25): a back office is a different product and blurred what Zodim is for. Photos read with Tesseract stay, as a channel feature | — | — |
| F10 Native apps | Desktop notifications, per-device pairing tokens with revocation, iOS and Android companion (Tauri) built from the same project, phone-first navigation | Median alert-to-tap < 3 s over 20 approvals; pairing and revocation pass on iOS, Android, macOS, Windows, Linux | **Partly automated:** pairing and revocation tests pass; the iOS app runs in the simulator; the Android APK builds but was not run (the local system image is incomplete); timing needs real devices |
| F11 Protection network and Guard | Signed protection list (domains, skills, patterns), daily refresh, starter list in the binary, local override of false positives; Guard plugins for OpenClaw (`before_tool_call`) and Hermes (`pre_tool_call`) | 100 installs using the list; ≥ 1 real block from it; < 5% contested; Guard contract tests on the two latest versions of each agent | **Waiting on community.** Guard suites pass against a real Zodim (`TestGuardPluginsAgainstZodim`); testing against real OpenClaw and Hermes releases is pending |
| F12 Longevity | Stable extension SDK (connectors, channels, judges, Guard, gallery, protection), Portuguese and English UI, security policy and threat model with a test per defense, LTS policy, governance, RFC process, code of conduct, user guide | 3 maintainers besides the founder; a published independent audit; LTS fixes within 7 days | **Waiting on people.** `govulncheck`: no reachable vulnerabilities (toolchain go1.26.8) |

## Everywhere

- **Web**: every feature, in Portuguese and English, installable as a PWA.
- **Desktop**: macOS DMG built and run locally; Windows and Linux bundles are built by the release workflow.
- **Phone**: iOS build verified in the simulator (pairing screen). The Android APK builds. The phone reaches Zodim at home over the Wi-Fi (no account) or from anywhere through the Tailscale built into Zodim; with both, it tries home first. The embedded Tailscale was checked up to the sign-in step; the Funnel link needs an owner's Tailscale account.

## Export, import and extension

- `zodim export` / `zodim import`, and **Ajustes › Exportar e importar tudo**, move everything (database, memory, connectors, secrets sealed with a passphrase) between machines.
- Connectors, channels, judges and Guard clients plug in without recompiling ([SDK](SDK.md), [connectors](CONNECTORS.md)).

## After the roadmap: closing the gaps with OpenClaw

Ordered by Jev (`tools/jev/decisions_gaps.py`), all built and tested:

1. **Connectors from the MCP registry** or by command or URL, with each tool's risk reviewed before installing (stdio and streamable HTTP).
2. **Run history** of all routines, and schedules every 5 to 30 minutes.
3. **Memory** organized every night (question and threshold calibrated with Jev) and searched by meaning.
4. **Chat** in the app: changes are rehearsed, then confirmed exactly as shown.
5. **Assistants**: named roles limited to their tools by the host.
6. **Help** that reads the user guide, **notification** settings by kind, and **Labs**.
7. **Voice**: dictation in the chat and answers read aloud.
8. **Web search** with Brave or SearXNG.
9. **Channels**: Discord, Slack and Signal, besides Telegram and WhatsApp.
10. **Models**: Anthropic, OpenAI, OpenRouter or Ollama per job, each with its price for the budget.
11. **Cloud backups** to S3-compatible storage or Google Drive, encrypted before upload.

Checked only against fake servers so far (they need the owner's accounts): Brave and SearXNG, Discord, Slack, signal-cli, the model APIs, S3 and Google Drive. The MCP registry search was checked live.

## Next: what users of other agents miss most

Ordered by Jev (`tools/jev/decisions_next.py`) from complaints about Hermes and OpenClaw:

1. **Routines that react to something new** (a watched read, polled without a model). Built.
2. **A writing step** in routines: a small model writes one text per item, bounded and billed. Built.
3. **An alert when a channel stops working**, sent on the others and again when it comes back. Built.
4. **Going back after an update**: the copy made before each update, like the daily ones, can be put back from Ajustes › Backup, and the owner is told after an update. It restores data, not the old binary (there is no auto-updater to roll back). Built.

## Pending decisions for the owner

- **License**: the plan says free and open source; the exact license is not chosen yet.
- **Publishing**: the `zodim-gallery` and `zodim-protection` repositories, releases and signing keys (the maintainer keys are in `~/.config/zodim/`).
- **Android emulator image**: about 1.5 GB to download before the APK can be run locally.
