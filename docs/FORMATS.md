# Data formats

From v1.0 these formats are frozen. A newer Pimpo reads data written by any older one, and every upgrade takes a snapshot first (`before-VERSION`, brought back by **Go back to** the previous version or `pimpo restore`). A change to a format adds; it does not rename, remove or reinterpret.

The promise is tested: `internal/compat` keeps, for each frozen format, data written by the version that froze it (`internal/compat/testdata/<format>/`), and every build opens it. Those files are never edited. A new format gets a new folder, written with `PIMPO_WRITE_COMPAT=1 go test ./internal/compat -run WriteFixtures`, while the old folders stay.

| Format | Where | Version | Rule |
|---|---|---|---|
| Database (event log, routines, explorations, runs, chats, progress, vault) | `pimpo.db`, SQLite | `PRAGMA user_version` = 1 | New tables and columns only, with defaults. The event log is append-only and hash-chained (`prev`, `hash`); the way a hash is computed never changes. A Pimpo refuses a database with a newer `user_version` than it knows instead of writing to it. |
| Routine | `routine_versions.body`, gallery and repository files | 1 (no field) | JSON `{name, description, manifest, code, tests}`. New fields are optional and older Pimpos ignore them. Gallery signatures cover the bytes, so a routine is never rewritten to add a field. |
| Connector | `connectors/<name>/connector.json` | 1 (no field) | JSON manifest with `capabilities` (name, risk, signature) and `contract`. Risks keep their meaning; new fields are optional. |
| Memory | `memory/*.md` | 1 | One fact per line, with its trust (high, low, learned) and source. |
| Company file | `<name>.company.yaml`, exported from a company | `format: 1` | YAML with the company (its `memory` rules among its settings), `departments`, `roles` and `members`, never naming a person: people's seats go to whoever imports it. A Pimpo refuses a file with a newer `format`. New fields are optional. Companies themselves live in `pimpo.db` (`companies`, `company_parts`), each part as JSON, so fields are added without new columns. |
| Backup | `.pimpo` file | `PIMPO-SEALED-2` header, `manifest.format` = `pimpo-backup-2` | The header (with the scrypt cost) and then a gzip tar with a manifest listing the SHA-256 of every file, the database copy, memory, connectors and the secrets, all sealed with the passphrase (scrypt + AES-GCM). Cloud backups sealed as `PIMPO-SEALED-1` or `ZODIM-SEALED-1` still open; `pimpo-backup-1` files not sealed as a whole (and `zodim-backup-1`, `vigia-backup-1`) open only with `pimpo import --unsealed`. |

## Long-term support

Each `.0` release of an even minor version (v2.0, v2.2…) is LTS, with security fixes for 12 months in a `release/X.Y` branch ([RELEASES.md](RELEASES.md)). v1.0 starts the first line.
