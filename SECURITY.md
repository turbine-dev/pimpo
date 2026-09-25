# Security

## Reporting a vulnerability

Please report privately, not in a public issue: use GitHub's **Report a vulnerability** button on this repository, or email owner@example.com with "Zodim security" in the subject. Include what you found, how to reproduce it, and which version you ran (`zodim version`).

What to expect:

| When | What |
|---|---|
| 72 hours | we confirm we received it and whether we can reproduce it |
| 7 days | a fix is released for the current version and the LTS version, for anything that lets someone act without approval, read secrets, or reach another person's data |
| 30 days | fixes for lower-severity issues |
| after the fix | a published advisory crediting you, unless you prefer otherwise |

## Supported versions

The latest release and the current LTS line get security fixes. See [docs/RELEASES.md](docs/RELEASES.md).

## What counts

Anything that breaks the promises in [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) counts. For example:

- an action that runs without the approval its rule requires
- a secret that reaches a model, a log, or a receipt
- content read from an email or page that becomes an instruction
- one person of the house reaching another's data
- a gallery routine or connector that does more than it declares
- a way past the web UI's authentication

## Audits

- Every change runs `go vet`, the race detector, the gate tests (200-email deletion, 50 injections, 50 memory poisonings, family isolation) and the UI accessibility checks.
- Before each release we run `govulncheck` and `npm audit`. The latest run found no reachable vulnerabilities after moving to Go 1.26.8.
- An independent audit is planned for v2 (see the roadmap). Its report will be published here in full.
