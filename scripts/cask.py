#!/usr/bin/env python3
"""Fill the desktop app's Homebrew cask from the release's .dmg files.

Usage: cask.py TEMPLATE DIR VERSION > pimpo-app.rb
"""
import hashlib
import os
import sys


def sha(path):
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def main():
    template, folder, version = sys.argv[1], sys.argv[2], sys.argv[3].removeprefix("v")
    out = open(template, encoding="utf-8").read().replace("@VERSION@", version)
    for arch in ("arm64", "amd64"):
        dmg = os.path.join(folder, f"Pimpo_{version}_macos_{arch}.dmg")
        if not os.path.exists(dmg):
            sys.exit(f"missing {dmg}")
        out = out.replace(f"@SHA_{arch.upper()}@", sha(dmg))
    sys.stdout.write(out)


if __name__ == "__main__":
    main()
