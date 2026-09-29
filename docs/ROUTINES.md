# Routines

A routine is a small JavaScript program that Pimpo runs for you: on a schedule, when something new shows up, when another service calls it, or when you answer one of its questions. You don't write it. You ask for something once, the agent does it while you watch, and Pimpo turns what happened into code that has tests and a fixed list of what it may touch. After that the routine runs without a language model. The only exception is a routine that needs a yes-or-no judgment or a short text, which it gets from a small model. This guide is for owners who make routines by asking, and for developers who write, review or share them by hand.

- [Lifecycle](#lifecycle)
- [Triggers](#triggers)
- [The routine format](#the-routine-format)
- [The JavaScript API](#the-javascript-api)
- [Limits](#limits)
- [Tests and proofs](#tests-and-proofs)
- [Writing, importing and sharing by hand](#writing-importing-and-sharing-by-hand)
- [Per-routine settings](#per-routine-settings)
- [Safety](#safety)
- [Troubleshooting](#troubleshooting)

See also [USER_GUIDE.md](USER_GUIDE.md), [CONFIGURATION.md](CONFIGURATION.md), [CONNECTORS.md](CONNECTORS.md), [SDK.md](SDK.md), [THREAT_MODEL.md](THREAT_MODEL.md) and [the gallery](../gallery/README.md).

## Lifecycle

| Step | What happens | Where you see it |
|---|---|---|
| Ask | You describe the task in chat, with **Nova tarefa**, or on a channel such as Telegram. | **Conversar**, **Início**, **Rotinas** |
| Explore | An agent does the task once over MCP. Reads are real. Changes are only simulated. | The exploration page: **Fazendo agora, com você olhando** |
| Compile | You tap **Transformar em rotina**. The compiler writes code, a manifest and tests, then checks them. | **Escrevendo a rotina e testando…** |
| Run | The routine runs on its trigger, without a model. | **Rotinas**, the routine's page, **Rotinas › Execuções** |
| Fail | A failed run stops the routine and tells you. | **Precisa de você**, a notice with **Rodar de novo** and **Refazer com o agente** |
| Repair | The agent explores the task again. Approving the result saves a new version. | The routine's **Histórico** tab |

### Explore

The agent (`internal/explore`) runs with a limit of 15 minutes and 40 turns. Its model budget per exploration is $1 at most, or what is left of the daily limit if that is less, with a floor of $0.05. It sees Pimpo's capabilities as MCP tools, named with `_` instead of `.` (`gmail_search`, `telegram_send`).

The exploration runs as a dry run:

- Calls with risk `read` really happen.
- Calls with risk `notify` (messages to you) really happen, so you get the result the way the routine will send it.
- Calls with risk `reversible` or `irreversible` are recorded and not performed. They return `{ok: true, dry_run: true}` and show as **simulado** in the list of steps. The final notice says how many actions were only simulated.
- The first time an exploration reaches a new web host, it asks you once.

Every subjective decision the agent makes ("is this email important?") must be recorded with its `decide` tool, one item at a time. The page shows each one as **Decidiu “…” · N sim, N não**. The routine can only repeat the decisions that were recorded.

When the exploration ends, you get its summary and two buttons: **Transformar em rotina** and **Descartar**. There is nothing to compile in two cases. If every read failed, a routine would only repeat the error. If the agent only set a one-time reminder (`reminder.set`), the reminder is already scheduled and nothing needs to repeat.

### Compile

The compiler (`internal/compiler`) sends the recorded trace and the capability catalog to the model for writing routines. It asks for one JSON object with `name`, `description`, `manifest`, `code` and `tests`. The attempt is then checked:

1. The manifest is valid, and something starts the routine: a schedule, a watch or a webhook.
2. Every judgment the manifest declares was made during the exploration.
3. The routine passes a **replay** of the exploration: the recorded reads, as canned answers, must lead to the same writes (derived from what the agent did: the same count, mentioning the same titles, subjects and names).
4. The routine passes its own tests.
5. The code does not hard-code data from the recording. A string literal that copies a recorded value of 10 or more characters is rejected, unless the value was part of what you asked.

If an attempt fails, its problems go back to the model and it tries again, up to 3 attempts in the app. If no attempt passes, the exploration stays ready and shows the error, and the best candidate is kept.

### Active, paused, stopped

A saved routine has one of three states:

| State | UI label | Runs? |
|---|---|---|
| `active` | **Ativa** | Yes |
| `paused` | **Pausada** | No. Webhook calls get `409`. |
| `broken` | **Precisa de atenção** | No, until you run, resume or repair it |

**Pausar** and **Reativar** on the routine's page switch between active and paused. **Rodar agora** runs the routine once. A paused routine stays paused; a routine stopped after a failure (**Precisa de atenção**) goes back on its schedule.

### Versions

Every save (compile, repair, repository update, gallery update, import) creates a new version with a reason, such as "compiled from exploration …" or "repaired from exploration …". The **Histórico** tab lists them and shows a code diff against the previous version. There is no button to go back to an older version. To restore one, put its files back in the repository folder and apply them (see [Importing](#importing)).

## Triggers

A manifest needs at least one of `schedule`, `watch` or `webhook`. Otherwise the routine is not saved. Every run receives a read-only global `event`.

| Trigger | Starts when | `event` | Recorded trigger |
|---|---|---|---|
| Schedule | The cron expression fires | `{items: []}` | `schedule` |
| Catch-up | Pimpo starts after a missed run | `{items: []}` | `catch-up` |
| Manual | You tap **Rodar agora** | `{items: []}` | `owner` |
| Watch | A poll finds items not seen before | `{items: [...]}` | `event` |
| Webhook | Another service calls the routine's address | `{webhook: {...}}` | `webhook` |
| Answer | You answer a question the routine asked | `{answer: {...}}` | `answer` |

A routine never runs twice at the same time. A second start while one run is still going fails with "already running".

### Schedule

`manifest.schedule` is a standard 5-field cron expression (minute, hour, day of month, month, day of week), read by `robfig/cron` in the owner's time zone. The scheduler also accepts descriptors such as `@daily` in the manifest. A schedule you set in **Ajustes da rotina** must be 5 fields, and it cannot fire more often than every 5 minutes. There is no RRULE support.

When Pimpo starts, it checks each active routine's last run. A run missed less than 12 hours ago runs once, late. Older misses are recorded as skipped and are not replayed.

### Watch

A watch runs the routine when a read capability returns something new. No model is used for the checks.

```json
"schedule": "",
"watch": {
  "capability": "gmail.search",
  "args": {"query": "from:{{remetente}}", "unread": true},
  "key": "id",
  "every": "10m"
}
```

- `capability` must have risk `read` and also appear in `capabilities`.
- `args` are passed to the capability. `{{name}}` in a string is replaced with the value of the parameter `name`.
- `key` is the field that identifies one item (`id`, `link`, `entity_id`). It is required.
- `every` is a Go duration. It is clamped to between 5 minutes and 24 hours, and defaults to 10 minutes.

The first poll only learns what is already there. After that, items whose key was not seen before are passed as `event.items`, each shaped like that capability's results. Pimpo remembers the last 2,000 keys per routine. The routine should work on `event.items` and not call the watched capability again. A routine can have both a schedule and a watch.

### Webhooks

Set `"webhook": true` in the manifest (with `schedule: ""` and no watch) for a routine that another service starts. Any routine can also get a webhook: on its page, turn on **Disparar por webhook**.

- Turning the webhook on creates a random 48-hex-character token, kept in the vault. The address is `/hook/{id}/{token}`, on this computer, on the home network, and on the public address when one is set. The card lists them as **Neste computador**, **Na rede de casa** and **Na internet**.
- **Trocar endereço** makes a new token, and the old address stops working at once. Turning the switch off deletes the token.
- Accepted methods are `POST`, `GET` and `PUT`. A wrong or unknown token gets `404`, which says nothing about whether the routine exists.
- A paused or stopped routine answers `409`. More than 30 calls a minute per routine get `429`. A body over 256 KB gets `413`.
- A call that passes these checks gets `202 {"ok": true, "routine": "<id>"}` at once. The routine runs in the background.

What the routine receives:

```json
{
  "webhook": {
    "method": "POST",
    "query": {"source": "shortcut"},
    "body": {"order_id": "1107", "total": 54.5},
    "received": "2026-10-08T10:00:00-03:00"
  }
}
```

`body` is the parsed JSON when the body is JSON. Otherwise it is the form fields as a string map, when the content type is a form. Otherwise it is the raw text. Repeated query or form values are joined with commas. A routine made for webhooks should do nothing when `event.webhook` is missing, for example on a manual run.

### Questions and answers

`ask.owner({question, options, key})` sends you a question. The options appear as buttons on Telegram and in **Precisa de você**, and numbered on channels without buttons. It returns `{asked: "<id>"}` at once, without waiting.

- It needs a question and 2 to 6 options. Each option is cut to 40 characters.
- `key` names the question. It defaults to the question text. A new question with the same key, from the same routine and for the same person, replaces the one still pending.
- Unanswered questions expire after 24 hours.

Your answer runs the same routine again (unless it is paused: then the answer is recorded, and you are told the routine will not act on it) with:

```json
{"answer": {"key": "treino", "question": "Treinou hoje?", "choice": "Sim", "index": 0, "asked": "2026-09-29T21:00:00-03:00"}}
```

The usual shape is:

```js
async function run() {
  if (event.answer) {
    // record event.answer.choice in state, reply with notify.send if useful
    return
  }
  await ask.owner({question: "Treinou hoje?", options: ["Sim", "Não"], key: "treino"})
}
```

Only the person the question was for, or the owner, can answer it.

### Reminders

A one-time request ("amanhã às 9h me lembra de…") does not become a routine. The exploration calls `reminder.set` and ends without offering to compile. A routine can also call `reminder.set({at, text})` or `reminder.set({in, text})` (risk `notify`), along with `reminder.list()` and `reminder.cancel({id})`. Pending reminders are listed under **Lembretes** at the top of **Rotinas**.

## The routine format

A routine is one JSON object (`internal/routine.Routine`):

```json
{
  "name": "Cotação do dia",
  "description": "A cotação em reais da moeda que você escolher, nos dias úteis.",
  "manifest": { },
  "code": "async function run() { … }",
  "tests": [ ]
}
```

In a repository folder, the same routine is split into three files. See [Writing by hand](#writing-importing-and-sharing-by-hand).

### Manifest

| Field | Type | Meaning |
|---|---|---|
| `schedule` | string | 5-field cron expression, or `""` for a routine started by a watch or a webhook |
| `capabilities` | [string] | Everything the code may call. Required, at least one. Scoped capabilities name their host: `http.getJSON:api.open-meteo.com`, `web.read:www.kabum.com.br`. |
| `judgments` | {name: question} | Yes-or-no questions about one item, answered by the judgment model. Names must be JavaScript identifiers. |
| `writes` | {name: instruction} | Short texts a small model composes per item. Each instruction is up to 500 characters. |
| `locale` | string | Language of the messages: `pt-BR`, `en-US`, `es-ES`, `fr-FR`, `de-DE`, `it-IT`, `ja-JP`, `zh-CN`, `ko-KR`, `ru-RU`. It drives `dates.format` and `money.format`. It defaults to Portuguese. |
| `params` | [Param] | Settings the owner changes without code (below) |
| `watch` | {capability, args, key, every} | What to watch (see [Watch](#watch)) |
| `webhook` | bool | Started by its webhook (see [Webhooks](#webhooks)) |
| `uses` | [string] | Ids of other routines this one runs with `routines.run`. Everything they touch must also be in `capabilities`. |

`Validate` refuses a manifest in these cases: an unknown capability, a missing or unexpected scope, a judgment or write name that is not an identifier, a write instruction that is empty or over 500 characters, a watch on a non-read or undeclared capability, a watch without `key`, a bad `every`, an empty `uses` id, or a parameter that is declared twice or invalid.

The capability catalog is in `internal/capability`, plus whatever connectors register (see [CONNECTORS.md](CONNECTORS.md#built-in-connectors)). Each capability has a risk: `read`, `notify` (a message to the owner only), `reversible` or `irreversible`. The routine page's **O que ela pode fazer** tab lists them with their risk.

### Params

```json
{"name": "moeda", "label": "Moeda", "type": "select", "options": ["USD", "EUR", "GBP"], "default": "USD", "help": "…"}
```

| Type | Value the code sees |
|---|---|
| `text` | string |
| `number` | number (a string like `"5,40"` is accepted and converted) |
| `boolean` | true or false |
| `date` | `"2026-10-05"` |
| `time` | `"07:30"` (24-hour) |
| `email` | a bare address |
| `select` | one of `options` (required) |
| `multiselect` | a list, each one of `options` (required) |
| `location` | `{name, latitude, longitude, timezone?, country?}` |
| `destinations` | a list of destination ids. It also sets where `notify.send` and `audio.send` deliver for the run. |

Every parameter needs a default, or a value set by the owner. A missing one fails the run with "<label> is not set". Unknown names are refused. Messages to the owner should use `notify.send` with a parameter `{"name": "destinos", "label": "Onde avisar", "type": "destinations", "default": []}`. With no destination chosen, messages go to the owner's usual channel.

## The JavaScript API

The code is plain JavaScript (ES2020, run by goja) that defines `async function run()`. There are no modules, `import`, `require`, `fetch`, timers, network, disk or `Intl`. `toLocaleString` and its relatives are not available for formatting either. The routine sees only the globals below.

### Capabilities

Each declared capability becomes a method on a global object named after its prefix. `gmail.search` becomes `gmail.search(...)`, and `http.getJSON` becomes `http.getJSON(...)`.

```js
const mails = await gmail.search({query: "from:banco", days: 7})
const data  = await http.getJSON("https://api.open-meteo.com/v1/forecast?latitude=" + params.cidade.latitude + "&longitude=" + params.cidade.longitude)
await notify.send({text: "…"})
```

- One argument is passed as is. Several arguments become a list. No argument becomes `{}`.
- The result is plain JSON data, with the field names the capability documents.
- For a scoped capability, the host of the URL (the argument itself for `http.getJSON`, the `url` field for `web.read`) must equal a declared scope, ignoring case. Otherwise the call fails with "… is outside the manifest scope".
- A failed call, including one blocked by a rule or denied by you, throws `Error("<capability>: <reason>")`. An error the code does not catch fails the run.
- The calls are synchronous inside the engine, so `await` is optional but harmless.

### Judgments and texts

| Call | Returns | Notes |
|---|---|---|
| `judge.<name>(item)` | `{p}`, the probability of yes | For each name in `manifest.judgments`. Treat `p >= 0.5` as yes. Pass the whole item. |
| `write.<name>(input)` | `{text}` | For each name in `manifest.writes`. At most 20 per run. It needs a model set up to write. |

Both check the daily spending limit before they call a model (judgments are estimated at $0.01 each, texts at $0.02), and both are recorded in **Atividade**.

### Data, state and helpers

| Global | What it is |
|---|---|
| `params` | The resolved parameters, frozen |
| `event` | What started the run (see [Triggers](#triggers)), frozen |
| `state.get(key)` | A value kept from earlier runs, or `null` |
| `state.set(key, value)` | Keeps plain JSON data. All state together must stay under 64 KB. |
| `state.delete(key)` | Forgets a key |
| `state.keys()` | The kept keys |
| `routines.run(id, params)` | Runs a routine listed in `uses` and returns what its `run()` returned. Only present when `uses` is set. |
| `now()` | The current time as an ISO 8601 string in the owner's zone |
| `log(text)` | A debug line, saved with the finished run |

State is saved only when the run succeeds, so a failed run never leaves half an update. The **Memória** tab shows it, and **Apagar a memória** clears it. A helper routine run with `routines.run` sees its own settings and state, but cannot change its state. Helpers cannot form a loop and can go at most 3 levels deep.

### Dates and money

All dates are ISO strings in the owner's zone and language.

| Function | Returns |
|---|---|
| `dates.zone()` | The zone name |
| `dates.today()` | Midnight today |
| `dates.startOfDay(iso, offsetDays)` | Midnight of that day plus the offset |
| `dates.addDays(iso, n)` | `iso` plus `n` days |
| `dates.addHours(iso, n)` | `iso` plus `n` hours (fractions allowed) |
| `dates.diffDays(a, b)` | Whole calendar days from `a` to `b` |
| `dates.sameDay(a, b)` | Boolean |
| `dates.isBefore(a, b)` | Boolean |
| `dates.weekday(iso)` | 0 for Sunday through 6 for Saturday |
| `dates.format(iso, pattern)` | Text. Tokens: `EEEE` (weekday), `EEE`, `d`, `dd`, `MMMM` (month), `MMM`, `MM`, `yyyy`, `HH`, `mm`. Text in single quotes is copied as is: `"d 'de' MMMM"`. |
| `dates.parse(text)` | ISO date of the first date in free text (`2026-09-26`, `26/09/2026`, `26/09`, `26 de setembro`, `Sep 26, 2026`), or `null` |
| `money.find(text)` | The first amount as written (`"R$ 1.482,35"`), or `null` |
| `money.parse(text)` | A number, reading Brazilian (`1.482,35`) or US (`1,482.35`) style, or `null` |
| `money.format(value, currency)` | `BRL`, `USD`, `EUR` or `GBP`, in the routine's language |

A date that is not ISO throws `dates: "…" is not an ISO date`. Calendar all-day events have a plain date (`2026-09-25`) as `start`.

## Limits

| Limit | Value | Where |
|---|---|---|
| The routine's own running time | 90 s per scheduled run, 5 s in checks and tests | `internal/scheduler`, `internal/routine` |
| Whole run, waits included | 15 minutes | `internal/scheduler` |
| Capability calls per run | 500 (judgments count, helpers share the total) | `internal/runtime` |
| Written texts per run | 20 | `runtime.MaxWrites` |
| Kept state | 64 KB as JSON | `runtime.MaxState` |
| Routines running routines | 3 levels | `runtime.MaxDepth` |
| Schedule you set | at most every 5 minutes | `internal/app` |
| Watch interval | 5 minutes to 24 hours, default 10 minutes | `runtime.Watch` |
| Webhook body and rate | 256 KB, 30 calls a minute | `internal/app/webhooks.go` |
| Approval wait | 30 minutes; the time a run waits for approval does not count toward its 15 minutes | `internal/approval`, `internal/pause` |
| Repository files | 1 MB each, no symbolic links | `internal/repo` |

The time limit counts only the routine's own work. A work clock stops while the routine waits on the host (a capability call, a judgment, a text, a helper routine) and starts again when the answer comes back. A routine that judges 200 emails one by one is therefore bounded by the 15-minute limit, not by the 90 seconds. Past its limit, the run fails with "routine ran past its time limit".

## Tests and proofs

### Scenarios

A test is a named scenario (`internal/trace.Scenario`). The routine runs against canned data, and Pimpo checks the calls it makes that change something or send something.

| Field | Meaning |
|---|---|
| `name` | The test's name |
| `now` | The clock, RFC 3339. Write it in UTC (ending in `Z`). |
| `responses` | `[{capability, result}]`, canned results for reads. Calls to the same capability take them in order, and the last one repeats. A capability with no response returns `[]`. The capability may carry its scope (`http.getJSON:brapi.dev`). |
| `params` | Values for this test. The rest keep their defaults. |
| `event` | What starts the run: `{items: [...]}`, `{webhook: {...}}` or `{answer: {...}}` |
| `judgments` | `{judgment: {substring of the item: probability}}`. An unlabeled item gets 0.05, a clear no. |
| `writes` | `{write: {substring of the input: text}}`. Without a match, the text is `(name)`. |
| `state` | What earlier runs kept |
| `expect_state` | `{key: value}` the run must keep. Timestamps compare as instants. |
| `expect` | `[{capability, count?, contains?, not_contains?}]` |

Every call that is not a read (`notify`, `reversible` or `irreversible`) is recorded, not performed, and answers `{ok: true}`. An expectation matches the text of every string and number in the arguments, without regard to case. For an expectation on `telegram.send`, `whatsapp.send` or `notify.send`, a call to any of the three counts. A failed check says what was actually sent, so you can tell a wrong routine from a wrong test.

`gmail.search` and `calendar.events` are answered like a real server: all the items in their responses form a pool, and each call gets the items that match its own arguments (Gmail query operators, `unread`, `days`, `max`, date ranges). There are two modes:

- **World scenarios**: the replay of an exploration and a holdout. They apply every filter, including `after:`, `before:`, `newer_than:`, `older_than:`, `category:`, `label:` and `in:`. The responses stand for what the service holds.
- **The routine's own tests**: they skip those date, category, label and folder operators. The responses answer the routine's own question, so the test does not have to label every item.

For a watching routine, a world scenario without `event` gets one made the way the scheduler would make it: the watched capability, asked with the watch's arguments, returns every item as new.

### Traces and the holdout

A trace (`internal/trace.Trace`) is a recorded exploration: `id`, `request`, `now`, `calls` (`capability`, `args`, `result`, `error`), `judgments`, `questions`, `outcome`, `expect`, an optional `event`, and an optional `holdout`. The holdout is a second world scenario that the compiler never sees. A routine that only memorized the recording fails it.

Example, shortened from [`testdata/openclaw/oc-23-order-webhook.json`](../testdata/openclaw/oc-23-order-webhook.json):

```json
{
  "id": "oc-23-order-webhook",
  "request": "Quando minha loja mandar o webhook de pedido novo, me avisa no Telegram com o nome do cliente, o valor e os itens.",
  "now": "2026-09-24T14:12:00-03:00",
  "event": {"webhook": {"method": "POST", "query": {}, "body": {
    "order_id": "1042", "customer": {"name": "Marina Souza"}, "total": 189.9, "currency": "BRL",
    "items": [{"name": "Caneca Pimpo", "qty": 2}, {"name": "Camiseta", "qty": 1}]}}},
  "calls": [
    {"capability": "telegram.send",
     "args": {"text": "🛒 Pedido 1042 de Marina Souza: R$ 189,90 — 2× Caneca Pimpo, 1× Camiseta"},
     "result": {"ok": true}}
  ],
  "outcome": "Uma mensagem para cada pedido que a loja manda, com cliente, valor e itens.",
  "expect": [{"capability": "telegram.send", "count": 1, "contains": ["Marina Souza", "189,90", "Caneca Pimpo"]}],
  "holdout": {
    "now": "2026-10-08T10:00:00-03:00",
    "responses": [],
    "event": {"webhook": {"method": "POST", "query": {}, "body": {
      "order_id": "1107", "customer": {"name": "Rafael Lima"}, "total": 54.5, "currency": "BRL",
      "items": [{"name": "Adesivos", "qty": 5}]}}},
    "expect": [{"capability": "telegram.send", "count": 1,
                "contains": ["Rafael Lima", "54,50", "Adesivos"], "not_contains": ["Marina"]}]
  }
}
```

Explorations in the app have no holdout. The holdout is used by the compiler proof.

### Running the proof

`cmd/proof` compiles every trace in a folder and checks each routine against the replay, its own tests and the holdout. It uses the Claude Code CLI (`llm.ClaudeCLI`), so Claude Code must be installed and signed in.

```sh
make proof                                          # go run ./cmd/proof -workers 4
go run ./cmd/proof -dir testdata/openclaw -out docs/proof/openclaw
go run ./cmd/proof -dir testdata/openclaw -only oc-22,oc-23 -attempts 1
go run ./cmd/proof -out docs/proof/openclaw -reuse -export /tmp/oc-routines
```

| Flag | Default | Meaning |
|---|---|---|
| `-dir` | `testdata/proof` | Folder of traces (`*.json`) |
| `-out` | `docs/proof` | Where `results.json` and `README.md` go |
| `-model` | `sonnet` | Model for the compiler |
| `-workers` | 3 | Compilations in parallel |
| `-only` | | Only traces whose id contains one of these, comma-separated |
| `-attempts` | 3 | Compile attempts, with feedback, as the app makes |
| `-export` | | Also write the accepted routines that pass the holdout to this folder, in the repository layout |
| `-reuse` | false | Compile nothing. Export from the `results.json` already in `-out` (needs `-export`). |

The report counts the gate (the first attempt is accepted and passes the holdout), first-attempt acceptance, final acceptance, final holdout passes, and cost. Exported routines take the trace's id without its numbering (`oc-23-order-webhook` becomes `order-webhook`). `testdata/proof/` and `testdata/openclaw/` hold the traces (23 in `openclaw`, listed in its [README](../testdata/openclaw/README.md)).

## Writing, importing and sharing by hand

### Repository layout

```
routines/
  cotacao-do-dia/
    routine.json   name, description, manifest
    routine.js     the code
    tests.json     a list of tests
```

Folder names are lowercase letters, digits and dashes, starting with a letter or digit, up to 64 characters. `routine.json` needs a `name`, and `routine.js` must exist. `tests.json` is optional, but a routine without tests fails the audit, so it cannot be imported.

A complete, valid example, adapted from [`gallery/routines/dolar-hoje.json`](../gallery/routines/dolar-hoje.json) (the arrow emoji is left out and only one of its tests is kept):

`routine.json`

```json
{
  "name": "Cotação do dia",
  "description": "A cotação em reais da moeda que você escolher, nos dias úteis.",
  "manifest": {
    "schedule": "0 10 * * 1-5",
    "capabilities": ["http.getJSON:economia.awesomeapi.com.br", "notify.send"],
    "locale": "pt-BR",
    "params": [
      {"name": "moeda", "label": "Moeda", "type": "select", "options": ["USD", "EUR", "GBP"], "default": "USD"},
      {"name": "destinos", "label": "Onde avisar", "type": "destinations", "default": []}
    ]
  }
}
```

`routine.js`

```js
async function run() {
  const q = (await http.getJSON("https://economia.awesomeapi.com.br/json/last/" + params.moeda + "-BRL"))[params.moeda + "BRL"];
  const change = Number(q.pctChange);
  await notify.send({text: params.moeda + ": " + money.format(Number(q.bid), "BRL") +
    " (" + (change > 0 ? "+" : "") + change.toFixed(2).replace(".", ",") + "% hoje)"});
}
```

`tests.json`

```json
[
  {
    "name": "euro",
    "now": "2026-09-24T07:00:00-03:00",
    "params": {"moeda": "EUR"},
    "responses": [
      {"capability": "http.getJSON", "result": {"EURBRL": {"bid": "6.02", "pctChange": "0.1"}}}
    ],
    "expect": [
      {"capability": "notify.send", "count": 1, "contains": ["EUR:", "6,02", "+0,10%"]}
    ]
  }
]
```

When you write a routine by hand, follow the rules the compiler follows:

- Never copy data into the code. Derive it from capability results.
- Put anything personal (a city, a threshold, a sender) in `params`.
- Use a judgment for anything subjective and plain code for anything objective.
- Declare the smallest set of capabilities the code calls.
- Write tests with new fictional data: one test that changes a parameter, one where nothing should be sent, and, for a routine with state, one with and one without earlier state.

### Importing

```sh
pimpo routines import [--active] [--data DIR] FOLDER
```

`FOLDER` holds a `routines/` directory in the layout above. `--data` is the data directory and defaults to `~/.pimpo`; it is created if it does not exist. Each routine is checked before it is saved:

1. The manifest is valid.
2. Every one of its own tests passes.
3. An audit shows it calls only what it declares.

A routine whose content is already installed is skipped ("already installed"). Routines arrive paused unless you pass `--active`, so review each routine's settings and tap **Reativar**. A running Pimpo lists them at once. Output lines start with `✓` (installed), `=` (unchanged) or `✗` (refused, with the reason).

In the app, **Rotinas › Repositório** does the same with a folder you choose: **Enviar rotinas para a pasta** writes every routine in this layout and makes a local commit, **Publicar (git push)** pushes, **Buscar mudanças (git pull)** pulls, and each changed routine can be applied (**Instalar** or **Atualizar**) after the same checks. It shows what a change adds (**Passa a poder: …**) and removes (**Deixa de usar: …**).

### Exporting

There is no `pimpo routines export` command. To get your routines as files, use **Enviar rotinas para a pasta** in **Rotinas › Repositório**. `go run ./cmd/proof -export DIR` writes compiled proof routines in the same layout.

### Publishing to the gallery

**Publicar** on a routine's page signs it with your author key, which is made on first use and kept in the vault. A routine without tests cannot be published. You then download the signed entry and open a pull request, as [gallery/README.md](../gallery/README.md) explains. From the terminal:

```sh
pimpo gallery keygen --out KEYFILE
pimpo gallery build --key KEYFILE --author ID --name "Your Name" DIR   # signs DIR/routines/*.json into DIR/index.json
pimpo gallery verify INDEX                                             # file or URL; fails on any problem
```

Before installing a gallery routine, Pimpo checks all of the following:

- The author is listed.
- The Ed25519 signature covers exactly this entry's id, author, code, manifest and tests.
- The content matches its hash.
- The hash is not revoked.
- The routine does not use other routines (`uses` is not allowed in the gallery).
- Its tests pass.
- The audit finds no undeclared call.

Gallery routines install active. When the gallery has a newer version, the routine's settings show **Nova versão na galeria** with **Atualizar**, which keeps your schedule and settings.

## Per-routine settings

**Ajustes da rotina** is at the top of every routine's page. Saving (**Salvar ajustes**) checks every value against the manifest before anything is stored.

| Setting | What it changes |
|---|---|
| **Quando** | The schedule: **Todo dia**, **Dias úteis**, **Dias da semana**, **Todo mês**, **A cada algumas horas**, **A cada alguns minutos** (5, 10, 15 or 30) or **Avançado (cron)**. **Voltar ao horário original** restores the manifest's schedule. |
| **Verificar a cada** | For a watching routine: 5 minutes to 1 day |
| The routine's params | One field per parameter: a city search for `location`, a yes/no choice for `boolean`, destinations with **Conectar mais canais**, and so on |
| **Modelo dos julgamentos e textos** | Shown only when the routine has judgments or writes. It picks the model for this routine alone (**Padrão** is the judgment model in **Ajustes › Modelos**) and its thinking level (low, medium, high or max). |

A routine started only by its webhook shows "Começa quando o webhook desta rotina é chamado" instead of a schedule. The webhook switch is the separate **Disparar por webhook** card below the settings. A routine with no parameters and no judgments says so and offers **Refazer com o agente** to gain settings.

Routines spend money only on judgments and written texts. Each one is checked against the daily limit before the model is called. When the limit is reached, the call fails, the run fails, and the notice says "O limite de gasto do dia acabou." The routine's page shows what it cost this month, and **Rotinas › Execuções** shows the cost of each run. Exploring and compiling also need budget left. Each compile attempt is capped at $2. See [CONFIGURATION.md](CONFIGURATION.md) for the daily limit and models.

## Safety

What a routine can do:

- Call the capabilities its manifest declares, and nothing else. The runtime creates only those objects, so any other name does not exist in the engine. Scoped capabilities reach only their declared hosts.
- Keep up to 64 KB of state, run declared helper routines that touch nothing beyond its own capabilities, ask the owner a question, and ask a small model for judgments and texts.

What it cannot do: reach the network, disk, processes, timers or the clock except through the capabilities and `now()`, read credentials, or change its own manifest.

Every capability call, from a routine or an exploration, goes through the host (`internal/host`):

1. **Policy.** The protection list is checked first, then the owner's rules (**Regras**). With no matching rule, the risk decides: reversible changes are allowed and kept undoable, and everything else is allowed. The balanced preset asks before anything irreversible. `whatsapp.send_to` and `ha.critical` always ask, whatever the rules say. A guest's changes always wait for the person responsible for them.
2. **Approval.** When the verdict is to ask, the run waits at that step. You get **Permitir**, **Todos desta vez** (the rest of this run), **Sempre** (this routine and capability from now on) or **Negar**. A denial or an unanswered request makes the call throw.
3. **Reversible form.** When a rule says "make it reversible", `gmail.delete` becomes `gmail.trash` and `gmail.send` becomes `outbox.send_later`, a send that waits long enough to be cancelled.
4. **Receipt.** Every call is written to the event log with its source (`routine:<id>#<run>`), arguments, result, risk, verdict, rule and approval. Receipts appear in **Atividade**. The only calls left out are successful reads made by watch polls.

Undo (`internal/undo`) works from the receipt. An archive or trash is restored, a label or draft is removed, a delayed send is cancelled, and a connector can return its own undo step (`sheets.append` is undone with `sheets.clear`, for example). Simulated and failed actions cannot be undone, and each action can be undone once.

Before a routine is saved from any source (compiler, repository, import or gallery), its own tests run and the audit runs them again with every capability reachable. Calling anything undeclared, or a host outside the scope, is a refusal. See [THREAT_MODEL.md](THREAT_MODEL.md) for the attackers and defenses, and [SDK.md](SDK.md) for the judge and guard APIs.

## Troubleshooting

| Symptom | Cause and what Pimpo does |
|---|---|
| "⚠️ … não rodou" with **Rodar de novo** and **Refazer com o agente** | The run failed. The routine is now stopped (**Precisa de atenção**) and does not run on its trigger again until you run, resume or repair it. The error shows on its page. |
| "routine ran past its time limit" | The code itself worked for more than 90 seconds. Waiting on services does not count. Look for a loop over a large result. |
| "routine made more than 500 capability calls" | A loop called a capability, or judged items, too many times. Filter in the query first. |
| "… is outside the manifest scope" | A URL's host is not the declared one. `www.` counts as part of the host. |
| "… is not set" or "the routine has no setting …" | A parameter has no value, or a setting is left over from an older version. Save **Ajustes da rotina** again. |
| "state would exceed 64 KB" | Keep less: the latest values, the last few ids. |
| "no model is set up to write text" or "no judgment backend configured" | Set a model for judgments in **Ajustes › Modelos**. |
| "O limite de gasto do dia acabou." | A judgment or text would pass the daily limit. |
| "… needs your approval" or "was not approved" | A rule asks, and nobody answered in time or you said no. **Sempre** stops the asking for this routine. |
| "… is already running" | The previous run has not finished. |
| "routine … is stopped after a failure" | A helper routine is stopped. Repair it first. |
| The exploration offers no routine | Every read failed, or it was a one-time reminder. |
| "the routine did not pass its checks: …" | No compile attempt passed. The exploration stays ready, and you can try again. |
| A run missed while the computer was off | It runs once, late, if it was due less than 12 hours ago. Otherwise it is skipped. |

### Repair

**Refazer com o agente** (on the routine's page, in the failure notice, or in **Precisa de você**) starts a new exploration of the original request, linked to the routine, telling the agent the error of the last failed run. You watch it and approve it as usual. Approving compiles a new version, and that version must also pass the tests of the version it replaces:

- The old tests are carried over (`carryTests`).
- A test that set a parameter the repair removed keeps working if it set that parameter to its old default. The value is simply dropped.
- A test that set a removed parameter to anything else was about that setting, so it is dropped. The event log records it as `routine.tests.dropped`.
- If a carried test fails, the repair is refused with "the repaired routine breaks what the old one did".

A successful repair saves the new version and makes the routine active again.
