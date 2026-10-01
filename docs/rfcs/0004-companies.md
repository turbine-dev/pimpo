# RFC 0004: Companies of agents

- Author: Pimpo maintainers
- Status: accepted
- Discussion: the pull request that adds this file

## Problem

People already use Pimpo's assistants and jobs as a small team: one assistant reads the news, another drafts posts, a job splits a big task into parts. What they cannot do is run a team that keeps working: several agents with their own job, their own accounts, their own routines and their own budget, who hand work to each other, ask the one above them before deciding, and leave the person in charge only what is theirs to decide.

People want to:

- create a company of any kind (a software project, a shop, an agency, a channel, a consultancy) with roles, functions, context and rules of their own;
- hire agents into roles, each with a boss, a model, its own accounts and any number of routines;
- choose, per agent, who decides what: Jev, another model, the boss, or a person, and say which decisions always go to the CEO, who is a person;
- see the company: the org chart, who is doing what, who is waiting for whom, what it costs.

## Where things stand

- **Assistants** (`internal/app/assistants.go`) are named roles with instructions and a capability list that the host enforces. They belong to their person and have no boss, budget, accounts or routines.
- **Jobs** (`internal/app/jobs.go`) plan a request into up to eight parts, run three at a time as explorations limited to their capabilities, save after every step, resume after a restart, stop at a budget and write a report.
- **Routines** run without a model on a schedule, a watch, a webhook, a push trigger or an answer, and belong to a person (`store.Routine.Person`).
- **The judge** (`internal/judge`) answers yes/no questions with a probability, through Jev, a model or a local model, with a calibrated cascade.
- **Approvals and questions** pause a run without the clock running (`internal/pause`), reach the person on every surface, and can be bound to the operation (8.3). **Needs you** gathers them (9.8).
- **Rules** decide before every capability call; risks are Read, Notify, Reversible and Irreversible.
- **Accounts** are per person (`internal/app/catalog.go`): each person connects their own mail, calendar and services, and the router resolves a call to the caller's account. Secrets live in the vault.
- **Spending** is recorded per model and job, and limited per person inside the house's limit (9.5).
- Everything personal belongs to its person, and the administrator sees none of it (M6).

## Proposal

### 1. What a company is

A company belongs to the person who created it. Nothing in the engine knows about any industry: software, marketing or finance are roles, context, rules and routines that come from a template or from the person.

| Concept | What it holds |
|---|---|
| **Company** | name, industry (free text), mission, time zone, working hours, budget, default decision policy, context, rules, shared accounts |
| **Department** | optional grouping with a name, a color, a budget, context and rules |
| **Role** | a reusable function: title, what it is for, responsibilities, deliverables, context, rules, and suggested capabilities, models, autonomy, routines and account kinds. Several members may hold one role |
| **Member** | an agent holding a role (name, avatar, persona, boss, overrides, accounts, routines, memory, budget, hours), or a person holding a seat (the CEO, a partner) |
| **Context** | what an agent needs to know to work well, in Markdown with attachments and versions, at four levels: company, department, role, member |
| **Rules** | Pimpo's rules with a company, department, role or member scope |

Members form a tree through **reports to**. The top is a person, the **CEO**. A company may add partners, people of the house invited with a role: view, approve or configure.

### 2. Context and rules in layers

- When an agent works, it receives the company's context, then its department's, its role's and its own, plus the parts of the company's memory that fit the task.
- Rules are evaluated by the host, as today. **The most specific rule wins, and it may restrict or allow**: the company says "never give a discount", the role *Sales manager* allows "up to 10%", the member *Clara* allows "up to 5%".
- Allowing what a higher level forbids is always explicit: the rule is marked as an **exception** and shows which rule it overrides. Only someone who configures the company creates exceptions, never an agent.
- The **preview** of a member shows the context it receives and the rule that applies to each capability, with where it comes from.
- House rules (the administrator's) and the invariants (section 6) are not company rules and no company rule allows past them.

### 3. Work: routines, tasks and waiting for a decision

- **A member's routines** are ordinary routines that belong to the member, of three kinds: *compiled* (no model), *agent task* (an exploration with the member's role, model, capabilities and cost cap) and *hybrid* (a compiled watch that starts the agent only when something is new, the suggested default).
- **New triggers**: company events, such as a task assigned, a message received, a question from a subordinate, a meeting.
- **Working hours, concurrency and a queue**: one agent task at a time per member and three per company by default, by priority and due date.
- **Tasks** are the unit of work between members: requester, assignee, state (backlog, to do, doing, waiting for a decision, review, blocked, done), priority, parent, due date, cost and links.
- **Delegation** goes down the tree, and across a department when the company allows it. A delegated task must carry its **objective, acceptance criterion, constraints, what is out of scope and a link to the root task**; the host refuses one without an objective or an acceptance criterion.
- **The task's dossier** links its origin (the brief, the evidence, the decisions, the root task). It is kept by Pimpo, not by the agents, so whoever does the work reads the original and not a summary of a summary.
- **Drift**: at each level Jev answers "does this task still serve the root task's objective?"; under the threshold, the assignee asks its boss before starting.
- **Side questions**: a member may ask whoever wrote the origin to clarify it, without going through the tree. Deciding still follows the tree and the decision levels.
- **Asking the boss**: `company.ask` with the question, options, what the boss needs to see and the agent's own recommendation. The task moves to *waiting for a decision*, the exploration is saved, nothing is spent and the clock stops. The answer resumes it from the same step, also after a restart. Meanwhile the agent may take its next task.
- **A question waits until the boss answers.** There is no timeout, no escalation to someone else and no going ahead alone; there are optional reminders, which decide nothing, and the wait shows on the org chart, in the boss's queue and in **Needs you**. Questions only go up, so waits cannot form a cycle.
- **Meetings** are bounded conversations between members (an agenda, participants, a turn and cost limit) whose minutes and decisions go to the company's memory.
- **Reports**: a short daily standup per member, consolidated by the boss, and a weekly summary for the CEO.
- **Loop guards**: a delegation depth limit (4), a limit of open tasks per member, duplicate and ping-pong detection, and a cost cap per task that cannot be turned off.

### 4. Decisions, autonomy and decision levels

**Deciders.** Each kind of decision points to one: Jev (a calibrated probability and a threshold), a model, the boss, a person, a committee of members, or a cascade such as `Jev ≥ 0.9 → boss → person`. Every decision records the question, the options, who decided, the probability or the reason, and the cost.

**Autonomy matrix.** Per member, per capability or per risk: does it alone, who approves, and limits (amounts, hosts, how many a day). A member from a template gets its role's matrix; one made from scratch gets the company's default, chosen when the company is created.

**Decision levels.** The company defines levels, each with a decider; the highest is always a person, the CEO. The suggested levels are operational (the member), tactical (the boss), managerial (the head of the department) and strategic (the CEO). Criteria the company sets place a decision on a level: amount, risk, whether it is public, kind (price, contract, partnership, hiring, budget, legal, architecture...), named entities, impact, and confidence (an unsure decider moves the decision up a level).

- Deterministic criteria (amount, risk, the declared kind, the capability) are checked by the host without a model; only what they leave open goes to a classifier (Jev or a model), and an unsure classifier moves the decision up.
- A strategic decision reaches the CEO **directly**, or **with opinions**: it goes up the tree, each boss adds a recommendation and none decides.
- An agent can never lower a decision's level. Every decision records its level and the criterion that set it. No rule or exception takes a decision away from the CEO.

**Earned autonomy.** Pimpo counts, per member and per kind of delivery, how many in a row were approved without edits. After N in a row (10 by default, configurable) it suggests autonomy for that kind; a person accepts. A rejected or corrected delivery afterwards returns that kind to approval. Earned autonomy never goes past the decision levels.

### 5. Accounts of their own

- Each member has its own accounts in any service Pimpo connects: mail, calendar, GitHub, YouTube, LinkedIn, Slack, Notion, WhatsApp Business, an MCP connector, and a browser profile of its own (RFC 0002). Several per service, with labels.
- This is the per-person account mechanism with the member as one more scope: secrets live in the vault under the company and member, and the router resolves each call to the account of the member that made it. A member never reaches another member's accounts, nor its person's own.
- **Company accounts** (the official channel, the support inbox) are shared with grants per member: read, act, publish.
- **Kinds of account**: *brand* (the default for public social networks), *machine* (where the terms allow automation, such as GitHub), *service* (mail, Slack, Notion), and *a real person's* (only with that person's approval of each post, or the autonomy they gave).
- **A persona is a signature, not a profile**: on public networks a member speaks inside the brand's account and signs ("— Maya, Pimpo's AI assistant").
- **Each connector declares its service's terms**: which kinds of account it accepts, the disclosures it requires and its limits. Linking a personal social profile of a person who does not exist shows the rule it breaks.
- **AI disclosure** is set where the platform has the field. **Assisted publishing**: when an API is missing or not approved, the agent prepares the post and the person's phone opens the network's app already filled in.
- A role lists the account kinds it needs; a member starts once they are connected. A person creates and connects accounts; agents never create accounts or type passwords.

### 6. Invariants

These are not configuration:

1. No agent changes its own autonomy, budget, capabilities, accounts or decision levels, nor those of anyone above it, and no agent creates rule exceptions.
2. No agent creates accounts or types passwords.
3. A company's budget never exceeds its person's limit, which never exceeds the house's.
4. Everything is logged with a receipt, and reversible where the service allows.
5. Content from outside is never an instruction and never grants a capability, also when another agent passes it on.
6. There is always a stop button for the company, a department and a member, and it works without a model.

### 7. Costs

- Every model, judge and paid API call carries the company, department, member, routine and task.
- Budgets in layers: company (day and month), department, member (a monthly "salary"), task. At 80% and 100% a member warns, switches to a cheaper model or pauses, as configured.
- A forecast of the monthly cost from schedules and average run cost shows before hiring a member and when changing its model.
- Cost per outcome: per accepted pull request, per published video, per answered customer.

### 8. Work that needs new capabilities

Members can use any capability Pimpo has, from the catalog, MCP connectors and skills. The templates of v1.0 need these, each as a connector with contract tests against recorded responses:

| Area | Capabilities | Risk |
|---|---|---|
| Company | `company.assign`, `company.report`, `company.ask`, `company.meet`, `company.remember` | Notify |
| GitHub | `github.repo`, `github.issue_create`, `github.issue_edit`, `github.pr_create`, `github.pr_review`, `github.checks` | Read or Reversible |
| GitHub | `github.merge`, `github.release` | Irreversible |
| Code | `code.workspace`: a coding CLI working on a branch in its own worktree | Reversible |
| Media | `media.voice`, `media.capture`, `media.render`, `media.check` | Reversible |
| YouTube | `youtube.upload` (always private), `youtube.publish`, `youtube.stats` | upload Reversible, publish Irreversible |
| LinkedIn | `linkedin.post` | Irreversible |
| Publishing | `publish.assisted`: a ready post sent to the person's phone | Notify |
| Finance | `stripe.balance`, `stripe.charges`, `github.sponsors`, `costs.read` | Read |

**Coding** runs Claude Code, Codex or opencode on the host, chosen per member, in a `git worktree` per task under the company's folder, on a branch of its own, with only the member's GitHub token and the project variables the person allows: nothing from the vault, nothing from `~/.pimpo`. The code sandbox (RFC 0001) is an option per member.

**Video** is built on this machine with ffmpeg, which Pimpo already uses for audio: a script, local voices (`internal/speech`), screenshots of the demo through Pimpo's own browser, captions and fixed templates (tutorial 16:9, short 9:16). Checks without a model (length, aspect, loudness, captions) run before a person is asked. Paid video generation can be added as a connector.

### 9. The screens

**Companies** joins the side menu, in Labs until v1.0:

- the list of companies;
- creating one by describing it (a model proposes departments, roles, context, rules, routines and accounts, and nothing is created before review), from a template, or blank, with the monthly forecast before hiring;
- the **org chart**: members as cards with their role, model, live state (idle, working on, waiting for whom, paused, over budget, account missing) and today's cost; drag a card onto another to change its boss; departments as areas; on the phone, a collapsible list. The tree is drawn by the app itself, with no new UI library;
- roles, context and rules editors, and the decision levels with a simulator;
- the member page: profile, context and rules, models, autonomy, accounts, routines, questions, budget, activity and performance;
- the task board, the live timeline with meetings and decisions, costs, and talking to a member or to the company;
- company widgets on dashboards (M7).

### 10. Templates

A template is a company file with roles, context, rules and routines, signed in the gallery like routines. v1.0 ships: software company (PO, PM, two full-stack developers, CTO, marketing, finance), marketing agency, online shop, content channel, consultancy, and blank. The quality practices are part of the roles:

- the **product owner**'s briefs cite a source for every claim, and Jev flags claims the evidence does not support; signals are deduplicated across sources; the score uses the roadmap's formula (2 × value + differentiation + 2 × adoption − build risk − safety risk); each brief that ships is checked against what it predicted after 30 and 90 days; the role starts as a weekly radar of signals and earns proposing;
- **marketing** keeps a bank of references the person likes and dislikes, makes 3 to 5 variants of each hook, script or thumbnail for the person to pick with one tap, uses fixed formats, runs the checks before asking, and reports weekly what worked.

### 11. A file for each company

`company.yaml` holds the company, departments, roles, context, rules, members, routines (by reference), decision levels and account slots without secrets. It is exported and imported, carried by backups, and is what a template is.

## Safety

- **Every promise of the threat model holds for members.** A member is one more actor: every call passes its capabilities, the rules, its autonomy, the decision levels and approvals, and is logged with a receipt.
- **Injection between agents.** Content from outside keeps its low-trust mark when it travels in a task; a task with that mark cannot ask for capabilities its author does not have, and the assignee sees the mark.
- **Coding CLIs on the host.** A minimal environment, a worktree per task, a token scoped to the company's repositories, nothing from the vault; review before merging per the matrix; the sandbox as an option.
- **Runaway cost.** Layered budgets, a cost cap per task, a delegation depth limit, loop detection and a forecast before hiring.
- **Reputation and bans.** Uploads are private first, publishing is irreversible, brand accounts by default, terms per connector, platform limits, assisted publishing where there is no approved API.
- **Privacy (M6).** A company and everything in it belong to its person; partners see what their grant allows; `TestNobodySeesAnotherPersonsThings` gains every new route.
- **Decision levels.** A test bank of labeled decisions checks that no strategic decision fails to reach the CEO; false positives are acceptable, false negatives are not.

## Alternatives

- **A separate product.** Its own brand and pace, but it would rebuild the engine. The feature lives in Pimpo, in its own section and package (`internal/company`), so it can move later.
- **The hierarchy as the path of all information.** Every hop loses detail. The tree governs (who approves, pays and escalates); information travels in the dossier.
- **Lower layers may only restrict.** Safer, but real companies grant exceptions per role. Exceptions are explicit and visible instead.
- **A timeout on questions.** It makes agents act on silence; a question waits for its answer.
- **React Flow for the org chart.** Two new dependencies for a tree; the app draws the tree itself.
- **Remotion for video.** It needs Node at run time; ffmpeg is already a dependency.

## Migration

- Assistants keep working; a member is a new thing that can be made from an assistant.
- New tables only add to the frozen formats (docs/FORMATS.md); backups carry companies, and older Pimpos ignore them.
- Routines gain an optional member; existing routines are unchanged.

## Plan (M10)

Built in waves, each usable in Labs before the next. The pilot measures the most expensive bet as soon as the engine and the engineering path exist.

| Item | What | Size |
|---|---|---|
| 10.1 | Companies, departments, roles and members; the API, privacy and the company file | M |
| 10.2 | Context and rules in layers, exceptions, the preview | M |
| 10.3 | Companies in the app: list, creating one, the org chart, the member page | M |
| 10.4 | Members' routines and agent tasks: hours, concurrency, queue, pause | L |
| 10.5 | Tasks, delegation with dossier, asking the boss and waiting, drift, loop guards | L |
| 10.6 | Meetings, reports and the company's memory | M |
| 10.7 | Deciders, the autonomy matrix and the invariants | M |
| 10.8 | Decision levels and the CEO | M |
| 10.9 | Costs in layers, forecast and cost per outcome | M |
| 10.10 | Members' own accounts, kinds of account and services' terms | M |
| 10.11 | GitHub for members and coding in a worktree | L |
| 10.12 | The pilot: two weeks, measured | owner |
| 10.13 | Product owner and project manager | M |
| 10.14 | Finance | M |
| 10.15 | Media, YouTube, LinkedIn and assisted publishing | L |
| 10.16 | Earned autonomy | S |
| 10.17 | Templates, creating a company by describing it, performance, company widgets and the public showcase | M |

## Decisions (2026-09-30 and 2026-10-01)

- Companies live inside Pimpo, in Labs until v1.0.
- Members may decide with Jev or any model; everything is configurable except the invariants.
- Irreversible and public actions depend on each member's configuration; templates suggest a matrix per role.
- Coding uses the CLIs on the host, configurable per member, with the sandbox as an option.
- v1.0 covers every department of the software template.
- Any kind of company: roles, functions, context and rules are each company's own.
- Each member may have accounts of its own.
- Rules in layers restrict and allow; the most specific wins; allowing is an explicit exception.
- A question waits until the boss answers.
- Decision levels send the highest decisions to the CEO, a person.
- The pilot's cost cap per accepted pull request is configurable, $5 by default; the number of approvals before suggesting autonomy is configurable, 10 by default; the public showcase exists, optional and only with what was approved.
- A second company of another industry in the launch criteria is on hold.
