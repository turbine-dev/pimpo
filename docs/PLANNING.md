# Pimpo — Complete plan

> **Working name.** Premises: free and open source, local-first, no deadlines (each phase ends at a gate).
> Priorities and UI decisions were evaluated with **Jev** (TypeSafe System One). The raw data is in `jev/`, and the method is in Appendix A.
> Community validation: **on hold** by Dener's decision.

---

## 0. One-page summary

**What it is:** a free, local personal AI agent that does what OpenClaw and Hermes do (chats over Telegram, reads email and calendar, automates tasks), but was **designed from their failures**.

**The core idea — the agent that turns into software:**
1. The first time, the agent solves the task with an LLM and asks for approval for anything that cannot be undone.
2. When it works, it **compiles the task into a routine**: readable code, with tests and a list of what it may touch.
3. The routine runs on its own on schedule, **without an LLM**, for cents. A judgment model only steps in for specific decisions ("is this email important?").
4. If something changes, the routine stops, alerts you and proposes a fix that you approve.

**Why it beats both:** they send **every** request through the LLM, every time. That is where the unreliability (the #1 reason people quit), the cost (up to $131 a day) and the forgotten rules (the case of the 200 deleted emails) come from. Pimpo is also **secure by construction**: rules outside the model, minimal capabilities, approval on the phone, invisible keys, a receipt and undo for everything, and a real spending limit (neither of them has one).

**The interface** (validated with Jev): **chat to talk and approve, plus a local web interface to see and control**. The main screen is **Routines**. The moment that makes someone say "wow" is **seeing the task that just ran turn into a routine, with the cost dropping to zero**.

**The full path (section 8):** 13 phases, from the first commit to a mature project. First build the core (F0–F4: compiler, security, UI, three perfect uses). Then open up to the world (F5–F8: installation, migration, family, gallery, connectors). Finally expand and mature (F10–F12: native apps, protection network, longevity; F9, small business, was removed).

**First milestone:** the morning brief runs 30 days in a row as a compiled routine, with LLM cost close to zero and no silent failure.

---

## 1. The problem, with evidence

| Pain | Evidence | Structural cause in the competitors |
|---|---|---|
| Not reliable | #1 reason for quitting in the Hacker News threads about OpenClaw; "a cron job does the same" | Every request is re-decided by an LLM |
| Expensive and unpredictable | Heartbeat of $18.75 per night; reports of up to $131 per day (Notebookcheck, OpenClaw docs) | Whole context resent every turn; no spending limit (O#42475, H#6839) |
| Forgets rules and does damage | A Meta researcher had more than 200 emails deleted after context compaction removed "confirm first" (TechCrunch, February 23, 2026) | Rules live only in the model's context |
| Insecure | 135,000 exposed instances, CVEs rated 9.9; malicious skills on ClawHub; banned at Meta, Google and Amazon | Approvals off by default; skills with full power; keys readable by the agent (O#7916) |
| Cannot see or undo | OpenClaw's audit log does not store arguments; Hermes has no audit table; rollback requests unanswered (H#12238, O#153257) | Observability and reversibility are not part of the design |
| Hard to maintain | About a 25% chance that an update breaks delivery; more than 5,000 open issues in each project | Large stacks (Node, Python, Docker) and a fast pace |

**The space is open:** OpenClaw entered "stabilization mode" (#5799) and closes feature requests automatically. Hermes leaves governance stuck in `needs-decision`. Requests for per-tool approval, auditing and spending limits had their PRs closed without merge.

---

## 2. Who it is for

**Persona 1 — Marina, the burned enthusiast (first target)**
- Developer or technical professional. Installed OpenClaw or Hermes on a Mac mini or a VPS.
- Used it for a morning brief and email triage. Gave up because it failed without warning and the API bill was scary. Or has been afraid since reading about the deleted emails.
- Wants: that it works every day, to know how much it will cost, to see what the agent did and to undo it.
- Where she is: r/openclaw, Hacker News, the projects' Discords.

**Persona 2 — Carlos, the small-business owner (second target)**
- Not technical. Does everything from his phone.
- Wants: a quote from a photo, a PDF invoice, collecting from customers, without fear of the agent sending the wrong message.
- Needs: installation without a terminal (F5) and clear approvals.

### 2.1 User research, ongoing and private
Jev pointed to "talking to real users" as the biggest gap in every review (up to 0.97). **Public promotion is on hold**, but private research is not promotion. Proposal, subject to decision #5:

| Phase | Research | With whom | What it decides |
|---|---|---|---|
| F1 | 5 interviews showing the compilation moment (prototype or video) | People who used and dropped OpenClaw or Hermes | Whether the routines idea solves their pain; which three uses to prioritize |
| F2 | 3 sessions on approvals and rules | The same people | The wording of approvals and the annoyance limit |
| F3 | Usability test with a fixed script (the phase gate) | 5 new people | Whether the UI tells the story on its own |
| F4 | Usage diary for 2 weeks | 3 people using it for real | Real failures, trust, perceived cost |
| F5 | Closed beta of installation and migration | 10 people, technical and non-technical | Whether it is ready for v1.0 |

Each round ends with a short record (what was learned and what changes in the plan), kept alongside the code.

**Out of scope, for now:** companies with compliance requirements (Microsoft and NVIDIA are there) and developers who want a coding agent (Claude Code and Codex cover that).

---

## 3. Product principles

1. **One routine is worth more than a thousand conversations.** What repeats becomes software. The LLM is for what is new.
2. **Nothing irreversible without you.** And almost everything is reversible.
3. **The rules are yours, not the model's.** They live outside the context and the agent cannot edit them.
4. **Everything has a receipt.** If it happened, you can see what, why, how much it cost and how to undo it.
5. **Never fail silently.** Every failure becomes an alert on the phone.
6. **Predictable cost.** Hard budget, monthly projection, no surprises.
7. **Few things, done perfectly.** Three flawless uses before 150 integrations.
8. **Explain it as you would to a person.** No jargon in the interface. "Will delete 212 emails from the Work folder", not `gmail.messages.batchDelete`.

---

## 4. Prioritized features (evaluated with Jev)

Scale from 0 to 3 for user value, differentiation against OpenClaw and Hermes, technical risk and MVP need. Priority is `0.45×value + 0.35×differentiation − 0.2×risk`.

### 4.1 MVP core (need ≥ 2.2), split into two deliveries

The morning brief only **reads** data and **sends it to you**. It has no irreversible action. That is why the technical proof (F1) can be small, and the full security package comes right after (F2), before any case that writes or deletes.

**F1 — the proof (v0.1)**

| Feature | Value | Differ. | Risk | MVP | Version in F1 |
|---|---|---|---|---|---|
| Routine compiler | 2.97 | 2.63 | 1.73 | 2.98 | Complete; it is the proof |
| Scheduler | 2.94 | 1.31 | 1.05 | 2.99 | Fixed time (cron) |
| Morning brief | 2.76 | 1.35 | 1.53 | 2.99 | Calendar + important emails |
| Exploration mode | 2.19 | 1.19 | 1.62 | 2.88 | Only with read capabilities and sending to the owner |
| Telegram channel | 2.76 | 1.17 | 1.30 | 2.79 | Chat and alerts |
| Per-routine capabilities | 2.94 | 2.33 | 1.19 | 2.69 | Manifest checked at runtime |
| **Spending limit** | 2.95 | 2.88 | 1.01 | 2.58 | Simple daily limit, checked before each call (**highest overall priority: 2.13**) |
| Calibrated judgment steps | 2.56 | 2.55 | 1.59 | 2.35 | One judgment: "important email?" |
| Health watchdog | 2.70 | 2.29 | 1.17 | 2.26 | Every failure becomes an alert |
| UI: Routines (minimal) | 2.61 | 2.45 | 0.89 | 1.63 | Card with runs, cost and capabilities; chosen by Jev as the main screen |

**F2 — secure by construction (v0.2)**

| Feature | Value | Differ. | Risk | MVP | Why in F2 |
|---|---|---|---|---|---|
| Approval on the phone | 2.91 | 1.92 | 0.88 | 2.92 | Needed from the first case that writes or deletes |
| Rules engine outside the model | 2.95 | 2.74 | 1.63 | 2.74 | Same; solves the 200 emails case |
| Credential broker | 2.79 | 2.45 | 1.36 | 2.52 | In F1, the few credentials stay in the local vault |
| Self-repair by diff | 2.46 | 2.68 | 1.93 | 2.44 | High risk; in F1, a failure only alerts |
| Undo | 2.81 | 2.53 | 1.53 | 2.36 | Only makes sense with write actions |
| Audit with arguments | 2.80 | 2.64 | 1.35 | 2.32 | In F1, the event log already records; F2 adds chaining and verification |
| **UI: Receipts** | 2.90 | 2.57 | 1.03 | 2.30 | **Second highest overall priority (2.00)**; ships together with undo |

### 4.2 v1 (high value or differentiation, outside the MVP core)
- UI: full Routines (detail with code, tests and history).
- UI: Cost (1.82), UI: Rules (1.69), UI: Live exploration (1.62) and UI: Onboarding (1.51).
- Sandbox on by default (1.49), memory with provenance (1.69) and local judgment model (1.68).
- Email triage (1.49), approval anti-annoyance (1.52) and safe update (1.31).
- Migration from OpenClaw and Hermes (1.52): high differentiation (2.69), but not needed in the MVP (0.43).
- Versioned memory (1.53) and UI: Memory (1.39).

### 4.3 Later
- Small-business back office (1.26): good value, but requires F5 (installation without a terminal).
- WhatsApp, email and voice channels; family mode (0.90); native mobile app (0.68); desktop app (0.83).
- Guard plugin for OpenClaw and Hermes (1.08) and shared protection network (1.14).
- Using a Claude or ChatGPT subscription as the brain (0.75) and importing skills from the ecosystem (0.70).

**Two honest surprises from Jev:**
- **Using an existing subscription** had low value for our target (0.95), despite being the most upvoted Hermes issue. It makes sense: with compiled routines, the LLM cost already drops close to zero. It stays for later.
- **Any provider by API** had almost zero differentiation (0.30). It is necessary parity, not a selling point.

---

## 5. UX and UI

### 5.1 Decisions validated with Jev
- **Surfaces:** chat (Telegram) to talk, receive alerts and approve, plus an **installable local web interface (PWA)** to see and control. Confidence 1.00 against "chat only", "desktop app" and "mobile app". **Desktop app out of the MVP** (P=0.16).
- **Main screen:** **Routines** (0.53, against Rules 0.26 and Receipts 0.14).
- **"Wow" moments, in order:**
  1. **The compilation** (0.59): seeing the task that just ran turn into a readable, tested routine with a list of capabilities, and seeing the cost drop to about $0.
  2. **The block** (0.40): a dangerous action blocked and explained on the phone, with undo in one tap.

### 5.2 Interface principles
1. **Show what the agent can do as concrete objects**, not as conversation. A routine is a card with a name, schedule, health, cost and capabilities, not a prompt lost in the history.
2. **Every action has three layers of detail:** the human sentence ("Archived 14 newsletters"), the technical summary (tool and target) and the full arguments, on demand.
3. **Color means risk, not decoration.** Green for read, amber for reversible write, red for irreversible. The same language appears in Telegram (emoji) and on the web.
4. **Never an empty screen without a next step.** The empty state of Routines is "Ask for something you do every week".
5. **Cost always in view**, discreet, on every routine and every exploration.
6. **Motion with purpose:** the compilation is animated (the conversation condenses into a routine card). It is the only place with striking animation, because it is the product's moment.

### 5.3 Information architecture (web)

```text
┌ Sidebar ────────────────┐
│  Routines        (home) │  routine cards + "New task"
│  Needs you         (3)  │  pending approvals and alerts
│  Receipts               │  timeline of everything that happened
│  Rules                  │  rules in natural language
│  Cost                   │  spending, budget, projection
│  Memory                 │  what the agent knows, with provenance
│  Connections            │  Gmail, calendar, Telegram, models
│  Settings               │
└─────────────────────────┘
 Top: search (⌘K) · today's budget ($0.04 of $1.00) · health status
```

### 5.4 The screens

**Routines (home)**
- Grid of cards. Each card shows: name ("Morning brief"), next time ("tomorrow 07:00"), the last 14 runs as colored dots, the month's cost ("$0.03"), the capability icons (📧 read · 📅 read · ✈️ send) and the health state.
- A card in "exploration" (not yet compiled) appears dashed, with the "Turn into a routine" button.
- At the top: "This week your routines saved ~$4.20 in LLM" (the projection compared with running the same thing with an LLM every time).
- **Empty state:** "Ask for something you do every week", with three clickable suggestions (morning brief, email triage, bill reminder).

**Routine detail**
- Tabs: **Overview** (runs, cost, health), **Code** (readable, with syntax highlighting and the capability calls emphasized), **Tests** (recorded inputs and the result of each test), **Capabilities** (what it can and cannot do, with the reason), **History** (versions and diffs of each fix, with who approved) and **Runs** (each run with its receipt).
- Actions: run now, pause, edit schedule, undo the last version.

**New task and live exploration**
- A large box: "What do you want me to do?".
- During exploration, a **live timeline**: each step appears with the human sentence, the risk level (color) and the accumulated cost. Irreversible actions stop with an inline approval card.
- At the end, if it worked, the **compilation moment**:
  1. "That worked. Do you want me to do this on my own every day at 7 a.m.?"
  2. The timeline condenses into a routine card (animation).
  3. Shown side by side: "Today: $0.31 with LLM → Routine: ~$0.002 per day", the requested capabilities and the generated tests (all green).
  4. Buttons: "Turn on routine", "View code", "Adjust".

**Needs you**
- List of approval requests, alerts and questions from the agent, identical to what arrived in Telegram.
- Each request shows what will happen in human language, the source (which routine or conversation), the risk and the options: allow once, always, or deny.
- Anti-annoyance: similar requests grouped ("12 emails to archive — review all") and the suggestion "You approved this 5 times. Make it permanent?".

**Receipts**
- Timeline grouped by day and by routine or conversation. Each item: human sentence, risk icon, cost and an **Undo** button when reversible ("Undo until 07:12").
- Filters: irreversible only, blocked only, by routine, by connection.
- Clicking an item opens the full arguments and the rule that allowed or blocked it.
- Blocks are highlighted: "Blocked: delete 212 emails — rule 'never delete email without asking me'".

**Rules**
- List of rules in natural language ("Never delete email without asking me", "Do not spend more than $1 a day", "Only send messages to me").
- When writing a new rule: the compiled policy appears next to it, in readable form, to check. Then **"Test on the last week"**: "this rule would have blocked 3 actions", with the list.
- Rules created by the agent do not exist. Only the user creates and edits them.

**Cost**
- Bar for today's and the month's budget, spending by routine, by conversation and by model, and the projection of the monthly bill.
- "Biggest spenders": which exploration or fix cost the most.
- Alert when a routine starts costing more than usual (a sign that something changed).

**Memory**
- Readable files (what the agent knows about you, your preferences, contacts), each fact with its source ("you said on Sep 12", "email from a stranger — low confidence").
- Version history and restore.

**Connections**
- Connected accounts (Gmail, Google Calendar, Telegram), the configured LLM model and the judgment backend.
- Each connection shows which routines use it and with which capability.
- Keys never appear: only "connected on Sep 12 · used by 3 routines".

**Onboarding (first run)**
1. "Hi, I'm Pimpo." Three short screens explaining: routines, rules and receipts.
2. Connect Telegram (QR code or bot link).
3. Connect the model (API key, or a local model via Ollama).
4. "What can I do without asking you?": three simple options (conservative, balanced, liberal), which become editable rules.
5. Daily budget, with a suggested value.
6. The first task: "Do you want a summary of your calendar tomorrow at 7 a.m.?", which leads straight to the compilation moment.
- **Goal:** the first active routine in less than 5 minutes.

### 5.5 The design of Telegram messages
Telegram is half of the interface and follows the same system:
- **Approval:**
  ```text
  🔴 Irreversible action — Email triage
  Will delete 212 emails from the "Work" folder (2019–2024).
  Rule: "never delete email without asking me"
  [Allow once] [Move to trash] [Deny]
  ```
- **Daily receipt** (optional): "Today: 3 routines ran, 1 action blocked, $0.04 spent. See receipts →".
- **Failure:** "⚠️ The Morning brief did not run: Gmail refused access. I need you to reconnect. [Open Connections]".
- **Self-repair:** "🛠 The weather API changed. I prepared a fix (3 lines). [View diff] [Approve] [Ignore]".
- Always with buttons. Short text. A link to the detail on the web.

### 5.6 Visual language
- **Identity:** calm and trustworthy. It is a product about control, not about magic. Dark by default, with a light mode.
- **Semantic colors:** green (read, success), amber (reversible write, attention), red (irreversible, block, failure) and blue (exploration in progress). Neutral colors everywhere else.
- **Typography:** a readable sans for the interface and a mono for code and arguments.
- **Components:** routine card, timeline, approval card, capability badge, risk badge, diff viewer, budget bar, run sparkline.
- **Motion:** discreet everywhere. Striking only in the compilation (the timeline condensing into a card) and in the block (the card shakes and turns red).
- **Accessibility:** AA contrast, full keyboard navigation, color never the only signal (always icon and text) and `prefers-reduced-motion` respected.
- **Responsive:** the web works on the phone (PWA). On the phone, the sidebar becomes a bottom bar with Routines, Needs you and Receipts.

### 5.7 UX metrics
- Time to the first active routine: less than 5 minutes.
- Approval requests per day: fewer than 5 (above that, people turn it off).
- Acceptance rate of "Turn into a routine" after a successful exploration.
- Undo used on less than 2% of actions (if more, the agent is making too many mistakes).
- Usability tasks (find what the agent did yesterday, undo an action, create a rule) completed without help by 4 of 5 people tested.

---

## 6. Technical architecture

### 6.1 Overview

```text
 Telegram ◀────────────┐                     ┌──────▶ Web UI (PWA, React)
                       │                     │
 ┌─────────────────────┴─────────────────────┴───────────────────────────┐
 │ pimpo — a single Go binary                                              │
 │                                                                         │
 │  Channel gateway ──▶ Conversation ──▶ Exploration (tool loop) ─────────┐ │
 │                                          │                             │ │
 │                                          ▼                             │ │
 │                                  Routine compiler                      │ │
 │                    (code + tests + capability manifest)                │ │
 │                                          │                             │ │
 │  Scheduler ──▶ Routine runtime (JavaScript on goja, pure Go) ───────────┤ │
 │                                                                        ▼ │
 │                     Policy engine  ◀── user rules                        │
 │              (capabilities · rules · risk · budget · approval)           │
 │                                          │                               │
 │         ┌───────────────┬────────────────┼────────────────┐              │
 │         ▼               ▼                ▼                ▼              │
 │   Connectors       Credential         LLM layer        Judgment          │
 │ (Gmail, calendar,  broker            (providers,      (Jev · local model │
 │  weather, HTTP)    (local vault)      spend proxy)     · cheap LLM)      │
 │                                                                          │
 │  Storage: SQLite (append-only events, chained audit)                     │
 │  Memory: versioned Markdown files (embedded git, go-git)                 │
 │  Health watchdog · HTTP + WebSocket API for the UI                       │
 └──────────────────────────────────────────────────────────────────────────┘
```

### 6.2 Stack
- **Backend:** Go (one binary, cross-compiled for macOS, Linux and Raspberry Pi). SQLite via `modernc.org/sqlite` (no cgo).
- **Routine runtime:** `goja`, a JavaScript interpreter in pure Go (no cgo), chosen with Jev as the change that most raises the chance of closing F1 (0.66). The routine has no access to network, disk or processes: it only sees the injected capability functions. Time is limited by interruption and memory is limited per run. If stronger isolation is ever needed, QuickJS on WebAssembly (`wazero`) comes in as an alternative, with the same interface.
- **UI:** React, TypeScript and Vite, embedded in the binary. Accessible components (Radix primitives), styles with our own design tokens, animations with Motion. Real-time updates over WebSocket.
- **Channels:** Telegram Bot API (MVP). WhatsApp later.
- **LLM:** API providers (Anthropic, OpenAI, OpenRouter) and local models via Ollama. Every call goes through the internal spend proxy.
- **Judgment:** a single interface with three backends: **Jev** (calibrated probabilities, recommended when there is a key), **small local model** (free and offline; trained with MLX and served via Ollama or llama.cpp) and **cheap LLM** (fallback).
- **Memory:** Markdown files in an embedded git repository (`go-git`), for history and restore.

### 6.3 Data model (main entities)
- `event`: append-only log of everything (type, actor, data, hash of the previous one) → audit and receipts.
- `routine`: name, current version, code, tests, manifest, schedule, state.
- `routine_version`: code, tests, manifest, diff, reason and who approved.
- `run`: a routine or exploration run (start, end, result, cost, actions).
- `action`: each capability call (tool, arguments, risk, policy decision, reversible?, undone?).
- `rule`: user text, compiled policy, active?.
- `approval`: request, context, decision, channel and when.
- `connection`: type, scope and reference to the secret in the vault (never the value).
- `budget`: limits per day, month and routine.

### 6.4 Compiled routines in detail
**Format**
```js
// routine: morning-brief v3
export const manifest = {
  schedule: "0 7 * * *",
  capabilities: ["gmail.read:inbox", "calendar.read:primary", "http.get:api.open-meteo.com", "telegram.send:owner"],
  judgments: { important: "Is this email important for the owner today?" },
};

export default async function run({ gmail, calendar, http, telegram, judge }) {
  const events = await calendar.today();
  const mails = await gmail.unread({ since: "24h" });
  const important = [];
  for (const m of mails) {
    const j = await judge.important(m);          // calibrated probability
    if (j.p >= 0.7) important.push(m);
    else if (j.p >= 0.4) important.push({ ...m, unsure: true });
  }
  const weather = await http.getJSON("https://api.open-meteo.com/v1/forecast?...");
  await telegram.send(formatBrief(events, important, weather));
}
```

**Lifecycle**
1. **Exploration:** the LLM uses the same capability functions, and every call is recorded (input and output).
2. **Compilation:** the LLM receives the exploration record and generates code, manifest and tests. The tests replay the recorded inputs and check the outputs and the calls.
3. **Verification:** the code runs in the isolated runtime against the tests. The manifest is checked: no call outside the declared capabilities is possible.
4. **Review:** the user sees the card (capabilities, estimated cost, tests) and turns it on.
5. **Execution:** the scheduler runs it without an LLM. Every call goes through the policy engine.
6. **Breakage:** a capability error, an API format change or a judgment that is too uncertain (configurable threshold) stops the run, alerts and opens a repair exploration.
7. **Repair:** the LLM proposes a diff, and the tests (old and new) must pass. The user approves the diff and a new version is born.

**What does not become a routine:** one-off tasks or tasks that are too open-ended. They stay in exploration mode, equally protected by the policy engine.

### 6.4.1 Specification: when a task is compilable
The compiler only proposes a routine when **all** conditions hold. The conditions are checked by code, and the ones that depend on judgment go through the judgment backend:
1. **Repetition:** the user asked for recurrence ("every day", "whenever it arrives"), or the same intent showed up 2 or more times.
2. **Verified success:** the exploration ended without error, without a denied action and with the result approved by the user (a 👍 or "looks good").
3. **Stable structure:** the capability calls of the exploration form a sequence that can be written as code with loops and conditions. It does not depend on reading free text to decide **which tool** to call next.
4. **Isolable judgments:** every decision that requires interpretation fits in a closed question (yes or no, choice among options, a score), with a well-defined input (an email, an event). Open questions ("write a reply") are not judgments: they become an explicit, budgeted LLM call inside the routine, marked on the card as "uses LLM".
5. **Testable:** the recorded inputs make it possible to replay the run without the network.

**Compiler success measure:** the fraction of eligible explorations whose generated routine passes its own tests on the first attempt, and the fraction of routines that run 30 days without a fix. Both are measured in the routine benchmark (section 7).

### 6.7 Privacy and data
- **Everything stays local:** database, memory, receipts and secrets stay on the user's machine. There is no Pimpo server.
- **What leaves the machine:** only what goes to the chosen LLM provider and judgment backend. The UI shows, on each receipt, **what was sent to which provider**.
- **Minimization:** exploration sends the LLM only what is needed (for example, the subject and sender before the body). Compiled routines send nothing to the LLM, except in judgment steps or declared "uses LLM" calls.
- **Redaction:** secrets never enter prompts (the broker guarantees it). Sensitive patterns (cards, ID documents) are masked before sending, when configured.
- **No telemetry by default.** Error reports only with explicit consent and content that can be reviewed before sending.
- **Deleting means deleting:** one command removes all data from a connection (cached emails, derived memory, receipts with content), keeping only the skeleton of the audit.

### 6.5 Policy engine
- Every capability call, whether from exploration, a routine or a skill, goes through it **before** running. There is no alternative path, because the capability functions are the only door to the world.
- It evaluates in order:
  1. declared capability;
  2. user rules;
  3. budget;
  4. risk classification (read, reversible write, irreversible).
- The decision is **allow**, **make reversible** (for example, delete becomes trash), **ask for approval** or **block**.
- Rules in natural language are compiled by the LLM into a declarative policy (for example, CEL) and **checked by the user** on the Rules screen. Execution uses only the compiled, deterministic policy.
- Rules do not go through the model's context to be obeyed. A short reminder is included in the exploration prompt only so the model plans better.

### 6.6 Threat model (summary)

| Threat | Defense |
|---|---|
| Prompt injection in an email that is read | External content is marked as untrusted; minimal capabilities; irreversible actions always approved; the content does not become instruction memory |
| Malicious skill | Runs in the isolated runtime with minimal declared capabilities; no shell, no network outside the list |
| Key leak | The agent only sees aliases; the broker injects the value on the way out and the value never goes back to the model |
| The agent edits its own rules or manifest | Rules and manifests only change by user action in the UI or in Telegram, with a receipt |
| Runaway spending or loop | Budget checked before each call; step limits per exploration |
| Interface exposed to the internet | Listens only on localhost by default; remote access only through an authenticated tunnel; token required |
| Tampering with receipts | Events chained by hash; verification in the UI |

---

## 7. Engineering and quality
- **Rules:** everything in English in the code; all behavior tested; no excess comments; nothing that looks AI-generated.
- **Tests:**
  - unit tests in every Go package, with `-race`;
  - negative runtime tests (the routine tries to access something outside the manifest and fails);
  - connector contract tests with recorded responses;
  - UI tests with Vitest and Playwright on the main flows (first routine, approval, undo, rule);
  - end-to-end tests with a simulated LLM.
- **Mandatory regression scenarios:** the 200 emails incident (rule given, context compacted, destructive request) must be blocked; prompt injection trying to exfiltrate keys must fail; an API that changes format must become an alert and a fix, never silence.
- **Continuous measurement:** our own routine benchmark (success rate, cost per run, human interventions), run on every version.

---

## 8. Complete roadmap

From the first commit to a mature, sustainable project. No deadlines: the order matters, the duration does not. Each phase has **goal, deliverables (product, UI, technology), quality, gate, metrics and dependencies**. A phase only starts when the previous phase's gate has been reached. The exception is what is marked as parallel.

```text
            ┌──────────── Build the core ────────────────┐
F0  Foundation and compiler proof
F1  The compiled routine ───────────────────────────▶ v0.1  experimental
F2  Secure by construction ─────────────────────────▶ v0.2
F3  The UI that tells the story ────────────────────▶ v0.3
F4  Three perfect uses ─────────────────────────────▶ v0.4
            ┌──────────── Open up to the world ──────────┐
F5  Installation without a terminal + migration ────▶ v1.0  first stable version
F6  Memory with provenance + family + WhatsApp ─────▶ v1.1
F7  Gallery of routines and skills with capabilities ▶ v1.2
F8  More channels and connectors ───────────────────▶ v1.3
            ┌──────────── Expand and mature ─────────────┐
F9  (removed: small business is not the focus)
F10 Native apps (desktop and phone) ────────────────▶ v1.5
F11 Shared protection network + Guard ──────────────▶ v2.0
F12 Longevity: SDK, languages, audit, LTS ──────────▶ v2.x
```

---

### F0 · Foundation and compiler proof
- **Goal:** prove the core bet before building the product, and set up the base.
- **Product:** none yet. Only the proof.
- **Technology:**
  - **Compiler proof:** a script that receives the record of an exploration (recorded calls and responses) and generates routine, manifest and tests, run in a minimal goja runtime. Tested with **10 recorded tasks**: morning brief, bill reminder, simple triage, email alert from one person, weekly calendar summary, and variations.
  - New repository, Apache-2.0 license, CI (Go and UI), ENGINEERING.md.
  - Binary skeleton: HTTP and WebSocket server, SQLite with chained events, local secrets vault.
  - Minimal Telegram channel (send, receive, buttons).
- **UI:** design tokens, semantic colors and the sidebar layout. Navigable prototypes (low fidelity) of the Routines, Compilation moment and Receipts screens, to guide the rest.
- **Quality:** proof tests with the 10 tasks; negative test (the routine tries to leave the manifest and fails).
- **Gate:** at least 8 of the 10 tasks generate a routine that passes its own tests on the first attempt, without accessing anything outside the manifest. If it does not get there, the plan is revised **before** building the rest.
- **Metrics:** first-attempt compilation rate; cost of compiling each task.
- **Depends on:** nothing.

### F1 · The compiled routine → v0.1 (experimental)
- **Goal:** one real routine running every day, for real, on Dener's phone.
- **Product:**
  - exploration mode with read capabilities and sending to the owner;
  - compiler integrated into the product;
  - scheduler (fixed time);
  - one judgment step ("important email?");
  - health watchdog: every failure becomes an alert with the "redo with the agent" button;
  - simple daily spending limit.
- **UI:** Routines screen (card with runs, cost and capabilities) and a simple version of the compilation moment.
- **Technology:**
  - connectors for Google Calendar (read), Gmail (read, via IMAP with an app password) and Telegram (send);
  - goja runtime with time and memory limits;
  - pluggable judgment backend (Jev or cheap LLM).
- **Quality:** connector contract tests with recorded responses; a simulated API failure test that must produce an alert; routine benchmark (cost and failures) against the same task with an LLM on every run.
- **Gate:** 30 consecutive daily runs of the morning brief, of which at least 29 succeed; average LLM cost of at most $0.01 per run; and 100% of failures produced an alert in Telegram within 5 minutes (checked through the event log).
- **Metrics:** successful runs / total; cost per run; time between the failure and the alert.
- **Parallel:** 5 private conversations with users (pending decision #5).

### F2 · Secure by construction → v0.2
- **Goal:** be able to turn on tasks that write and delete without fear.
- **Product:**
  - full policy engine (capabilities, rules, budget, risk), with the decisions allow, make reversible, ask for approval and block;
  - rules in natural language, compiled and checked by the user;
  - approvals in Telegram (section 5.5);
  - credential broker and per-capability domains;
  - spending limit with monthly projection;
  - chained audit with arguments;
  - undo for email (trash, delayed send);
  - self-repair by diff, which must pass the old and new tests before going to approval.
- **UI:** Needs you, Receipts (with undo and highlighted blocks), and the basic version of Rules and Cost.
- **Technology:** policies in CEL; vault with local encryption (key derived from the operating system: Keychain on macOS, libsecret on Linux); delayed send with a persistent queue.
- **Quality:** mandatory regression scenarios (200 emails, prompt injection to exfiltrate keys, a loop against the budget); fuzzing of the policy evaluator; first voluntary external security review (a request to the security community once the hold ends).
- **Gate:** three automated tests pass in CI: (1) the 200 emails scenario ends with zero emails deleted, one approval request sent and undo restoring everything; (2) 50 different prompt injection attempts to exfiltrate keys result in zero keys in any output; (3) a deliberate loop ends with spending less than or equal to the budget.
- **Metrics:** irreversible actions without approval (zero); budget overrun (zero); false blocks per week.

### F3 · The UI that tells the story → v0.3
- **Goal:** someone who has never seen the product understands the difference in 5 minutes.
- **Product:** anti-annoyance (grouping, "make permanent" and risk classifier).
- **UI:**
  - full compilation moment (animation, cost comparison, tests and capabilities);
  - live exploration with inline approval;
  - Rules with "test on the last week";
  - full Cost;
  - routine detail with code, tests, history and diffs;
  - onboarding with the first routine in less than 5 minutes;
  - full design system (components documented in a visual catalog);
  - AA accessibility, mobile layout and installable PWA.
- **Technology:** Playwright on the main flows; visual regression tests of the components.
- **Quality:** usability test with 5 people (can be done in private, without promotion).
- **Gate:** with a fixed script and recorded sessions, 4 of 5 participants who have never seen the product complete the four tasks without any moderator intervention, each within 10 timed minutes: create the first routine, find what the agent did yesterday, undo an action and create a rule.
- **Metrics:** time to the first routine; approval requests per day (fewer than 5); acceptance rate of "Turn into a routine".

### F4 · Three perfect uses → v0.4
- **Goal:** Pimpo truly replaces what Dener used before.
- **Product:**
  - email triage (drafts, archive, unsubscribe; always reversible);
  - a third use chosen by Dener (bill reminder, weekly summary or package tracking);
  - versioned memory;
  - safe update with snapshot and automatic rollback.
- **UI:** Memory screen; "what changed since yesterday" panel on the Routines screen.
- **Technology:** local judgment model (trained with MLX, served via Ollama or llama.cpp) evaluated against a labeled set; memory in embedded git (`go-git`).
- **Quality:** labeled set of judgments with at least 300 examples per judgment; calibration measured (the stated confidence matches the accuracy).
- **Gate:** for 30 consecutive days, the three routines (morning brief, email triage and the third use defined at the start of the phase) run at every scheduled time; the event log shows at least 97% successful runs, less than 2% undone actions, and zero failures without an alert.
- **Metrics:** undo used on less than 2% of actions; accuracy and calibration of the local and Jev judgments.

### F5 · Installation without a terminal and migration → v1.0 (first stable version)
- **Goal:** anyone can install it, and people who use OpenClaw or Hermes migrate without pain.
- **Product:**
  - installer for macOS (menu bar app that embeds the binary) and a one-line script for Linux, Raspberry Pi and VPS;
  - `pimpo migrate openclaw|hermes`: skills, memory, channels and schedules, with the report "what each skill could do and what it can do now";
  - imported skills run with minimal capabilities and, when they repeat, become routines;
  - guided Google OAuth in the UI (the user's own client), replacing IMAP as the default.
- **UI:** installation wizard; migration report; "What Pimpo guarantees and what it does not" page.
- **Technology:** signed binaries (notarization on macOS); stable and beta update channels; event and routine schema frozen at v1, with tested migrations.
- **Quality:** test matrix on macOS, Ubuntu and Raspberry Pi OS; migration tests with real anonymized OpenClaw and Hermes installations.
- **Gate:** 3 of 3 non-technical participants finish the installation within 15 minutes without editing files, with the default rules active; 3 real installations (OpenClaw or Hermes) migrate with one command and at least 80% of the skills work without adjustment. *(Needs the hold to end.)*
- **Metrics:** installation time; migration time; imported skills that work without adjustment.

### F6 · Memory with provenance, family and WhatsApp → v1.1
- **Goal:** the agent cannot be poisoned by what it reads, and it serves a whole household.
- **Product:**
  - provenance on all memory, with low-confidence memory isolated from instructions;
  - multiple users with roles (owner, member, guest), credentials and memory per person;
  - per-person rules ("the kids cannot buy anything");
  - WhatsApp as the second channel.
- **UI:** memory shows the source and confidence of each fact; People screen; approval routed to whoever is responsible.
- **Technology:** WhatsApp via the official API (Cloud API) or a local bridge, to be decided by cost and by the risk of being blocked; per-user data isolation in the database.
- **Quality:** memory poisoning attack suite (malicious emails and pages); isolation tests between users.
- **Gate:** a suite of 50 memory poisoning attacks (malicious emails and pages) ends with 100% blocked; the isolation tests between users pass 100% (no access to another user's data or credentials).
- **Metrics:** suite attacks blocked (100%); requests routed to the right person.

### F7 · Gallery of routines and skills with capabilities → v1.2
- **Goal:** people share ready-made routines safely, the opposite of ClawHub.
- **Product:**
  - routine gallery: readable code, capability manifest, tests and history;
  - installing a routine shows exactly what it can touch;
  - author signatures and reproducibility (the same code produces the same hash);
  - reports, with removal after review;
  - ecosystem skills (SKILL.md) adapted to run with minimal capabilities.
- **UI:** gallery browser with filters by capability ("show only routines that send nothing outside"), a page for each routine and an install button with the capability review.
- **Technology:** static index in a public git repository (no server of our own); signing with Sigstore; verification on the client.
- **Quality:** automatic analysis of submitted routines (declared versus used capabilities); human review for sensitive capabilities.
- **Gate:** 50 routines published by the community; the verifier (which runs each routine's tests in an instrumented runtime and compares the capabilities called with the ones declared in the manifest) reports zero mismatches in 100% of them; and every confirmed report led to removal within 72 hours, measured by the history of the index repository. *(Needs a community.)*
- **Metrics:** routines installed; confirmed reports; review time.

### F8 · More channels and connectors → v1.3
- **Goal:** cover the places where people already live, without losing quality.
- **Product and technology:**
  - channels: email (talking to the agent by email), voice notes (input and output), personal Discord and Slack, and PWA push notifications;
  - connectors: see the catalog in section 10;
  - **Connector SDK:** a connector is a package that declares the capabilities it offers, with the risk level of each one and mandatory contract tests.
- **UI:** Connections screen with the catalog, state and capabilities of each connector.
- **Quality:** a connector only gets in with contract tests, capabilities classified by risk and an example routine.
- **Gate:** 15 connectors with green contract tests against the real API for 4 consecutive weeks.
- **Metrics:** active connectors per user; connector failures per week.

### F9 · (removed)
- **Dener's decision on September 25, 2026:** the small-business back office (customers, quotes, invoices, payment links) left the roadmap. It is another product, a CRM, and it mixed concepts: Pimpo is an agent that turns requests into reliable routines, not a management system.
- Small-business owners use Pimpo like anyone else: by asking for routines ("every 5th of the month, tell me who hasn't paid") over email, the calendar or a spreadsheet.
- Only the generic parts of the work done remain: reading photos (Tesseract) and audio (whisper.cpp) on any channel.

### F10 · Native apps → v1.5
- **Goal:** approval and control in one tap, even without Telegram.
- **Product and UI:**
  - desktop app (Tauri) with a menu bar icon, notifications and auto-start;
  - companion phone app (iOS and Android) focused on three things: approvals, receipts and health;
  - secure pairing with the Pimpo at home (QR code), with no intermediary server.
- **Technology:** end-to-end encrypted communication through a tunnel (for example, Tailscale or an optional blind relay); push notifications.
- **Quality:** pairing and revocation tests; audit of the apps' permissions.
- **Gate:** across 20 measured approvals, the median time from alert to tap is under 3 seconds; pairing and revocation pass the automated tests on iOS, Android, macOS, Windows and Linux.
- **Metrics:** approval time; app usage versus Telegram.
- **Note:** Jev rated native apps as low priority for the initial audience (0.68 and 0.83). That is why they come late, when there are already users asking for them.

### F11 · Shared protection network and Guard → v2.0
- **Goal:** each user protects the others, and people who stayed on OpenClaw or Hermes benefit too.
- **Product:**
  - optional network: signatures of malicious skills, exfiltration domains and dangerous action patterns, shared anonymously;
  - **Guard** plugin for OpenClaw and Hermes, using the official hooks (`before_tool_call`, `pre_tool_call`) with the same policy engine.
- **UI:** "Community protection" in Receipts ("blocked: skill flagged by 12 users").
- **Technology:** signed list distributed as a static file; contribution with differential privacy; no personal content.
- **Quality:** process for contesting false positives; Guard contract tests against each supported version of OpenClaw and Hermes.
- **Gate:** at least 100 installations that sent or downloaded the signed list in the last 30 days (counted by the public index, without identifying anyone); at least 1 block recorded in a receipt whose rule came from the shared list; less than 5% of the list's entries contested and removed as false positives; and Guard passes the contract tests of the two most recent versions of OpenClaw and Hermes. *(Needs a community.)*
- **Metrics:** threats blocked by the network; contested false positives.

### F12 · Longevity → v2.x
- **Goal:** the project survives and improves without depending on a single person.
- **Deliverables:**
  - public, stable SDK for connectors, judgments and channels;
  - languages: Portuguese and English since F3; Spanish and others through the community;
  - independent security audit, with a published report;
  - long-term support (LTS) versions with security fixes;
  - open governance: maintainers beyond the founder, an RFC process and a code of conduct;
  - complete documentation: user, connector developer and threat model.
- **Gate:** at least 3 maintainers beyond the founder, each with 10 or more PRs reviewed per month for 3 months; one independent audit published; and security fixes released within 7 days of the report on the LTS version.
- **Metrics:** active contributors; response time to security reports; LTS versions maintained.

---

## 9. Complete feature map

All 46 features evaluated by Jev, with the phase in which they come in. The numbers are the calculated priority (`0.45×value + 0.35×differentiation − 0.2×risk`).

| Phase | Features |
|---|---|
| F0 | Compiler proof · binary base · minimal Telegram · design tokens |
| F1 | Routine compiler (1.91) · Scheduler (1.57) · Morning brief (1.41) · Exploration mode (1.08) · Telegram channel (1.39) · Capabilities (1.90) · Daily spending limit (2.13) · Calibrated judgments (1.73) · Health watchdog (1.78) · Minimal Routines UI (1.85) · Any provider by API (0.42) · Single binary (1.40) |
| F2 | Rules engine (1.96) · Approval on the phone (1.81) · Credential broker (1.84) · Audit with arguments (1.91) · Undo (1.84) · Self-repair (1.66) · Sandbox by default (1.49) · Receipts UI (2.00) · Needs you UI (1.41) · Basic Rules and Cost UI |
| F3 | Full Cost UI (1.82) · Rules UI with testing (1.69) · Live exploration UI (1.62) · Onboarding UI (1.51) · Anti-annoyance (1.52) · PWA UI (0.85) |
| F4 | Email triage (1.49) · Versioned memory (1.53) · Memory UI (1.39) · Local judgment model (1.68) · Safe update (1.31) |
| F5 | Migration (1.52) · Import skills from the ecosystem (0.70) |
| F6 | Memory with provenance (1.69) · Family mode (0.90) · WhatsApp channel (0.81) |
| F7 | Routine gallery (new, derived from F5) |
| F8 | Email channel (0.78) · Voice (0.95) · Claude or ChatGPT subscription as the brain (0.75) · Connector SDK |
| F9 | ~~Small-business back office (1.26)~~ removed by Dener's decision |
| F10 | Desktop app (0.83) · Phone app (0.68) |
| F11 | Shared protection network (1.14) · Guard plugin (1.08) |

---

## 10. Connector catalog by phase

Each connector declares capabilities with a risk level: 🟢 read, 🟡 reversible write, 🔴 irreversible.

| Phase | Connector | Capabilities |
|---|---|---|
| F1 | Google Calendar | 🟢 read events |
| F1 | Gmail (IMAP) | 🟢 read messages |
| F1 | Telegram | 🟡 send to the owner |
| F1 | HTTP (domain list) | 🟢 GET on declared domains |
| F2 | Gmail | 🟡 archive, label, draft · 🔴 send, delete (made reversible: delayed send, trash) |
| F4 | Google Calendar | 🟡 create event (reversible) |
| F4 | Local files | 🟢 read declared folders · 🟡 write with snapshot |
| F5 | Gmail (OAuth), Outlook and generic IMAP | read and write as above |
| F6 | WhatsApp | 🟡 send to the owner · 🔴 send to third parties (always approved) |
| F8 | Notion, Obsidian | 🟢 read · 🟡 write with history |
| F8 | Todoist, Google Tasks, Apple Reminders | 🟢 read · 🟡 create and complete |
| F8 | Home Assistant | 🟢 read states · 🟡 reversible actions · 🔴 critical physical actions (locks, alarm) always approved |
| F8 | GitHub | 🟢 read issues and PRs · 🟡 comment |
| F8 | RSS and web pages | 🟢 read |
| F8 | Weather, package tracking, exchange rates | 🟢 read |
| F8 | Spreadsheets (Google Sheets, CSV) | 🟢 read · 🟡 write with history |
| F8 | Local OCR for photos | 🟢 local |

**Permanent rule:** no bank or brokerage connector with the capability to move money. Read only, and only after F12, with an audit.

---

## 11. Project operations

**Versions and releases**
- Semantic versioning. Schemas (events, routines, manifests, policies) frozen at v1 and only grow.
- Stable and beta channels. Release notes in human language ("what changes for you").
- Signed and reproducible binaries; notarization on macOS.

**Updates**
- Automatic snapshot before each update and automatic rollback if the service does not respond well (F4).
- Updates are never automatic without consent; an alert through Telegram with a summary.

**Support and reports**
- Bug reports from the UI itself, which builds an anonymized, reviewable package (no personal content) before sending.
- Issue templates on GitHub; weekly triage; Jev can help classify issues (duplicate, bug, request), always with human review.
- Discussions and questions in a public forum (GitHub Discussions), not in ephemeral chat.

**Security**
- SECURITY.md with a private reporting channel and a public response time.
- Security fixes published with an advisory and an LTS version (F12).
- Security regression scenarios in CI (section 7).

**Documentation**
- User guide (in Portuguese and English), connector reference, SDK guide and threat model.
- An honest "What Pimpo guarantees and what it does not" page.

**Terms of use and legal risks**
- **WhatsApp:** use only the official API (Cloud API). Unofficial bridges risk banning the user's number and stay out of the project.
- **Gmail and Google:** respect the Google API user data policy; the OAuth client belongs to the user, so the data never passes through a project app. Google verification only comes in if there is ever a shared app.
- **Telegram:** bots follow the platform's terms; the bot belongs to the user (created with BotFather during onboarding).
- **LLM providers:** the user uses their own keys and accepts the provider's terms. Pimpo shows, on each receipt, what was sent to which provider.
- **Liability:** Apache-2.0 license with no warranty, and the "What Pimpo guarantees and what it does not" page makes it clear that irreversible decisions are always the user's.
- **Gallery and protection network (F7 and F11):** content policy, removal process and appeals published before opening.

**Languages**
- Interface and Telegram messages in Portuguese and English since F3; structure ready for the community to translate.

---

## 12. Community and sustainability (all free)

- **License:** Apache-2.0 for all the code. No paid version, no locked features.
- **Zero cost to run the project:** no servers of our own. The gallery and the protection network are static files in public repositories, and the user pays only for their own LLM (or uses a local model).
- **Voluntary support:** GitHub Sponsors and Open Collective, with transparent spending (for example, the security audit). No investors demanding monetization.
- **Governance:** the founder as lead maintainer until F11; in F12, additional maintainers, an RFC process and open decisions.
- **Reaching the public (when the hold ends):** an honest technical report (the 200 emails incident reproduced and blocked; routine cost versus an LLM every time), a video demo of the compilation moment, and posts where the personas are (Hacker News, r/openclaw, Brazilian tech communities).

---

## 13. Metrics

| Metric | Target |
|---|---|
| LLM cost per routine run | Close to zero; compared with the same task with an LLM on every run |
| Runs with silent failure | Zero |
| Successful runs over 30 days | More than 99% |
| Irreversible actions without approval | Zero |
| Approval requests per day | Fewer than 5 |
| Deviation between spending and budget | Zero or negative |
| Successful explorations that become routines | More than 50% of repeated tasks |
| Time to the first routine | Less than 5 minutes |
| Time to migrate from OpenClaw or Hermes | Less than 5 minutes |

---

## 14. Risks and responses

| Risk | Response |
|---|---|
| **Compiling tasks into reliable routines does not work well** (the core bet) | That is F1, with a single case and a 30-day gate. If it fails, the product is still worth it as a "secure by construction agent", but it loses its big differentiator, and the plan is reassessed before moving on |
| Google connectors require app verification for sensitive Gmail scopes | Each user creates their own OAuth client (guided in the UI) or uses IMAP with an app password; consider verification only when there are users |
| Few tasks repeat | One-off tasks stay in exploration, which is already protected. The initial focus is precisely the repetitive uses (brief, triage) |
| External APIs change | The health watchdog detects it and self-repair proposes a diff; never silence |
| Too many approvals become annoying | Anti-annoyance and a risk classifier; a target of fewer than 5 requests per day |
| The competitors' huge communities | Do not compete on quantity: win over those who got burned, with migration in 1 command |
| They copy the idea | It would require rewriting their core (everything goes through the LLM) and building capability-based security; even so, speed and focus |
| Dependence on Jev for judgments | Pluggable backend; the local model and the cheap LLM make sure the product works without it |
| False sense of security | Document honestly what each layer guarantees and what it does not |

---

## 15. Pending decisions (Dener's)

1. **Final name** (check trademark and domain).
2. **Default judgment backend:** Jev (better calibration, requires a key) or a local model (free, offline, less accurate). Recommendation: **Jev when there is a key, local model as the free default** from F4.
3. **Routine language:** JavaScript on goja (LLMs write JS well; pure Go) or Starlark. Recommendation: **JavaScript on goja**, the change Jev most recommended for the MVP.
4. **Compiled policy language:** CEL (Google's standard, simple and safe) or our own DSL. Recommendation: **CEL**.
5. **Private conversations with users during F1:** Jev pointed to "talking to real users" as the piece most missing from the plan (0.84). This is different from public promotion, which is on hold: it would be 5 private conversations with people who used OpenClaw or Hermes, showing the compilation moment. Recommendation: **do it**, without a public announcement.
6. **Gmail access:** IMAP with an app password (simple, works today) or OAuth with each user's own client (more secure, more tedious setup). Recommendation: **IMAP in F1 and guided OAuth in F3**.
7. **When to come off the hold** on community promotion (F5 and F7 depend on it).

---

## 16. Long-term vision

**Where Pimpo wants to get to:** to be the standard, trustworthy way to have a personal agent. Not because of the number of integrations, but because it is the only one people **trust to leave running on its own**.

- **From the agent that talks to the agent that turns into software:** over time, most of what the agent does for a person is compiled, readable and cheap routines. The LLM stays for what is new.
- **An open library of auditable routines:** each routine with code, tests and declared capabilities, built by the community (F7). It is the opposite of a marketplace of skills that run with full power.
- **Protection that grows with use:** the shared network (F11) is the asset no agent builds on its own.
- **Small, local judgment models:** more and more decisions run for free, offline, on the person's computer, with their own data.
- **What Pimpo will never do:** sell data, charge for security, move money on its own, or hide what the agent did.

---

## Appendix A — How Jev was used

- **Prioritization:** 46 candidate features, each evaluated on four questions on a 0–3 scale (value for the target user, differentiation against OpenClaw and Hermes, risk for a solo founder, need for the MVP), with the product, the audience and the competitors as context. Script: `jev/prioritize.py`; results in `jev/features.json`.
- **UI decisions:** four choice and yes/no questions (main surface, main screen, "wow" moment, need for a desktop app). Results in `jev/ui.json`.
- **Plan review:** each section evaluated for concreteness, open decisions and consistency with the MVP (`jev/review.py`, `jev/review.json`); then each roadmap phase evaluated for concreteness, verifiable gate and position in the sequence, and the whole plan for completeness (`jev/review_full.py`, `jev/review_full.json`). Summary in Appendix B.
- **Limits:** Jev gives calibrated judgments about the text it receives. It does not replace real users. Validation with people is still the next step when the hold ends.

## Appendix B — Result of Jev's review

Three rounds of review, with fixes between them.

| Question about the whole plan | 1st round | Final | Reading |
|---|---|---|---|
| Would the result be better than OpenClaw and Hermes for the target audience? (0–3) | 2.75 | 2.75 | "Clearly better on the pains that make people give up" |
| Is the order of the phases correct? | 0.88 | 0.88 | Yes |
| Does the UI make the two "wow" moments visible? | 0.88 | 0.89 | Yes |
| Do F0 and F1 fit one person with coding agents? | 0.50 | 0.52 | Uncertain: Jev has no way to know, and the uncertainty is concentrated in the compiler (0.81 as the biggest threat) |
| Biggest risk | compiler (1.00) | compiler (1.00) | That is why F0 starts with the compiler proof and has a gate that stops everything if it fails |
| Most missing piece | talking to users (0.84) | talking to users (0.82) | Became pending decision #5 (private conversations, no promotion) |

**Actions taken from the review:**
1. F0 now starts with the **compiler proof** (10 recorded tasks, 8/10 gate).
2. The **specification of what is compilable** was created (6.4.1).
3. F1 was **trimmed down**: no automatic self-repair (moves to F2), minimal connectors, minimal UI.
4. The MVP table was **split into F1 and F2**, because the morning brief has no irreversible action.
5. The routine runtime became **goja**, the change with the most impact according to Jev (0.66 against 0.13 for the second option).
6. The **privacy and data** section was created (6.7).
7. **Private conversations with users** came in as a pending decision.

**About the uncertainty on scope:** a probability near 0.5 means Jev sees similar arguments on both sides, not that it thinks it does not fit. The right answer is not more text in the plan, it is **running the F0 compiler proof**, which settles the question with data.

### Review of the complete plan (roadmap F0–F12)

| Question | Before the fixes | After |
|---|---|---|
| Is it a complete plan, from the first commit to a mature product? | 0.96 | 0.96 |
| Are the post-v1.0 phases as detailed as the early ones? (0–3) | 2.33 | 2.57 |
| Is the UI covered across the whole lifecycle? | 0.80 | 0.73 |
| Biggest gap | talking to users (0.97) | talking to users (0.94) |

**Verifiable gates per phase** (probability that the gate can be checked without subjective judgment):

| Phase | Before | After |
|---|---|---|
| F1 | 0.47 | 0.80 |
| F2 | 0.56 | 0.80 |
| F3 | 0.45 | 0.63 |
| F4 | 0.24 | 0.61 |
| F5 | 0.51 | 0.58 |
| F6 | 0.54 | 0.78 |
| F7 | 0.36 | 0.49 |
| F9 | 0.48 | 0.67 |
| F11 | 0.48 | 0.53 |

**What was done:** every gate now has numbers and a verification source (event log, CI tests, recorded sessions, repository history); the ongoing private research plan came in (2.1); the terms of use and legal risks section came in (11).

**What remains open, honestly:**
- **Talking to users** is still the biggest gap. The research plan exists, but it depends on decision #5, and Jev only sees the text, not the execution.
- **The F7 and F11 gates** depend on a community, and so are never fully under the project's control.
- **F11 comes late in the sequence** (0.70 in the position evaluation). Guard could come earlier, as an entry point for OpenClaw and Hermes users, but the prioritization gave it low value for the initial audience (1.08). It stays as a decision to revisit after v1.0.


## Appendix C — Technical decisions for F6 to F12 (Jev)

Made with `tools/jev/decisions_later.py` (result in `decisions_later.json`), before each phase:

| Decision | Choice | Probability | Closest alternative |
|---|---|---|---|
| WhatsApp | Official API (Cloud API) | 0.56 | local bridge (0.26), with the risk of the number being blocked |
| Gallery signing | authors' Ed25519 keys | 0.74 | Sigstore (0.25) |
| Voice | local whisper.cpp | 0.51 | provider API (0.26) |
| PDF | pure Go library | 0.84 | Typst (0.09) |
| Photos | local Tesseract | 0.51 | Tesseract with fallback to the model (0.35) |
| Phone alerts | Telegram as the push channel | 0.53 | ntfy (0.44) |
| Connector SDK | MCP processes over stdio | 0.47 | compiled Go packages (0.43) |
| Protection network | signed static file in a repository | 0.95 | own API (0.05) |

Two changes from the original plan, both because of the decisions above: the gallery uses Ed25519 keys instead of Sigstore, and the phone app uses Telegram (and the generic channel) for instant alerts instead of its own push.

The local judgment model was measured before becoming the default: on its own it is 85% accurate; in a cascade with Jev, sending only the 22% of uncertain answers, it is 92% accurate (Jev alone: 94.5%). That is why the local backend runs in a cascade.
