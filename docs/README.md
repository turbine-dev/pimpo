# Pimpo documentation

## Using Pimpo

- [User guide](USER_GUIDE.md): getting started and everything you can do, screen by screen.
- [Configuration](CONFIGURATION.md): the command line, settings, models, channels, the data folder, network access and backups.
- [Running on a server](SELF_HOSTING.md): Docker, systemd, reaching it safely, and the apps opening it.
- [Validating on real accounts](VALIDATION.md): what `pimpo report` measures and what counts as done.
- [Routines](ROUTINES.md): how a request becomes a routine, what triggers it, how it is tested, and how to write or edit one.

## Extending Pimpo

None of these needs recompiling Pimpo.

- [Connectors](CONNECTORS.md): add a service with a JSON file, an OpenAPI description, an MCP server or a program.
- [Extending Pimpo](SDK.md): the channel API for new chat apps, the judge API, the Guard API, the gallery and the protection list.
- [Gallery](../gallery/README.md): publishing a routine for others.
- [Protection list](../protection/README.md) and [Guard plugins](../guard/README.md).
- Examples: [`examples/connectors`](../examples/connectors).

## Safety

- [Threat model](THREAT_MODEL.md): what Pimpo promises and how.
- [Reporting a vulnerability](../SECURITY.md).

## The project

- [Contributing](../CONTRIBUTING.md) · [Engineering practices](../ENGINEERING.md) · [Governance](../GOVERNANCE.md) · [Code of Conduct](../CODE_OF_CONDUCT.md)
- [Roadmap](ROADMAP.md): what comes next, milestone by milestone, up to v1.0.
- [Changelog](../CHANGELOG.md) · [Releases and support](RELEASES.md) · [Data formats](FORMATS.md) · [Status](STATUS.md) · [Plan](PLANNING.md) · [RFCs](rfcs/)
