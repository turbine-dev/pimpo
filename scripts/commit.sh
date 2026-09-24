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
git -c user.name="Dener Fernandes" -c user.email="owner@example.com" commit -q -m "$1

${2:-}

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
git log --oneline -1
