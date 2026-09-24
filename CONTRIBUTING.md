# Contributing

Thanks for helping. A few things keep Vigia trustworthy:

- **Everything is tested.** A behavior without a test is a bug waiting. Run `make check` before every commit; `scripts/commit.sh` refuses to commit when it fails.
- **English in code.** The UI speaks Portuguese and English.
- **Read [ENGINEERING.md](ENGINEERING.md).** Code should read like the code around it: few comments, and only ones that say why.
- **Safety changes go through an RFC.** Anything touching the rules engine, approvals, the vault, the runtime's capabilities, or what counts as low trust needs one (see below).

## Where to start

- **Connectors** are the easiest way in, and need no Go: see [docs/CONNECTORS.md](docs/CONNECTORS.md).
- **Gallery routines**: see the gallery README.
- **Protection list entries**: see [protection/README.md](protection/README.md).
- Issues labeled `good first issue`.

## RFCs

Copy `docs/rfcs/0000-template.md` to `docs/rfcs/NNNN-short-name.md` and open a pull request. Discussion stays open at least 7 days. Two maintainers approve, one of them outside the author's organization.

## Pull requests

- Keep each pull request to one change, with tests and a description of why.
- Two approvals for code under `internal/policy`, `internal/vault`, `internal/runtime`, `internal/host` or `internal/approval`; one elsewhere.
- Commits are signed off (`git commit -s`): you certify the Developer Certificate of Origin.
