#!/usr/bin/env bash
# Fail when the CLI fallback version (cmd/zever/VERSION) is lower than the
# newest release tag of the cmd/zever module, so scaffolds never pin a stale
# framework version. Skips (exit 0) when no tags are available, e.g. in a
# shallow clone.
# Usage: tools/check-version.sh [VERSION_FILE]
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
file="${1:-$root/cmd/zever/VERSION}"

if [ ! -f "$file" ]; then
  echo "error: $file not found" >&2
  exit 1
fi

ver="$(tr -d '[:space:]' <"$file")"
ver="${ver#v}"
if ! printf '%s' "$ver" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "error: $file must hold X.Y.Z (got '$ver')" >&2
  exit 1
fi

# Newest stable tag cmd/zever/vX.Y.Z (pre-releases ignored).
newest="$(git -C "$root" tag --list 'cmd/zever/v*' 2>/dev/null |
  sed 's|^cmd/zever/v||' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -n 1 || true)"

if [ -z "$newest" ]; then
  echo "check-version: no cmd/zever/v* tags available (shallow clone?), skipping"
  exit 0
fi

lowest="$(printf '%s\n%s\n' "$ver" "$newest" | sort -V | head -n 1)"
if [ "$lowest" != "$newest" ]; then
  echo "error: $file is $ver but newest tag is cmd/zever/v$newest; bump it (scaffolds pin this version)" >&2
  exit 1
fi

echo "check-version: $ver >= newest tag v$newest"
