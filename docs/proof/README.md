# Compiler proof (F0 gate)

The F0 gate: at least 8 of 10 recorded tasks compile into a routine that, **on the first attempt**, passes the replay of the exploration, its own tests and the no-memorization check, **and** passes a holdout scenario the compiler never saw (different emails, events, amounts, and traps such as `mariana@acme.com` next to `ana@acme.com`).

Fixtures: `testdata/proof/`. Runner: `go run ./cmd/proof`. Model: Claude Sonnet via Claude Code headless.

## History

| Version | What changed | First attempt + holdout | After one retry + holdout |
|---|---|---|---|
| v1 | First compiler | 6/10 | 8/10 |
| v2 | Routines may only use judgments the exploration made; transient format errors retried | 5/10 | 7/10 |
| v3 | Standard library for dates and money (goja has no `Intl`) | 7/10 | 8/10 |
| v4 | Routines declare their language | 6/10 | 8/10 |
| **v5** | Test harness applies the routine's own search like a real server; keyword filters no longer flagged as memorization | **8/10 — gate passed** | 8/10 |

Run-to-run variation is about ±1 task, so the gate result should be read with that margin. Each version's raw results are in its folder.

What the proof found in the product, not just the compiler: IMAP searches `FROM` by substring, so a search for `ana@acme.com` also returned `mariana@acme.com`. The mail connector now filters full addresses exactly, as Gmail does.

## Latest run (v5)

| Task | First attempt accepted | First attempt passes holdout | Accepted after retry | Final passes holdout | Cost |
|---|---|---|---|---|---|
| 01-morning-brief | yes | yes | yes | yes | $0.10 |
| 02-bills | no | yes | no | yes | $0.21 |
| 03-newsletter-triage | yes | yes | yes | yes | $0.06 |
| 04-boss-alert | yes | no | yes | no | $0.06 |
| 05-weekly-agenda | yes | yes | yes | yes | $0.05 |
| 06-package-tracking | yes | yes | yes | yes | $0.07 |
| 07-meeting-prep | yes | yes | yes | yes | $0.09 |
| 08-currency-alert | yes | yes | yes | yes | $0.02 |
| 09-unanswered-emails | yes | yes | yes | yes | $0.06 |
| 10-birthdays | yes | yes | yes | yes | $0.05 |

**Gate (first attempt accepted and passes the holdout): 8/10.** First attempt accepted: 9. After one retry: 9 accepted, 8 also pass the holdout. Total cost $0.78.
