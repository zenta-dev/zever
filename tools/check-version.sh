#!/usr/bin/env bash
# Release-version consistency gate. Fails when:
#   1. the CLI fallback version (cmd/zever/VERSION) is lower than the newest
#      release tag of the cmd/zever module, so scaffolds never pin a stale
#      framework version (skipped when no tags are available, e.g. in a
#      shallow clone);
#   2. any file listed in tools/release-pins.txt (installers, CITATION.cff,
#      docs pins, server versions, ...) hard-codes a version other than
#      cmd/zever/VERSION;
#   3. the newest released CHANGELOG.md section is not cmd/zever/VERSION;
#   4. any intra-repo `require github.com/zenta-dev/zever/... vX` in any
#      go.mod differs from cmd/zever/VERSION (tools/modgraph --check-pins);
#   5. cmd/zever/go.mod commits a replace directive (breaks go install).
# `make release-prep VERSION=vX.Y.Z` (tools/release-prep.sh) rewrites all of
# these together; tools/tag-release.sh runs this gate before tagging/verifying.
# Usage: tools/check-version.sh [VERSION_FILE]
set -euo pipefail

root="${ZEVER_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
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

# 1. Newest stable tag cmd/zever/vX.Y.Z (pre-releases ignored).
newest="$(git -C "$root" tag --list 'cmd/zever/v*' 2>/dev/null |
  sed 's|^cmd/zever/v||' | grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' | sort -V | tail -n 1 || true)"

if [ -z "$newest" ]; then
  echo "check-version: no cmd/zever/v* tags available (shallow clone?), skipping tag comparison"
else
  lowest="$(printf '%s\n%s\n' "$ver" "$newest" | sort -V | head -n 1)"
  if [ "$lowest" != "$newest" ]; then
    echo "error: $file is $ver but newest tag is cmd/zever/v$newest; bump it (scaffolds pin this version)" >&2
    exit 1
  fi
  echo "check-version: $ver >= newest tag v$newest"
fi

bad=0
semver='[0-9]+\.[0-9]+\.[0-9]+'

# 2. Hard-coded version pins.
pins="$root/tools/release-pins.txt"
if [ -f "$pins" ]; then
  while IFS="$(printf '\t')" read -r path prefix; do
    case "$path" in ''|'#'*) continue ;; esac
    target="$root/$path"
    if [ ! -f "$target" ]; then
      echo "error: $path (listed in tools/release-pins.txt) not found" >&2
      bad=1
      continue
    fi
    found="$(grep -oE "${prefix}${semver}" "$target" | grep -oE "${semver}\$" || true)"
    if [ -z "$found" ]; then
      echo "error: $path: no version matching '${prefix}X.Y.Z' (fix tools/release-pins.txt)" >&2
      bad=1
      continue
    fi
    for v in $found; do
      if [ "$v" != "$ver" ]; then
        echo "error: $path pins $v, want $ver (run: make release-prep VERSION=v$ver)" >&2
        bad=1
        break
      fi
    done
  done <"$pins"
fi

# 3. Newest released CHANGELOG section.
changelog="$root/CHANGELOG.md"
if [ -f "$changelog" ]; then
  top="$(sed -n 's/^## \[v\([0-9][0-9.]*\)\].*/\1/p' "$changelog" | head -n 1)"
  if [ "$top" != "$ver" ]; then
    echo "error: newest CHANGELOG.md release section is v$top, want v$ver" >&2
    bad=1
  fi
fi

if [ "$bad" -ne 0 ]; then
  exit 1
fi

# 4. Intra-repo go.mod requires.
if [ -d "$root/tools/modgraph" ] && command -v go >/dev/null 2>&1; then
  (cd "$root" && go run ./tools/modgraph --check-pins) || exit 1
fi

# 5. The go install target must not commit replace directives, or
# `go install github.com/zenta-dev/zever/cmd/zever@vX` is rejected.
if grep -Eq '^[[:space:]]*replace[[:space:]]' "$root/cmd/zever/go.mod" 2>/dev/null; then
  echo "error: cmd/zever/go.mod commits replace directives; go install <module>@version would fail (run: go run ./tools/modgraph)" >&2
  exit 1
fi

echo "check-version: release pins consistent at v$ver"
