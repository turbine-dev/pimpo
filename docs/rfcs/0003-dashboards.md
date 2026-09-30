# RFC 0003: Dashboards and widgets

- Author: Pimpo maintainers
- Status: accepted
- Discussion: the pull request that adds this file

## Problem

Pimpo does things on its own, but you only see them as notices, receipts and run history. Many routines exist to *know* something at a glance: the balance of an account, today's agenda, whether the backup ran, how many orders came in, the price of the dollar. Today that means reading a message each time. There is nowhere to *look*.

People want:

- several dashboards (home, work, the shop), each laid out as they like;
- any routine to become a widget;
- widgets outside the app, each on its own: floating on the Mac, Windows and Linux desktop, and on the home and lock screens of iOS and Android.

## Where things stand

- A run produces side effects, notices and kept state. Nothing structured comes out of it. `runtime.Result.Return` is computed and thrown away by the scheduler (`internal/scheduler/scheduler.go`), and `store.Run` has no output column.
- Capabilities are the only way a routine reaches the outside, with four risks: Read, Notify, Reversible and Irreversible (`internal/capability`).
- The compiler learns the capabilities from the catalog it lists in its prompt, and follows fixed rules for common cases (`internal/compiler/compiler.go`).
- The Home page is a fixed grid of four cards.
- The desktop app already opens an extra always-on-top window, the floating Pimpo. It shares the main window's session, remembers its corner and follows the app's lock.
- The phone apps are the Tauri template. There is no Swift or Kotlin of ours, and no App Group.
- Devices have a login token and, for automations, a `pk_` key that can only *report* phone events. Nothing can only *read*.
- Every route has an access rule, and everything personal belongs to its person, the administrator included (M6). A live event reaches someone whole only when it names them (`person` or `to`).

## Proposal

### 1. Widgets are what a routine shows

A new capability, **`widget.show`**, is what a routine calls to put something on a widget. Its risk is Notify: it only ever reaches the routine's own person, it is recorded like any other call, and a routine's tests can check it.

```js
await widget.show({
  key: "balance",                 // one routine may show several widgets
  kind: "metric",               // metric | list | status | text | table | chart
  title: "Checking account",
  value: 1843.20, unit: "BRL",  // metric
  trend: -2.4,                  // optional, percent
  status: "ok",                 // ok | warn | alert, any kind
  items: [...],                 // list and table
  link: "https://…",            // optional: opens on tap
})
```

- **Kinds**, each with a small, medium and large layout:
  - **metric**: a number, its unit, a trend and a sparkline of its history;
  - **list**: up to 20 items, each with a title, a line, a badge and a link;
  - **status**: ok, warn or alert, with a sentence;
  - **text**: a few lines;
  - **table**: up to 8 columns and 20 rows;
  - **chart**: a time series.
- **Snapshots.** Pimpo keeps the latest snapshot of each widget, with its time. It also keeps a short history of metric and chart values (the last 90) for sparklines and charts. Everything is capped (16 KB a snapshot) and validated against the kind's schema; anything else is refused.
- **Data, never instructions.** A snapshot is shown as text. Links must be `https:`. HTML is never rendered.

**Built-in widgets** need no routine:
- what needs you (approvals, questions);
- today's runs and the next ones;
- spent today against the limit;
- next reminders;
- a routine's health (last run, next run, broken or not);
- the latest recording.

### 2. Routines become widgets

- **Asking for one.** "Show me the dollar every hour", "keep a widget with today's orders". The explorer calls `widget.show` when the request is to see or track something, and the compiler gains a rule, next to the one for `notify.send`: a request to *see* or *track* ends in `widget.show`, and a request to *be told* ends in `notify.send`.
- **An existing routine.** **Turn into a widget** on the routine's page re-explores it, as **Redo with the agent** does, with the instruction to also show its result as a widget of a chosen kind. It becomes a new version with its tests, shown and approved like any other change.
- **Without changing the routine.** Every routine can be added as a *status* widget from its own runs: last result, when, and the next run. No new version needed.
- **Refreshing.** A widget refreshes when its routine runs, on its schedule, webhook or watch. **Refresh now** runs the routine once, and says what that costs when it uses a model. A snapshot older than twice the routine's interval shows as stale, with its time.

### 3. Dashboards

- **Each person has any number of dashboards**, with a name, an emoji and an order; one is the default. Home keeps its cards, and can be replaced by a chosen dashboard.
- **A grid.**
  - Desktop: 12 columns; widgets are dragged and resized to their kind's sizes.
  - Phone: the same widgets reflow to one or two columns, so no layout is kept per device.
- **Private.** Dashboards and widgets belong to their person, like everything since M6. The administrator sees nobody else's.
- **Shared dashboards are a later step.** Sharing would need a "household" snapshot, the way memory has household facts; see *Decisions*.
- **Stored in the database,** in a new `dashboards` table (layout as JSON) and a `widget_snapshots` table. It is a new table, so the frozen format grows by addition only (docs/FORMATS.md).

### 4. Widgets outside the app, each on its own

Every surface reads the same **widget feed**. `GET /api/widgets/feed` returns the chosen widgets' latest snapshots in one compact JSON file. The native widgets use it with a **widget key** (`wk_…`):

- The key is made on the device, like the automation key.
- It can only read the widgets pinned to that device. It cannot open the app or change anything.
- It is revoked with the device, and expires with it.

| Surface | How | Refresh | Needs |
|---|---|---|---|
| **Web app, phone app** | Dashboards pages, live over the event stream (`widget.updated`, which carries the person) | instant | nothing |
| **Floating widgets on macOS, Windows, Linux** | Each widget its own small always-on-top window, like the floating Pimpo: its own size and place, remembered, shown on every desktop, hidden while the app is locked | instant | nothing |
| **Android home screen** | An app widget in the Android app, one per pinned widget, placed from the app or chosen from the launcher (built with Android's own `AppWidgetProvider` and `RemoteViews`, so no new libraries) | the system's update, every 30 min, plus on open | Kotlin in `gen/android` |
| **iOS home and lock screen** | A WidgetKit extension with an App Group, and App Intents to choose the widget and, on iOS 17, to refresh or run it | the system's timeline budget, about every 15–60 min | Swift in `gen/apple`, and an **Apple Developer account** (App Groups, signing) |
| **macOS Notification Center and desktop** | The same WidgetKit extension, in the desktop app's bundle | as iOS | the Apple account, and the desktop app **signed and notarized** (2.2) |
| **Windows Widgets board** | A widget provider with Adaptive Cards | as Android | a packaged (MSIX) app; later, if people ask |
| **Linux** | Floating widgets; no standard native widget | instant | nothing |

- **Away from home.** A phone out of reach of its Pimpo keeps showing the last snapshot, with its time. It fetches again when it can, from home or through Tailscale.
- **Push instead of polling** would need a relay run by the project, for Apple's and Google's push services. That is not local-first, so the native widgets poll.

## Safety

- **Nothing new can act.**
  - `widget.show` is a Notify capability that reaches only the routine's person.
  - **Refresh now** and **Run now** go through the same rules and approvals as any run.
  - An interactive widget button can only run a routine that has no irreversible step. Anything else opens the app.
- **Privacy between people (M6).**
  - Dashboard, widget and feed routes are member routes that act only for their person. `TestNobodySeesAnotherPersonsThings` gains the new routes.
  - `widget.updated` names the person, so nobody else receives it whole.
  - Floating widgets hide while the desktop app is locked.
- **Keys.**
  - A widget key reads only the widgets pinned to its device.
  - It is kept only as a hash, dies with the device, and never opens the app (a test in the style of the automation key's).
- **The lock screen.**
  - Each widget has **Hide on the lock screen**. On iOS it uses the system's redaction.
  - It is on by default for widgets that show money or messages: `unit` is a currency, or the routine reads mail or chats.
- **What reaches the phone.** Only snapshots, never the routine, its code or its credentials. They sit in the App Group on iOS and in encrypted preferences on Android.
- **Untrusted content.** A snapshot may carry text from an email or a page, so it is always text, never markup or a link other than https. The explorer still treats that content as data.

## Alternatives

- **Using `Result.Return` as the widget.** It is already computed. But it is invisible in tests and receipts, a routine could show only one widget, and helpers already use it for something else.
- **Rendering widgets as HTML the routine writes.** Flexible, but it is exactly what a poisoned email would like to reach. Fixed kinds keep it data.
- **Push for native widgets.** Fresher, but it needs a project-run relay that sees when every widget changes.
- **Only the web dashboards.** Simplest, but it misses what people asked for: widgets on the desktop and home screen, each on its own.

## Migration

- Existing routines keep working unchanged.
- They can be added as *status* widgets at once, or turned into widgets as a new, approved version.
- The new tables and fields only add to the frozen formats. Backups carry dashboards and snapshots; older Pimpos ignore them.
- The phone apps need a new build for native widgets; until then the dashboards work in their web view.

## Plan (M7)

| Item | What | Size | Done when |
|---|---|---|---|
| 7.1 | `widget.show`, snapshots and history, built-in widgets, the dashboards API, privacy and tests | M | a routine's widget and a built-in one show on a dashboard, and the privacy sweep passes |
| 7.2 | Dashboards in the app: N per person, grid, drag and resize, phone layout, live updates, stale and refresh | M | three dashboards with ten widgets work on desktop and phone |
| 7.3 | Routines become widgets: the compiler rule, **Turn into a widget**, status widgets from runs | M | "show me the dollar every hour" makes a widget unaided in 4 of 5 tries |
| 7.4 | Floating widgets on macOS, Windows and Linux | M | three widgets float independently, keep their places and hide with the lock |
| 7.5 | The widget feed and key; Android home-screen widgets | L | an Android widget shows a routine's widget and refreshes on its own |
| 7.6 | iOS widgets (home and lock screen), then macOS WidgetKit | L | an iOS widget refreshes on its own and redacts on the lock screen · needs the Apple account |
| 7.7 | Windows Widgets board | M | only if people ask for it after 7.4 |

## Decisions (2026-09-30)

1. **No Apple Developer account for now.** The Mac gets floating widgets (7.4) and the iPhone gets dashboards inside the app. 7.6 waits.
2. **Shared dashboards: yes.** A dashboard can be shared with the house. It shows others only widgets marked shared, and their own ready-made ones.
3. **Refresh now:** it runs at once, unless the routine uses a model and its last run cost a cent or more; then it asks first.
4. **Order:** as proposed. Dashboards, routines as widgets and floating widgets come first, then Android, then iOS.
5. **Dashboards are tabs.** Any number of them, and widgets come in many good-looking kinds (numbers, goals, status, lists, tables, and line, area, bar and donut charts).
