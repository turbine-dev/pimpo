#!/bin/bash
# Commit only when every check passes. Usage: scripts/commit.sh "Subject" "Body"
set -euo pipefail
cd "$(dirname "$0")/.."
log=$(mktemp)
if ! make check >"$log" 2>&1; then
  tail -40 "$log"
  echo "check failed; nothing committed" >&2
  exit 1
fi
git add -A
git -c user.name="denerFernandes" -c user.email="837495+denerFernandes@users.noreply.github.com" commit -q -m "$1

${2:-}"
git log --oneline -1
