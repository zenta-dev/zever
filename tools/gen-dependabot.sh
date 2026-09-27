#!/usr/bin/env bash
# Regenerate .github/dependabot.yml so every directory containing a go.mod
# gets its own `gomod` update entry (Dependabot tracks one lockfile per
# entry; a single `directory: "/"` entry only covers the root module).
# No go.work yet, so discovery is `find` over go.mod files.
#
# Usage: bash tools/gen-dependabot.sh
set -euo pipefail
cd "$(dirname "$0")/.."

out=".github/dependabot.yml"

{
  printf 'version: 2\nupdates:\n'
  find . -type f -name go.mod -not -path "./.git/*" -exec dirname {} \; \
    | sort \
    | while IFS= read -r d; do
      if [ "$d" = "." ]; then
        dir="/"
      else
        dir="${d#.}"
      fi
      printf '  - package-ecosystem: gomod\n'
      printf '    directory: "%s"\n' "$dir"
      printf '    schedule:\n'
      printf '      interval: weekly\n'
      printf '      day: monday\n'
      printf '    open-pull-requests-limit: 10\n'
      printf '    groups:\n'
      printf '      go-modules:\n'
      printf '        patterns:\n'
      printf '          - "*"\n'
      printf '\n'
    done
  printf '  - package-ecosystem: github-actions\n'
  printf '    directory: "/"\n'
  printf '    schedule:\n'
  printf '      interval: weekly\n'
  printf '      day: monday\n'
  printf '    open-pull-requests-limit: 10\n'
} > "$out"

echo "wrote $out"
