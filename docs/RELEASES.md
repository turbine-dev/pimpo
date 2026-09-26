# Releases and support

## Versions

Pimpo uses semantic versioning. From v1.0 on, the event log, routine and backup formats are stable: newer versions read older data, and every upgrade takes a snapshot first (`pimpo restore` brings it back).

| Channel | What | Support |
|---|---|---|
| **stable** | `vX.Y.Z` tags | until the next minor release |
| **LTS** | every `.0` release of an even minor version (v2.0, v2.2…) | security fixes for 12 months, in `release/X.Y` branches |
| **beta** | `vX.Y.Z-beta.N` tags | none; for testing |

## Security fixes

Fixes for serious issues ship within 7 days of a report, on stable and on every supported LTS line (see [SECURITY.md](../SECURITY.md)).

## How a release is made

1. `make check` and `make e2e` pass on main.
2. `govulncheck ./...` and `npm audit --omit=dev` are clean.
3. Tag `vX.Y.Z`. CI builds binaries for Linux, macOS, Windows and Raspberry Pi with checksums, desktop bundles for macOS, Windows and Linux, and a draft release.
4. A maintainer checks the draft and publishes it. `scripts/install.sh` verifies checksums before installing.
