# Security

## Reporting a vulnerability

Report it on GitHub, privately, so it is not public before there is a fix:

1. Open the repository's [Security](https://github.com/turbine-dev/pimpo/security) tab and choose **Report a vulnerability** (or go straight to [the form](https://github.com/turbine-dev/pimpo/security/advisories/new)).
2. Describe what you found, how to reproduce it, and which version you ran (`pimpo version`). Leave out real keys, tokens and personal data.
3. Submit. Only the maintainers see it, and the conversation about the fix happens there.

This works like an issue, but private. For anything that is **not** a security problem, open an ordinary [issue](https://github.com/turbine-dev/pimpo/issues/new/choose) instead.

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
