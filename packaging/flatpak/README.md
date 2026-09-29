# Flatpak

`app.pimpo.desktop.yml` builds the Flatpak from the release's `.deb`, and `app.pimpo.desktop.metainfo.xml` describes the app for Flathub.

This has not been built yet: it needs Linux with `flatpak-builder`. Before submitting:

1. Replace `@VERSION@`, `@SHA256@` (the x64 `.deb`), `@SHA256_ARM64@` and `@DATE@` with the release's values.
2. Check the paths inside the `.deb` (`dpkg -c Pimpo_X.Y.Z_linux_amd64.deb`): the binaries, the icons and the `.desktop` file.
3. Build and run it: `flatpak-builder --user --install --force-clean build packaging/flatpak/app.pimpo.desktop.yml && flatpak run app.pimpo.desktop`.
4. Submit to Flathub following <https://docs.flathub.org/docs/for-app-authors/submission> (a pull request to `flathub/flathub` with the manifest).

The auto-update in the desktop app is for the other packages; a Flatpak is updated by Flathub, so the app's own updater should stay off there.
