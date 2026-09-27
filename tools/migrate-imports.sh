#!/usr/bin/env bash
# tools/migrate-imports.sh rewrites pre-split import paths to the
# multi-module layout, in place:
#
#   zever/<battery>/<adapter>  ->  zever/adapters/<battery>/<adapter>
#   zever/<battery>            ->  zever/core/<battery>
#   zever/internal/dsl         ->  zever/dsl
#   zever/codec                ->  zever/shared/codec
#   zever/internal/<helper>    ->  zever/shared/<helper>
#   zever/internal/redis       ->  zever/shared/redisclient
#   zever/storage/s3core       ->  zever/shared/s3opts
#
# Battery and adapter names are derived from the checkout itself
# ($ROOT/core/* and $ROOT/adapters/*/*), so the script tracks the tree
# instead of a hardcoded list. Only quoted import strings are touched;
# comments and string literals that merely mention a battery name are left
# alone. Touched files are gofmt-normalized at the end.
#
# Usage: tools/migrate-imports.sh [--root DIR] [--check] [path ...]
#   path ...   files or directories to rewrite (default: walk --root)
#   --check    list files that would change, without writing
set -euo pipefail

ROOT="."
CHECK=0
PATHS=()

while [ "$#" -gt 0 ]; do
  case "$1" in
    --root)
      ROOT="${2:?--root needs a directory}"; shift 2;;
    --check)
      CHECK=1; shift;;
    -h|--help)
      sed -n '2,20p' "$0"; exit 0;;
    *)
      PATHS+=("$1"); shift;;
  esac
done

if [ "${#PATHS[@]}" -eq 0 ]; then
  PATHS=("$ROOT")
fi

FILES=()
for p in "${PATHS[@]}"; do
  if [ -d "$p" ]; then
    while IFS= read -r f; do
      FILES+=("$f")
    done < <(find "$p" \( -name .git -o -name .worktrees \) -prune -o -type f -name '*.go' -print)
  elif [ -f "$p" ]; then
    FILES+=("$p")
  else
    echo "migrate-imports: no such file or directory: $p" >&2
    exit 1
  fi
done

if [ ! -d "$ROOT/core" ] || [ ! -d "$ROOT/adapters" ]; then
  echo "migrate-imports: --root $ROOT has no core/ or adapters/ (not a new-layout checkout?)" >&2
  exit 1
fi

MOD="github.com/zenta-dev/zever"

# Build the sed program. Adapter rules first (longest prefix wins), then
# batteries anchored on the closing quote (package-only imports), then the
# known helper renames. Every rule anchors the match end on a quote or a
# slash, so storage/s3 never claims storage/s3core's prefix.
PROG=""
while IFS= read -r d; do
  b="$(basename "$(dirname "$d")")/$(basename "$d")"
  PROG+="s|\"$MOD/$b\\([\"/]\\)|\"$MOD/adapters/$b\\1|g;"
done < <(find "$ROOT/adapters" -mindepth 2 -maxdepth 2 -type d | sort)

while IFS= read -r d; do
  b="$(basename "$d")"
  PROG+="s|\"$MOD/$b\"|\"$MOD/core/$b\"|g;"
done < <(find "$ROOT/core" -mindepth 1 -maxdepth 1 -type d | sort)

PROG+="s|\"$MOD/internal/dsl\\([\"/]\\)|\"$MOD/dsl\\1|g;"
PROG+="s|\"$MOD/codec\\([\"/]\\)|\"$MOD/shared/codec\\1|g;"
for h in registry retry cas endpoint lrucache s3opts httpclient firebase; do
  PROG+="s|\"$MOD/internal/$h\\([\"/]\\)|\"$MOD/shared/$h\\1|g;"
done
PROG+="s|\"$MOD/internal/redis\\([\"/]\\)|\"$MOD/shared/redisclient\\1|g;"
PROG+="s|\"$MOD/storage/s3core\\([\"/]\\)|\"$MOD/shared/s3opts\\1|g;"

CHANGED=()
for f in "${FILES[@]}"; do
  tmp="$(mktemp)"
  if sed "$PROG" "$f" > "$tmp"; then
    if ! cmp -s "$f" "$tmp"; then
      CHANGED+=("$f")
      if [ "$CHECK" -eq 0 ]; then
        cat "$tmp" > "$f"
      fi
    fi
  else
    echo "migrate-imports: sed failed on $f" >&2
    rm -f "$tmp"
    exit 1
  fi
  rm -f "$tmp"
done

if [ "$CHECK" -eq 1 ]; then
  if [ "${#CHANGED[@]}" -gt 0 ]; then
    printf '%s\n' "${CHANGED[@]}"
    exit 1
  fi
  exit 0
fi

if [ "${#CHANGED[@]}" -gt 0 ]; then
  gofmt -w "${CHANGED[@]}"
  printf 'migrate-imports: rewrote %d file(s)\n' "${#CHANGED[@]}" >&2
else
  echo "migrate-imports: nothing to rewrite" >&2
fi
