# Changelog

Notable changes to Pimpo. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [semantic versioning](https://semver.org) as described in [docs/RELEASES.md](docs/RELEASES.md).

## Unreleased

No version has been tagged yet. The first release will include everything built so far: routines compiled from explorations, rules and approvals, receipts, the gallery, the Guard, the desktop, web and phone apps, the chat channels, and the changes below.

### Added

- Finance for companies: `costs.read` with the month by member and department and what looks wrong, a monthly report asked of the finance member, budgets it proposes and a person applies, a read-only Stripe connector (balance and charges) and `github.sponsors`.
- Earned autonomy: after a configurable run of approvals of one kind of delivery (10 by default), Pimpo suggests letting a company member do it alone; a person accepts, and a later no takes it back to approval.
- A product owner's signals, deduplicated, and briefs that cite and quote a source for every claim, scored by the roadmap's formula, flagged by Jev where the quote does not hold, decided on the new **Product** tab and checked against their predictions 30 and 90 days after shipping; standups for project managers and acceptance criteria on GitHub issues.
- Company members code: `code.workspace` runs the member's coding CLI (Claude Code, Codex or opencode), with its sandbox if chosen, in a git worktree and branch of its own per task, and Pimpo commits and pushes it with the member's GitHub token.
- GitHub capabilities for repositories, issues, pull requests, reviews, checks, merges and releases, for routines and company members alike.
- A company's memory per task, per agent and for the whole company: agents keep notes in each, freely, after a decider approves (a person by default for the whole company's) or not at all; Pimpo can keep a note of each piece of work by itself; notes waiting for approval reach you in **Needs**.
- **Meetings** with the CEO: talk with one agent, several or all of them, discuss a question before answering it, turn what they say into tasks, and end with minutes.
- Company members' own accounts (mail, GitHub, Slack, Notion and every other connection), the company's shared accounts with read or act grants, kinds of account with warnings about services' terms, and agents kept out of their person's accounts, devices and memory.
- A company's **Costs**: spending by member and department, a forecast, cost per outcome, salaries and budgets in layers, subscription use shown apart (and counted if you choose), and work that waits when a subscription runs out of turns.
- On a company's page: autonomy lines for members and roles, a **Decision levels** tab with suggested levels, a simulator and every decision taken.
- Decision levels for companies: triggers (amount, risk, kind, public, words) put each decision at a level that names who decides, the highest always the CEO, with a classifier for unclear questions and the bosses' opinions on the way up.
- Autonomy for company members: per member, role or company, who decides what an action asks first (the member, Jev, a model, the boss, a committee, a cascade or a person), with every decision kept.
- A company's **Memory** tab with its notes and meetings, and **This week** on the **Work** tab.
- A company's memory (decisions, lessons, minutes) that every member reads, meetings between agents with minutes and decisions, and a summary of the week for the CEO every Monday.
- A company's **Tasks** tab: the board by state, handing a member a task, each task's dossier, and members' questions answered in place and in **Needs you**.
- Company members work together: `company.assign` hands a task with its objective, how to know it is done and a dossier of where it came from; `company.report` reports back; `company.ask` stops the work until the boss answers (an agent with `company.answer`, or you from **Needs you** and your channels), and the work goes on from where it was; tasks that stray from their root ask the boss first.
- On a company's page: who is working now and what waits, a **Work** tab with stop, giving a member work or an agent routine, giving it your own routines, working hours, and pausing the company, a department or a member.
- Company members at work: live work from a queue with working hours, one piece at a time per member, agent routines on a schedule, compiled routines that wake a member's agent with `company.wake`, and pausing a member, a department or a company.
- **Context and rules** on a company's page, per company, department, role or member, with exceptions marked, and **What this agent receives** in a member's window.
- **Companies** in the app (Settings › Labs): create or import a company, its roles, departments and members, and see it as an org chart where dragging a card onto another changes its boss.
- Context and rules in layers for companies: Markdown contexts and rules for the company, a department, a role or a member; the most specific rule wins and allowing what a broader rule forbids must be saved as an exception; company rules never go past the house's; a member's preview shows what it is told and what each capability would do.
- Companies of agents, in Labs (RFC 0004): a company of any kind with departments, roles and members in a tree under its person, the CEO; shared with partners of the house by grant (view, approve, configure) and hidden from everyone else; exported and imported as a `company.yaml` that never names a person.
- Push triggers: routines that watch Gmail (through Google Pub/Sub) or Slack hear of new items as they happen, and a GitHub webhook with a signed secret starts a routine; polling stays as the fallback.
- Models and spending per person: the owner chooses which models each person and each assistant may use and a daily limit per person inside the house's (guests start at $0.25 a day); each person sees their own in **Account**.
- **For this routine** on approvals: a routine repeats exactly the approved action (same recipients, hosts, amounts up to the limit) without asking, until its code changes; each person revokes theirs in **Routines**.
- Settings history (**Settings › History**): every change to rules, budget, connections, models, people and settings, with who and when, secrets never kept, and undo; a member sees their own accounts' history in **Account**.
- Database integrity: Pimpo checks its database at start and before every snapshot; a damaged one is moved to a quarantine folder untouched, and a recovery page offers the newest snapshot that passes its check, a fresh start, or the damaged file as a zip.
- Private credential prompts: a routine or task that needs a missing or refused key sends its person a link to a one-time form that writes straight to the vault, and keys pasted into a chat or channel are removed before the model or the log sees them.
- Lasting progress: long jobs and routine runs show their parts or steps, the current step and the cost so far on Home, the routine and the job, the same after a reload or a restart; a followed job keeps one message up to date on Telegram, Discord and Slack.
- Memory shows where each fact came from (a conversation, a task, a routine, an email, an import, what you typed, a learned preference), with a link; **Memory › Sources** deletes everything one source gave, each person only their own.
- **Lessons**: what Pimpo noticed and would keep (a learned preference, a note it took, a task asked often enough to become a routine, a repair of a failing routine, a suggestion) in one list per person, each accepted, edited or rejected there; a rejected lesson never comes back, and a weekly notice links to the list.
- A live model catalog: each provider's model list comes from the provider (kept a day, refreshed on demand), new models are marked, retired ones are flagged with a suggestion to switch, and long Anthropic conversations are compacted on Anthropic's servers (on by default, cost counted).
- Outside password managers: any secret field can hold a reference to 1Password (`op://…`, with the op CLI or a Connect server) or HashiCorp Vault (`vault://…#field`), read when a connection needs it; the house's are set up in Connections and each person's own in Account.
- **Needs you** is one list of everything waiting for you (approvals with For this routine, keys asked for privately, questions with typed answers, stopped routines, jobs with problems, plans and tasks ready, lessons to review), the most urgent first, with filters by kind, answered in place on the page, in the bell and on Home.
- Dashboards in tabs with widgets of seven kinds (numbers, goals, status, lists, tables, and line, area, bar and donut charts), ready-made widgets, and sharing with the house.
- Floating widgets in the desktop app: any widget in a small always-on-top window that keeps its place and hides while the app is locked.
- A Telegram Mini App: **Dashboard** in the bot's menu opens what needs you, your routines, spending and your widgets inside Telegram, signed in as you (needs a public https address, such as Tailscale Funnel).
- Android home-screen widgets for any dashboard widget, read with a phone's own widget key that sees only the widgets pinned to it.
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

- A company task that may have strayed could start before its boss answered, and, once answered, could wait forever.
- The **liberal** safety level let irreversible actions through without asking (a GitHub comment, clearing a sheet, imported tools). It now asks before anything irreversible except deleting email, which moves it to the trash, and installs that chose it are upgraded.
- Repairs are told the error of the last failed run.
- Routines started only by a webhook no longer log a false "invalid schedule" failure.
- An answer to a question no longer runs a paused routine; **Run now** keeps a paused routine paused.
- The time a run waits for approval no longer counts toward its 15 minutes.
- Saving settings with only some fields keeps the others; the judge uses the configured Ollama address; the home network follows the port of `--addr`.
- opencode runs that start together no longer fail on a locked database.
- Windows builds (free disk space was read with a Unix-only call).
- A data race in how the language of messages was chosen.
