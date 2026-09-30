# Roadmap

What stands between Pimpo and being a serious alternative to OpenClaw and Hermes Agent, in the order we plan to build it. Written 2026-09-29.

Pimpo already does what they do not: tasks become tested routines that run without a language model, and every action passes rules, approvals, undo and a tamper-evident log outside the model. What it lacks is mostly **distribution and ecosystem** (nobody can install it yet, and there is little to install into it), and a few **open-ended powers** that people use those agents for every day: a browser, a shell, long tasks.

The order comes from scoring each item with Jev (TypeSafe) on user value, differentiation, how much its absence blocks adoption, build risk and safety risk (see [the scores](#appendix-scores)), then reviewing the result by hand. Where the order differs from the scores, the reason is written down.

## Principles

- **Foundation before features.** A release nobody can install, or one that breaks on real accounts, makes every feature worthless.
- **New powers keep the promises.** Anything that gives the agent new reach (a browser, a shell, a phone) goes through an [RFC](rfcs/), runs through capabilities, rules and approvals like everything else, and is covered in the [threat model](THREAT_MODEL.md) before it ships.
- **Explore once, then compile.** Each new power should also work inside routines, so it becomes cheap and repeatable instead of a model call every time.
- **Measured, not assumed.** Every milestone ends with criteria checked on real use, not only on the proof suite.

## Milestones

| | Version | Theme | Items |
|---|---|---|---|
| **M1** | v0.6 | Solid ground | fix the known bugs · setup without Claude Code · real-account validation · first release |
| **M2** | v0.7 | Easy to get and keep | auto-update with rollback · official signing · Docker and VPS · packages · website and demo |
| **M3** | v0.8 | An ecosystem | gallery and protection list published, 50 routines · SKILL.md skills · proactivity |
| **M4** | v0.9 | Hands | agent-driven browser · code sandbox · learning the owner |
| **M5** | v1.0 | Everywhere | stable formats · phone as part of the agent · voice conversation · parallel agents and long tasks · more channels |
| **M6** | v1.1 | Each person their own | a login per person · privacy between people, the owner included · passkeys and sessions |
| **M7** | v1.2 | At a glance | dashboards · routines as widgets · floating desktop widgets · Android and iOS home-screen widgets |

Sizes: **S** is days, **M** one to two weeks, **L** several weeks. Items marked **owner** need something only the project owner can do (an account, a certificate, a decision).

---

## M1 · Solid ground (v0.6)

Goal: someone who is not the author installs Pimpo, sets it up in minutes with what they already have, and it keeps working on their real accounts.

### 1.1 Fix the bugs found while documenting · S

Found while writing [ROUTINES.md](ROUTINES.md) and [CONFIGURATION.md](CONFIGURATION.md):

- repair never receives the failure message (`Explore.Repair` is always called with an empty problem);
- routines started only by a webhook log a false "invalid schedule" failure on every reschedule;
- answering a question runs a paused routine, where a webhook refuses with 409;
- approvals wait 30 minutes while a scheduled run is cancelled after 15;
- **Run now** activates a paused routine;
- `pimpo routines import --data DIR` fails when DIR does not exist;
- the over-budget message points to **Cost**, but the limit is set in **Settings › General**;
- the provider validation message lists 4 providers of 13;
- the judge ignores the configured Ollama address;
- `PUT /api/settings` empties the fields it does not receive;
- the home-network listener ignores the port in `--addr`;
- the **liberal** safety level lets irreversible actions through without asking, unlike what its label says.

**Done when** each has a test that failed before the fix, and the docs describe the fixed behavior.

### 1.2 Setup in minutes without Claude Code · M

Today exploring and compiling assume Claude Code. A new user should get to their first routine with **only an API key** (any of the 12 providers) **or a local Ollama**, in under five minutes.

- The welcome screen detects what is there (Claude Code, Codex, opencode, Ollama, LM Studio) and otherwise asks for one API key, tested with one word.
- It picks sensible models per job for that provider and a daily limit, and says what a routine will cost.
- Exploration and compilation are proven on at least two API providers and one local model, with the proof suite's success rate recorded for each.

**Done when** five people who never used Pimpo reach a working routine without help, on macOS, Windows and Linux, with at least one of them on Ollama only.

### 1.3 Validate with real accounts · M (weeks of calendar time)

The proof suite passes 20 of 20 on simulated data. Real mailboxes, calendars and sheets are messier.

- Run the 10 most common routines (morning brief, inbox triage, bills, price watch, news podcast, reminders…) on at least three real households for three weeks.
- Record every failure, silent or loud, and whether repair fixed it.
- Turn each new failure into a scenario in the proof suite.

**Done when** three weeks pass with no silent failure, every loud failure repaired or explained, and irreversible actions always approved.

### 1.4 First release · S · owner

- Push the pending work and get CI green on Linux, macOS and Windows.
- Release **v0.6.0** from **Actions › release** (binaries with checksums, desktop installers for macOS, Windows and Linux, a draft release reviewed by hand).
- Changelog, release notes in English and Portuguese, and the install command working against the release.

**Done when** `curl … | sh`, the `.dmg`, the `.exe` and the `.deb` each install and start on a clean machine.

---

## M2 · Easy to get and keep (v0.7)

Goal: installing, trusting and updating Pimpo is as easy as any mainstream app, on a laptop or a server.

### 2.1 Auto-update with rollback · M

- The desktop app checks for updates (Tauri updater, update manifests signed with a key held by two maintainers), shows what changed, and installs on the next restart.
- A snapshot is taken before every update, and **go back to before the update** is one click, as the release policy promises.
- The server binary updates itself too (`pimpo update`), with the same checks.
- Beta and stable channels.

**Done when** an update from v0.7.0 to v0.7.1 happens without a manual download on all three systems, and a broken update rolls back.

### 2.2 Official signing · S · owner

- **macOS:** an Apple Developer ID certificate and notarization. The workflow already signs and notarizes when the `APPLE_*` secrets exist (**owner**: the Apple Developer Program membership and the secrets).
- **Windows:** Authenticode signing of the installers, with a certificate or Azure Trusted Signing (**owner**: the account).

**Done when** the installers open without "unidentified developer" or SmartScreen warnings.

### 2.3 Docker and a server install · M

Much of OpenClaw's and Hermes's use is on an always-on server.

- An official multi-arch image (amd64, arm64) published to GitHub Container Registry, running as a non-root user with the data folder as a volume.
- A `compose.yaml` with health checks, and a guide for a VPS and a home server (Raspberry Pi), with remote access over Tailscale (already built in) instead of open ports.
- The desktop and phone apps connect to that server instead of running their own.
- Backups of the volume covered by the existing cloud backup.

**Done when** a new VPS runs Pimpo in under ten minutes following the guide, and the desktop app is paired with it.

### 2.4 Packages · S

Homebrew (formula and cask), winget, Flathub and a Debian/RPM repository, updated by the release workflow.

**Done when** `brew install pimpo`, `winget install Pimpo` and `flatpak install pimpo` each install the latest release.

### 2.5 Website and demo · M · owner

A site (**owner**: the domain) with what Pimpo does differently, a two-minute video of a request becoming a routine, the documentation, and the downloads per system. A comparison with OpenClaw and Hermes that is factual and dated.

**Done when** the site is live, linked from the README and the release notes.

---

## M3 · An ecosystem (v0.8)

Goal: a new user finds useful things to install on day one, and anyone can share what they built.

### 3.1 Publish the gallery and the protection list · M · owner

- Create the `pimpo-gallery` and `pimpo-protection` repositories (**owner**), with their signing keys held by two maintainers, and point Pimpo's defaults at them.
- Grow the gallery from 11 to about **50 routines** that each solve a real, common need, each signed, audited and with tests, in the 10 languages.
- A contribution path: a pull request template, CI that verifies signatures and audits, and credit for authors in the app.

**Done when** 50 routines install from the app, and at least five came from people other than the maintainers.

### 3.2 SKILL.md skills · L

OpenClaw's ClawHub and the agentskills.io format have thousands of skills. Pimpo can use them without inheriting their risk.

- Install a skill from a folder, a URL or ClawHub. Pimpo reads its `SKILL.md`, shows what it asks for, and maps it to **declared capabilities**: a skill can only use what the owner allowed, and scripts inside it run only in the sandbox (4.2).
- Skills are text that reaches the model, so they count as low-trust content: they can never grant themselves capabilities or change rules.
- The protection list flags known-malicious skills.
- A skill that works is offered for compilation into a routine, like any exploration.

**Done when** the 50 most installed ClawHub skills that make sense for a personal agent install and work, each limited to what it declares, and the known-malicious ones are refused.

### 3.3 Proactivity · M

The thing OpenClaw and Hermes users love most is an agent that starts the conversation.

- A daily (configurable) loop where a cheap model looks at what changed (email, calendar, feeds, failed routines) and suggests one or two things: "you pay this bill every month; want a routine for it?".
- Suggestions go to the inbox and the channel, never act by themselves, and learn from being ignored.
- Every suggestion stays inside the daily spending limit.

**Done when** in real-account use (1.3) at least a third of the suggestions are accepted, and nobody turns it off for being noisy.

---

## M4 · Hands (v0.9)

Goal: the open-ended tasks people give to OpenClaw and Hermes (a site with no API, a script, a file to transform) work in Pimpo, safely.

These three items give the agent the broadest new powers on this roadmap. Jev rates their safety risk the highest of all (see the appendix). Each starts with an RFC and a threat-model update, and ships behind a Labs flag first.

### 4.1 Agent-driven browser · L · RFC

- A browser the agent drives (open, read, click, type, choose, upload, download), isolated in its own profile, with the owner's logins only for the sites the owner allows, stored in the vault.
- Every browser action is a capability with a risk: reading is `read`; submitting a form, buying or sending is `irreversible` and asks first, with a screenshot of what will be sent.
- Page content is low-trust: nothing on a page becomes an instruction, and the protection list blocks exfiltration.
- Browser steps that worked are compiled into routines, with selectors and a fallback to re-exploration when a page changes.

**Done when** ten common tasks on sites without an API (booking, a utility's bill, a government portal, a shop order status) work in chat and as routines, with every submission approved.

### 4.2 Code sandbox · L · RFC

- Run shell commands and code (Python, JavaScript, shell) in an isolated sandbox: a container or a lightweight VM locally, over SSH on a host the owner adds, or in the Docker deployment (2.3).
- No secrets and no network by default; the network opens only to domains allowed per task; files come in and out only through declared folders.
- Anything that leaves the sandbox (a file written outside, a request to a new domain) passes the rules and approvals.
- Used by explorations, by skills (3.2) and inside routines, where a script step is recorded and tested like any other.

**Done when** data tasks (convert a spreadsheet, analyze a CSV, resize photos, run a script from a skill) work end to end, and a red-team pass cannot reach a secret, the host or an unallowed domain from inside the sandbox.

### 4.3 Learning the owner · M

- A model of the owner's preferences (tone, what matters, who is important, times) built from corrections, choices and ignored suggestions, kept in memory with provenance, readable and editable in **Memory**.
- It shapes answers, suggestions (3.3) and the judge's prompts; it never changes rules or capabilities.

**Done when** in blind comparisons the owner prefers answers with it over answers without it, and every learned preference can be traced to where it came from and removed.

---

## M5 · Everywhere (v1.0)

Goal: v1.0, with stable formats and Pimpo present wherever its owner is.

### 5.1 Stable formats for v1.0 · M

The event log, routine, connector and backup formats are frozen as [RELEASES.md](RELEASES.md) promises: newer versions read older data, and every upgrade snapshots first. An LTS line starts.

### 5.2 The phone as part of the agent · L

The phone app becomes a node: camera (a photo of a receipt or a document), location (routines that run when arriving somewhere), the phone's notifications and shortcuts, each a capability the owner grants on the phone, with the same rules and receipts.

**Done when** a routine can be triggered by arriving home and another by a photo of a bill.

### 5.3 Voice conversation · M

A wake word on desktop and phone (on-device), and continuous spoken conversation using the local voices and Whisper already built in, with barge-in. Approvals are never given by voice alone for irreversible actions.

### 5.4 Parallel agents and long tasks · L

A large job ("compare these 20 suppliers and prepare a proposal") is split across several agents that run in the background for hours, with a plan, progress in the app, a budget for the whole job, checkpoints that survive a restart, and a final report. Each agent has only the capabilities its part needs.

### 5.5 More channels · M each

- **iMessage** on the Mac.
- **Personal WhatsApp.** The unofficial protocol can get a number banned and breaks without warning. Offered only with a clear warning, off by default, and never as the only way to approve.

---

## M6 · Each person their own (v1.1)

Goal: several people share one Pimpo and nobody sees anyone else's things, not even the owner who runs it.

### 6.1 A login per person · M

Every device and session belongs to one person and every request acts only for them. The owner pairs a phone for someone (a QR code that signs in as them); removing a person revokes their devices.

### 6.2 Privacy between people · L

Every route has an access rule, and a route without one is the owner's. Routines, explorations, runs, conversations, memory, receipts, approvals, questions, recordings, jobs and the phone belong to their person; the owner administers the house (people, connections, models, rules, backups) but sees none of it. Members answer their own approvals; a lasting "always" is a house rule, so it stays the owner's. The live event stream gives others only an event's type.

**Done when** a test calls every route as a member with the owner's ids and as the owner with the member's, and nothing crosses.

### 6.3 Passkeys and sessions · M

A passkey to sign in on a computer, sessions that expire and can be revoked one by one, and a limit on sign-in attempts.

---

## M7 · At a glance (v1.2)

Goal: any number of dashboards per person, any routine as a widget, and widgets outside the app, each on its own: floating on the Mac, Windows and Linux desktop, and on the home and lock screens of Android and iOS. The design is [RFC 0003](rfcs/0003-dashboards.md).

- **7.1 Widgets and snapshots · M.** A `widget.show` capability (Notify) with fixed kinds (metric, list, status, text, table, chart), snapshots and history, built-in widgets, the dashboards API, private to each person.
- **7.2 Dashboards in the app · M.** Several per person, a grid to drag and resize, a phone layout, live updates, stale marks and refresh.
- **7.3 Routines become widgets · M.** A compiler rule for "show me…", **Turn into a widget** as a new approved version, and status widgets from any routine's runs.
- **7.4 Floating desktop widgets · M.** Each widget its own always-on-top window on macOS, Windows and Linux, hidden while the app is locked.
- **7.5 Android widgets · L.** A read-only widget key and feed, and Glance home-screen widgets.
- **7.6 iOS and macOS widgets · L · owner.** A WidgetKit extension with lock-screen redaction; needs the Apple Developer account.
- **7.7 Windows Widgets board · M.** Only if people ask for it after 7.4.

---

## Where the order differs from the scores

- **Docker (2.3)** scored lower than signing and the updater, mainly for build risk. It moves up to M2 anyway: Pimpo is a single Go binary, so the image is small work, and the server audience is a large part of the competitors' users.
- **Browser and sandbox (M4)** have high user value (the browser scores among the highest) but the highest safety risk of all items. They stay in M4, after the ecosystem, and are designed as RFCs early, so they do not ship as an afterthought.
- **Skills (3.2)** depend on the sandbox (4.2) for any script they carry. Skills that are only instructions ship in M3; skills with scripts wait for 4.2.
- **Subagents, voice, phone node and channels** scored lowest: little differentiation and high build or safety risk. They remain on the roadmap for v1.0 because they matter for completeness, not because they win users.

## How progress is tracked

Each item becomes an issue with the `roadmap` label and its milestone; [STATUS.md](STATUS.md) records what is done and what waits on people or time; the [CHANGELOG](../CHANGELOG.md) records what shipped. This roadmap is reviewed at the end of each milestone.

## Appendix: scores

Jev (`tools/jev/decisions_roadmap.py`, 2026-09-29), each from 0 to 3. **Total** is 2 × value + differentiation + 2 × adoption − build risk − safety risk.

| Item | Total | Value | Differentiation | Blocks adoption | Build risk | Safety risk |
|---|---|---|---|---|---|---|
| Real-account validation (1.3) | 7.89 | 2.83 | 1.10 | 2.16 | 1.93 | 1.26 |
| Known bugs (1.1) | 7.71 | 2.27 | 1.15 | 1.87 | 1.14 | 0.58 |
| Setup without Claude Code (1.2) | 7.42 | 1.87 | 1.54 | 2.35 | 1.46 | 1.10 |
| First release (1.4) | 7.05 | 1.82 | 0.72 | 2.27 | 1.12 | 0.73 |
| Official signing (2.2) | 5.10 | 1.03 | 1.90 | 1.44 | 1.11 | 0.63 |
| Auto-update (2.1) | 4.75 | 1.29 | 2.52 | 1.28 | 1.20 | 1.71 |
| Proactivity (3.3) | 4.70 | 2.11 | 1.01 | 1.32 | 1.72 | 1.45 |
| Website and packages (2.4, 2.5) | 4.41 | 0.76 | 1.10 | 1.74 | 1.11 | 0.58 |
| Gallery and protection list (3.1) | 4.38 | 1.58 | 1.56 | 1.18 | 1.38 | 1.32 |
| Learning the owner (4.3) | 3.83 | 1.78 | 1.32 | 1.00 | 1.68 | 1.37 |
| SKILL.md skills (3.2) | 3.50 | 1.59 | 1.67 | 1.25 | 1.92 | 1.93 |
| Docker and VPS (2.3) | 3.35 | 1.27 | 1.00 | 1.47 | 1.46 | 1.67 |
| Code sandbox (4.2) | 2.87 | 1.60 | 1.08 | 1.53 | 1.78 | 2.69 |
| Browser (4.1) | 2.68 | 1.91 | 0.91 | 1.40 | 1.99 | 2.86 |
| Phone as a node (5.2) | 1.32 | 1.16 | 0.92 | 0.97 | 1.80 | 2.06 |
| More channels (5.5) | 1.09 | 1.37 | 0.21 | 1.39 | 1.89 | 2.75 |
| Parallel agents (5.4) | 0.51 | 0.90 | 1.04 | 0.75 | 1.90 | 1.93 |
| Voice conversation (5.3) | 0.29 | 0.90 | 0.81 | 0.63 | 1.87 | 1.71 |
