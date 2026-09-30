# Releases and support

## Versions

Pimpo uses semantic versioning. From v1.0 on, the event log, routine, connector and backup formats are stable ([FORMATS.md](FORMATS.md)): newer versions read older data, a version refuses data a newer one wrote rather than damage it, and every upgrade takes a snapshot first (`pimpo restore` brings it back).

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
3. Tag `vX.Y.Z`, either by pushing the tag or from **Actions › release › Run workflow**, which asks for the version and tags the branch you pick (it refuses a version that is malformed or already exists). CI (`release.yml`) builds binaries for Linux, macOS, Windows and Raspberry Pi with checksums, and a draft release; then `desktop.yml` builds the desktop installers and attaches them:

   | System | Files |
   |---|---|
   | macOS, Apple Silicon and Intel | `.dmg`, and the `.app` as `.tar.gz` |
   | Windows x64 | `_setup.exe` (NSIS) and `.msi` |
   | Linux x64 and arm64 | `.deb`, `.rpm` and `.AppImage` |

   macOS builds are signed and notarized when the repository has the `APPLE_CERTIFICATE`, `APPLE_CERTIFICATE_PASSWORD`, `APPLE_SIGNING_IDENTITY`, `APPLE_ID`, `APPLE_PASSWORD` and `APPLE_TEAM_ID` secrets; without them they are signed ad hoc, and macOS asks to confirm the first time they open. Windows installers are signed when it has `WINDOWS_CERTIFICATE` (a code-signing `.pfx`, base64) and `WINDOWS_CERTIFICATE_PASSWORD`; without them SmartScreen warns on first run. The same run publishes the server image `ghcr.io/turbine-dev/pimpo` with the version's tag (and `latest` for a final release); `edge` follows main.

   Getting the certificates (a maintainer's task): **Apple**, join the Apple Developer Program, create a *Developer ID Application* certificate, export it as `.p12` (its base64 is `APPLE_CERTIFICATE`), and create an app-specific password for notarization (`APPLE_PASSWORD`). **Windows**, buy an OV or EV code-signing certificate from a certificate authority and export it as `.pfx`. Set each secret with `gh secret set NAME < file`.

   The same desktop builds run on demand (**Actions › desktop › Run workflow**) and on pull requests that touch `desktop/`, keeping the installers as workflow artifacts for 14 days.
4. A maintainer checks the draft and publishes it. `scripts/install.sh` verifies checksums before installing.

## Updates

**Desktop app.** It looks for a new version when it starts and every six hours, from the latest published release, or from the `channel-beta` release for people who turned on **Beta versions** in **Settings › General**. An update found shows at the top of the app and in the menu bar; installing it restarts Pimpo. Updates are signed: the app carries the project's public key (`plugins.updater.pubkey` in `desktop/src-tauri/tauri.conf.json`) and refuses anything not signed with the private one.

Before installing, the app remembers the version it leaves. **Go back to** the previous version then asks Pimpo to restore, on its next start, the snapshot the new version took of the data when it first started (`before-VERSION`), and installs the earlier version again.

**Command line and servers.** `pimpo update` checks and installs the latest release for this system, verified against the release's checksums, and `pimpo update --rollback` puts the previous binary back (see [CONFIGURATION.md](CONFIGURATION.md#running-pimpo)).

**For maintainers.** The release workflow signs update bundles when the repository has two secrets, `TAURI_SIGNING_PRIVATE_KEY` (the contents of the private key file) and `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`. It then builds the macOS `.app.tar.gz`, the Windows setup and the Linux AppImage with their `.sig` files, writes `latest.json` (`scripts/latest-json.py`) and attaches it to the release and to `channel-beta`. Without the secrets the installers are built as before and the workflow warns that automatic updates are off. The key pair was generated with `npx tauri signer generate`; the private key is held by the maintainers, never in the repository. Losing it means shipping a new public key in a version users install by hand.

## Packages and the website

| Where | How | Needs |
|---|---|---|
| Homebrew, command line | `brew install turbine-dev/tap/pimpo`, written by GoReleaser | `HOMEBREW_TAP_TOKEN`, a token that can push to `turbine-dev/homebrew-tap` |
| Homebrew, desktop app | `brew install --cask turbine-dev/tap/pimpo-app`, written by `scripts/cask.py` in `desktop.yml` | the same token |
| winget | `TurbineDev.Pimpo`, updated by `desktop.yml` | a first manual submission, then `WINGET_TOKEN` ([packaging/winget](../packaging/winget/README.md)) |
| Flathub | `app.pimpo.desktop` | building and submitting the manifest in [packaging/flatpak](../packaging/flatpak/README.md) |
| Docker | `ghcr.io/turbine-dev/pimpo`, from `docker.yml` | nothing: the workflow's token publishes it |
| Website | `site/`, from `pages.yml` | Pages set to **GitHub Actions** in the repository's settings; a domain in `site/CNAME` |

Packages are published for final releases only; betas reach people through the desktop app's beta channel and the `v…-beta.N` Docker tags.

