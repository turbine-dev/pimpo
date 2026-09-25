#!/bin/sh
# A failed or interrupted DMG build leaves its scratch image mounted, and
# the next build then fails too. Detach those and remove the files.
dir="$(cd "$(dirname "$0")/.." && pwd)/desktop/src-tauri/target/release/bundle/macos"
hdiutil info | awk -v d="$dir/rw." '
  /^image-path/ { mine = index($0, d) > 0 }
  mine && /^\/dev\/disk[0-9]+[ \t]/ { print $1; mine = 0 }
' | while read -r dev; do hdiutil detach "$dev" -force >/dev/null 2>&1; done
rm -f "$dir"/rw.*.dmg
