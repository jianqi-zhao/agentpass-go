#!/bin/sh
set -eu

version=${1:-}
changelog=${2:-CHANGELOG.md}
if ! printf '%s\n' "$version" | grep -Eq '^v0\.[0-9]+\.[0-9]+$'; then
  echo "usage: $0 v0.x.y [CHANGELOG.md]" >&2
  exit 2
fi

# Fail before publishing if the reviewed entry is absent or empty. Never fall
# back to generated comparison links that hide breaking integration changes.
awk -v version="$version" '
  /^## / {
    if (active) exit
    if ($2 == version) { active = 1; found = 1 }
    next
  }
  active { print; if ($0 ~ /[^[:space:]]/) content = 1 }
  END { if (!found || !content) exit 1 }
' "$changelog"
