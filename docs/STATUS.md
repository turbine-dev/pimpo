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
| F9 Small business | **Removed from the roadmap** (2026-09-25): a back office is a different product and blurred what Pimpo is for. Photos read with Tesseract stay, as a channel feature | — | — |
| F10 Native apps | Desktop notifications, per-device pairing tokens with revocation, iOS and Android companion (Tauri) built from the same project, phone-first navigation | Median alert-to-tap < 3 s over 20 approvals; pairing and revocation pass on iOS, Android, macOS, Windows, Linux | **Partly automated:** pairing and revocation tests pass; the iOS app runs in the simulator; the Android APK builds but was not run (the local system image is incomplete); timing needs real devices |
| F11 Protection network and Guard | Signed protection list (domains, skills, patterns), daily refresh, starter list in the binary, local override of false positives; Guard plugins for OpenClaw (`before_tool_call`) and Hermes (`pre_tool_call`) | 100 installs using the list; ≥ 1 real block from it; < 5% contested; Guard contract tests on the two latest versions of each agent | **Waiting on community.** Guard suites pass against a real Pimpo (`TestGuardPluginsAgainstPimpo`); testing against real OpenClaw and Hermes releases is pending |
| F12 Longevity | Stable extension SDK (connectors, channels, judges, Guard, gallery, protection), Portuguese and English UI, security policy and threat model with a test per defense, LTS policy, governance, RFC process, code of conduct, user guide | 3 maintainers besides the founder; a published independent audit; LTS fixes within 7 days | **Waiting on people.** `govulncheck`: no reachable vulnerabilities (toolchain go1.26.8) |

## Everywhere

- **Web**: every feature, in Portuguese and English, installable as a PWA.
- **Desktop**: macOS DMG built and run locally; Windows and Linux bundles are built by the release workflow.
- **Phone**: iOS build verified in the simulator (pairing screen). The Android APK builds. The phone reaches Pimpo at home over the Wi-Fi (no account) or from anywhere through the Tailscale built into Pimpo; with both, it tries home first. The embedded Tailscale was checked up to the sign-in step; the Funnel link needs an owner's Tailscale account.

## Export, import and extension

- `pimpo export` / `pimpo import`, and **Ajustes › Exportar e importar tudo**, move everything (database, memory, connectors, secrets sealed with a passphrase) between machines.
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
5. **Prompt caching** with Anthropic's API: system prompt, tools and the growing conversation are cached; cache writes and reads are priced in the budget. Checked against a fake server; needs an owner's API key to check live.
6. **Desktop as a client of a remote Pimpo**: the menu bar switches between the local server and one elsewhere, paired with the phone's link; the local server stops meanwhile. The connection page was checked in the browser against a real server with the native calls stood in for; the native switch itself was built but not clicked through, since the owner's app was running.
7. **iMessage**: cut (needs full disk access to read Messages).

## Pimpo

Renamed from Zodim (after Vigia) to Pimpo, the owner's tuxedo cat, with a black-and-white identity: the app icon, tray silhouette, favicon and logo are the cat's head, and the interface's accent is black on light and white on dark (color is kept only where it carries meaning). Existing installs carry over: ~/.zodim moves to ~/.pimpo, the keychain key is copied, old backups (including their secrets, which the first rename missed) and the old Drive folder still work, `zodim <code>` still pairs, the phone keeps its saved link, and a Tailscale node that already existed keeps its name so paired phones keep their link. The mascot lives in the app and, in the desktop app, optionally on the desktop in a small always-on-top window.

## Complex routines, Pimpo's way

Asked for after comparing with OpenClaw and Hermes skills (folders of scripts run by the agent), built without running arbitrary code:

1. **State between runs** (`state.get/set`, 64 KB, saved only when a run succeeds), tested with `state`/`expect_state` in scenarios. Checked live: Claude compiled "only if the dollar went up" into a stateful routine on the first attempt.
2. **Routines in a repository**: one folder per routine, export with a local commit, push on click, pull fast-forward only, every change checked (tests, audit, capability diff) and applied only by the owner. Tested end to end with real git repositories.
3. **Routines using routines** (`routines.run`, `manifest.uses`): the caller must declare everything the helper touches, three levels at most, no loops, helpers' state read-only. Checked live: Claude reused an installed routine when asked.

## From the OpenClaw documentation (2026-09-27)

Ranked with Jev (`tools/jev/decisions_openclaw_docs.py`), then ordered by user value and dependencies, since the formula penalises table stakes: model setup redone (detection of Claude Code, Ollama and LM Studio; eleven providers; prices from OpenRouter's public catalog, never guessed; test before use), fallback models per job, spending by model and job, one conversation per chat channel with typing indicators, and a check-up that tests every part for real. Left out on purpose: a browser tool (against Pimpo's safety model; an MCP browser server can be added), group chats, a Codex backend, and auto-update (waits on the owner's signing and publishing decisions).

## Languages

The app, server messages (channels, approvals, notices), built-in connector texts, routine dates and money, and the phone/desktop shell page are in ten languages: pt, en, es, fr, de, it, ja, zh, ko, ru, with each language's plural rules. Portuguese is the source; the translations were written by a model and checked for keys, {slots} and plural forms, not reviewed by native speakers yet. Texts speak about Pimpo in the third person; first person is kept only for the owner's own words (example requests, preset rules). The user guide is English only.

## Roadmap to v1.0 (docs/ROADMAP.md)

| Item | Status |
|---|---|
| 1.1 Known bugs | **Done** (2026-09-29): 12 bugs fixed with tests, including the liberal level letting irreversible actions through. |
| 1.3 Real-account validation | **Tooling built** (2026-09-29): `pimpo report` and `GET /api/report` measure runs, failures, silent failures, late runs, time Pimpo was off (a heartbeat records it from now on), repairs, approvals and cost; [VALIDATION.md](VALIDATION.md) says how. On the owner's own data for 14 days: 30 runs, 4 failed, 0 silent. **Waiting on time and people:** three households for three weeks. |
| 2.1 Auto-update with rollback | **Built** (2026-09-29): the desktop app checks the stable or beta channel at start and every six hours, installs signed updates and goes back to the previous version with the data as it was (restoring the pre-update snapshot); `pimpo update` and `--rollback` for servers. The signed macOS bundle was built and signed locally. **Waiting on the owner:** the two `TAURI_SIGNING_PRIVATE_KEY` secrets in the repository. |
| 2.2 Official signing | **Ready, waiting on the owner:** the workflow signs and notarizes macOS with the `APPLE_*` secrets and signs Windows installers with `WINDOWS_CERTIFICATE`; both need accounts only the owner can open ([RELEASES.md](RELEASES.md)). |
| 2.3 Docker and a server install | **Built** (2026-09-29): multi-arch image (non-root, `/data` volume, health check) published to ghcr.io by release and on main, `compose.yaml`, [SELF_HOSTING.md](SELF_HOSTING.md). Built and run locally: healthy, same login after restart, `pimpo report` inside. |
| 2.4 Packages | **Ready, waiting on the owner:** a Homebrew cask for the command line (GoReleaser) and one for the desktop app (`scripts/cask.py`), published to `turbine-dev/homebrew-tap` with `HOMEBREW_TAP_TOKEN`; winget updates with `WINGET_TOKEN` after a first manual submission; a Flatpak manifest and metainfo, not built yet (needs Linux). See `packaging/`. |
| 2.5 Website | **Built** (2026-09-29): `site/`, published by `pages.yml`, with downloads for the visitor's system from the latest release. **Waiting on the owner:** Pages set to GitHub Actions, a domain (`site/CNAME`), and a demo video. |
| 3.3 Proactivity | **Built** (2026-09-29): a daily round of at most two routine suggestions from metadata only, accepted into an ordinary exploration, declined ones never repeated, backing off to Mondays; counted in `pimpo report`. **Waiting on time:** the acceptance rate in real use. |
| 3.1 Gallery and protection list | **Gallery built** (2026-09-29): 51 signed, audited routines with tests (from 11): the 20 that passed the OpenClaw proof and 20 new ones, each imported with its tests and the audit. **Waiting on the owner:** creating the `pimpo-gallery` and `pimpo-protection` repositories. |
| 3.2 SKILL.md skills | **Built** (2026-09-29): install from GitHub or a zip, review, grant capabilities (none means none); the agent loads a skill when a task matches and is then limited to its capabilities; scripts never run; the protection list refuses known skills; tested with Anthropic's public `internal-comms` skill. **Waits on 4.2** for skills that need their scripts. |
| 4.1 Agent-driven browser | **Built** (2026-09-29, Labs, off by default): `browser.open/read/follow/type/choose/click` on Pimpo's own Chrome profile, scoped by host, clicks irreversible, passwords never typed, a window to sign in to sites ([RFC 0002](rfcs/0002-browser.md)); tested with real Chrome against a local site. **Waiting on use:** the ten real sites of the roadmap's measure. |
| 4.2 Code sandbox | **Built** (2026-09-29, Labs, off by default): `code.run` in a Docker container with no network, read-only root, no capabilities, an unprivileged user, no environment and limits ([RFC 0001](rfcs/0001-code-sandbox.md)). The isolation flags are tested on every build. **Waiting on the owner:** the live test pulls official images (`alpine`, `python`) and runs with `PIMPO_SANDBOX_LIVE=1`; a network allowlist and an SSH backend are follow-ups. |
| 4.3 Learning the owner | **Built** (2026-09-29, on by default, off in Ajustes): a weekly round learns up to five preferences from the owner's own requests and decisions only, into memory as *learned* with the evidence; confirmed or removed in Memória, removed ones never relearned; $0.03 a round at most. **Waiting on time:** how many learned preferences owners keep. |
| 5.1 Stable formats | **Built** (2026-09-29): [FORMATS.md](FORMATS.md) freezes the database (`user_version` 1, a newer one is refused), routine, connector, memory and backup formats; `internal/compat` opens data written by the version that froze each format on every build. **Waiting on the owner:** tagging v1.0, which starts the first LTS line. |
| 5.2 The phone as part of the agent | **Built** (2026-09-29): **Celular** on the paired phone shares location, camera and shortcuts, each turned on on the phone; places with a radius or by name; arriving and leaving while the app is open or from iOS Shortcuts and Tasker with a key that only reports events; photos read on the computer; routines watch `phone.arrivals`, `phone.photos` and `phone.shortcuts` and run as soon as the phone reports. Tested end to end: a routine runs on arriving home and another on a photo of a bill. **Waiting on the owner:** a build of the phone app with the new camera and location permissions; background location without an automation needs a native plugin (a follow-up). |
| 5.3 Voice conversation | **Built** (2026-09-29): **Conversar** in the chat, on desktop and phone: the wake word ("Pimpo") and every utterance are transcribed by Whisper on the owner's Pimpo, voice activity detection ends turns, answers are read with the chat's voice, talking interrupts them, follow-ups need no wake word for 12 s; actions are never approved by voice. **Waiting on use:** tuning the thresholds in noisy rooms; a wake word while the app is closed needs a native keyword spotter (a follow-up). |
| 5.4 Parallel agents and long tasks | **Built** (2026-09-29): **Trabalhos** plans a job into up to eight parts, each with only the capabilities it needs; the owner reviews the plan and starts it with a budget; parts run three at a time in the background (up to 90 minutes and 80 turns each), are saved after every step and resumed after a restart; the job stops at its budget; a final report puts the parts together. **Waiting on use:** a real multi-hour job to tune the plan and the per-part limits. |
| 5.5 More channels | **Built** (2026-09-29): **iMessage** on the Mac (Messages' database read-only with Full Disk Access, answers through AppleScript; tested with a copy of the database's tables) and **personal WhatsApp** through a local WAHA bridge, off until chosen in Labs, with the ban warning, and never a way to approve (tested against a fake bridge). **Waiting on the owner:** a live test with a second Apple ID and with WAHA on a number that can be lost. |
| 6.1 A login per person | **Built** (2026-09-30): the first visit creates the administrator's account (name, then a passkey); devices belong to a person and act only for them; the administrator pairs a phone for someone; removing a person revokes their devices. In Portuguese the role is called *administrador*. |
| 6.2 Privacy between people | **Built** (2026-09-30): an access rule for every route (tested), everything personal scoped to its person with the owner seeing none of it, members answering their own approvals, others' events streamed as types only; a test calls every route across people. |
| 6.3 Passkeys and sessions | **Built** (2026-09-30): passkeys per person in **Conta** (go-webauthn), a sign-in screen, passkey sessions that expire after 30 unused days and devices after 180, a limit on wrong sign-ins (requests without a credential do not count); tested end to end with a software authenticator, including a forged signature and a removed person. **Waiting on use:** trying it with Touch ID, an iPhone and Windows Hello. |
| 1.2 Setup without Claude Code | **Built** (2026-09-29): the welcome screen sets every job up from what the computer has or from one API key, testing the main model. Proof without Claude Code: DeepSeek V4 Pro through opencode, **5/5** first-attempt accepted and passing the holdout, $0.06. **Waiting on people:** five new users reaching a routine unaided; **waiting on keys:** proofs on two API providers and on a local model (Ollama has no model installed here, and the disk has 9 GB free). |

## Pending decisions for the owner

- **License**: Apache License 2.0 (chosen 2026-09-29).
- **Publishing**: the `pimpo-gallery` and `pimpo-protection` repositories, releases and signing keys (the maintainer keys are in `~/.config/pimpo/`).
- **Android emulator image**: about 1.5 GB to download before the APK can be run locally.
