#!/usr/bin/env python3
"""Write the desktop updater's latest.json from a folder of release files.

Usage: latest-json.py DIR REPO TAG [NOTES_FILE] > latest.json

DIR holds the installers the desktop workflow collected, with the updater's
signatures next to them (FILE.sig). Each system points at the file the
updater installs: the .app.tar.gz on macOS, the NSIS setup on Windows and
the AppImage on Linux.
"""
import json
import os
import sys
from datetime import datetime, timezone

PLATFORMS = {
    "darwin-aarch64": "_macos_arm64.app.tar.gz",
    "darwin-x86_64": "_macos_amd64.app.tar.gz",
    "windows-x86_64": "_windows_amd64_setup.exe",
    "linux-x86_64": "_linux_amd64.AppImage",
    "linux-aarch64": "_linux_arm64.AppImage",
}


def main():
    folder, repo, tag = sys.argv[1], sys.argv[2], sys.argv[3]
    notes = ""
    if len(sys.argv) > 4 and os.path.exists(sys.argv[4]):
        notes = open(sys.argv[4], encoding="utf-8").read().strip()
    files = os.listdir(folder)
    platforms = {}
    for name, suffix in PLATFORMS.items():
        found = [f for f in files if f.endswith(suffix) and f + ".sig" in files]
        if not found:
            print(f"no signed update for {name}", file=sys.stderr)
            continue
        f = sorted(found)[-1]
        signature = open(os.path.join(folder, f + ".sig"), encoding="utf-8").read().strip()
        platforms[name] = {"signature": signature, "url": f"https://github.com/{repo}/releases/download/{tag}/{f}"}
    if not platforms:
        sys.exit("no signed updates found; is TAURI_SIGNING_PRIVATE_KEY set?")
    out = {
        "version": tag.removeprefix("v"),
        "notes": notes or f"Pimpo {tag}",
        "pub_date": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "platforms": platforms,
    }
    json.dump(out, sys.stdout, indent=2)
    print()


if __name__ == "__main__":
    main()
