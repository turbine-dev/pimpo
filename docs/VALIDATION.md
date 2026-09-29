# Validating Pimpo on real accounts

The proof suite checks routines against recorded scenarios. Real mailboxes, calendars and sheets are messier, so before each milestone Pimpo runs for three weeks in real households and the result is measured, not guessed ([roadmap](ROADMAP.md), item 1.3).

## What to run

At least three households, each with the routines it would really use, among the most common:

| Routine | What it exercises |
|---|---|
| Morning brief (agenda, important emails, weather) | calendar, mail search, judgments, a daily schedule |
| Inbox triage (newsletters archived, important flagged) | reversible changes, undo, judgments at volume |
| Bills and due dates | extraction from emails, reminders |
| Price watch on a shop page | web reading, a watch trigger |
| News summary as a podcast | feeds, the write step, audio |
| Reminders asked in chat | one-off schedules |
| A routine started by a webhook (a form, a Shortcut) | webhooks, events |
| A weekly routine that asks and waits for an answer | questions, answers |

Keep Pimpo running as people normally would: the desktop app on a laptop that sleeps, or the server on a machine that is always on. Both matter.

## What is measured

`pimpo report` reads the data folder and prints the last days' runs per routine; the same report is at `GET /api/report?days=21`.

```bash
pimpo report --days 21 > report.md
pimpo report --days 21 --json > report.json
```

| Measure | Meaning | Goal |
|---|---|---|
| **Silent failures** | A scheduled run that did not happen, and nobody was told, while Pimpo was running | **zero** |
| Failed runs | A run that stopped with an error; the owner is told and can repair it | each one explained, and repaired or fixed in Pimpo |
| Late | Ran after its time: caught up after the computer slept, or reported | reported, never silent |
| While off | Scheduled times that fell while Pimpo was not running (stopped, or the computer asleep) | counted apart: not a Pimpo failure, but worth knowing on a server |
| Approvals | Asked, denied, expired | irreversible actions always asked |
| Cost | What the routines spent | cents per routine per day |

Pimpo tells "not running" apart from "failed while running" with a heartbeat: it records when it starts (and when it was last alive before), and any gap of more than three minutes, such as a laptop asleep.

## After each failure

Every failure becomes a scenario in the proof suite (`testdata/proof` or `testdata/openclaw`), so the next compiler is checked against it. A failure that was Pimpo's fault gets an issue with the report's lines about it; leave out anything personal.

## Done when

Three weeks pass with no silent failure, every failure repaired or explained, and every irreversible action approved.
