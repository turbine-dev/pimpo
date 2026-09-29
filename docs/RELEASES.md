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
3. Tag `vX.Y.Z`. CI (`release.yml`) builds binaries for Linux, macOS, Windows and Raspberry Pi with checksums, and a draft release; then `desktop.yml` builds the desktop installers and attaches them:

   | System | Files |
   |---|---|
   | macOS, Apple Silicon and Intel | `.dmg`, and the `.app` as `.tar.gz` |
   | Windows x64 | `_setup.exe` (NSIS) and `.msi` |
   | Linux x64 and arm64 | `.deb`, `.rpm` and `.AppImage` |

   macOS builds are signed and notarized when the repository has the `APPLE_CERTIFICATE`, `APPLE_CERTIFICATE_PASSWORD`, `APPLE_SIGNING_IDENTITY`, `APPLE_ID`, `APPLE_PASSWORD` and `APPLE_TEAM_ID` secrets; without them they are signed ad hoc, and macOS asks to confirm the first time they open. Windows installers are not signed yet.

   The same desktop builds run on demand (**Actions › desktop › Run workflow**) and on pull requests that touch `desktop/`, keeping the installers as workflow artifacts for 14 days.
4. A maintainer checks the draft and publishes it. `scripts/install.sh` verifies checksums before installing.
