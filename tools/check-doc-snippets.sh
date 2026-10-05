#!/usr/bin/env bash
# check-doc-snippets.sh — guard against stale Go import paths / symbols in docs.
#
# Extracts ```go blocks from all *.md/*.mdx files (excluding node_modules,
# .git, any CHANGELOG.md, and
# docs/src/content/docs/getting-started/migration.mdx — old paths there are
# intentional history) and, per block:
#   1. every quoted import path starting with github.com/zenta-dev/zever
#      must resolve via `go list` (workspace mode from repo root);
#   2. every `pkg.Symbol` reference where pkg maps to a zever import in the
#      same block (explicit alias or path basename; `_`/`.` imports skipped)
#      must exist per `go doc <path>.<Symbol>` (a `doc: ...` diagnostic or
#      empty output means missing — note `go doc` exits 0 even then).
#
# Blocks WITHOUT imports: import check skipped; with no package map there is
# nothing to symbol-check against, so skipped gracefully.
# `package main` vs fragments: never compiled — only resolve+existence checks.
#
# Fails closed with file:line (block opening fence) on first violation.
# `--self-test` feeds a planted stale import (zever/cache) and expects the
# checker to fail, proving the guard bites; a valid block must pass.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PREFIX="github.com/zenta-dev/zever"

declare -A LIST_OK=() # import path -> 0 resolves / 1 stale
declare -A SYM_OK=()  # "path.Symbol" -> 0 exists / 1 missing

err() { printf '%s\n' "$*" >&2; }

# resolve_import <path> : 0 if `go list` resolves it (cached).
resolve_import() {
  local path="$1"
  if [[ -v "LIST_OK[$path]" ]]; then
    return "${LIST_OK[$path]}"
  fi
  if (cd "$ROOT" && go list "$path" >/dev/null 2>&1); then
    LIST_OK[$path]=0
    return 0
  fi
  LIST_OK[$path]=1
  return 1
}

# symbol_exists <path> <sym> : 0 if `go doc path.Sym` shows docs (cached).
# `go doc` exits 0 for missing symbols, printing `doc: no symbol ...`,
# so the output text (not the exit code) is the signal.
symbol_exists() {
  local path="$1" sym="$2" key="$path.$sym" out
  if [[ -v "SYM_OK[$key]" ]]; then
    return "${SYM_OK[$key]}"
  fi
  out="$(cd "$ROOT" && go doc "$key" 2>&1 || true)"
  if [[ -z "$out" || "$out" == doc:* ]]; then
    SYM_OK[$key]=1
    return 1
  fi
  SYM_OK[$key]=0
  return 0
}

# check_block <file> <line> <blockfile> : 0 clean, 1 violation (reported).
check_block() {
  local file="$1" start="$2" block="$3"
  local line imp alias base
  local -A pkgmap=() # alias -> import path

  # (1) import paths must resolve; build alias map.
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" != *"$PREFIX"* ]]; then
      continue
    fi
    if [[ "$line" =~ ^[[:space:]]*(import[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)?[[:space:]]*\"(github\.com/zenta-dev/zever[^\"]*)\" ]]; then
      imp="${BASH_REMATCH[3]}"
      alias="${BASH_REMATCH[2]}"
      if [[ "$alias" == "import" ]]; then
        alias=""
      fi
      if [[ -z "$alias" ]]; then
        base="${imp##*/}"
        alias="$base"
      fi
      if [[ "$alias" == "_" || "$alias" == "." ]]; then
        continue
      fi
      pkgmap["$alias"]="$imp"
      if ! resolve_import "$imp"; then
        err "$file:$start: unresolvable import \"$imp\""
        return 1
      fi
    else
      # Quoted zever path in an unparseable position — fail closed.
      # A bare (unquoted) mention is prose, not an import position: skip.
      if [[ "$line" != *"\"$PREFIX"* ]]; then
        continue
      fi
      imp="$(printf '%s' "$line" | grep -o "\"$PREFIX[^\"]*\"" | head -n 1 | tr -d '"')"
      if [[ -z "$imp" ]]; then
        err "$file:$start: unparseable zever path: $line"
        return 1
      fi
      if ! resolve_import "$imp"; then
        err "$file:$start: unresolvable import \"$imp\""
        return 1
      fi
    fi
  done < "$block"

  # No imports: nothing to symbol-check against.
  if ((${#pkgmap[@]} == 0)); then
    return 0
  fi

  # (2) pkg.Symbol refs must exist. Strip double-quoted strings first so
  # import paths / string literals can't produce phantom refs.
  local stripped sym path
  stripped="$(sed 's/"[^"]*"//g' "$block")"
  for alias in "${!pkgmap[@]}"; do
    path="${pkgmap[$alias]}"
    while IFS= read -r sym; do
      sym="${sym#*.}"
      if ! symbol_exists "$path" "$sym"; then
        err "$file:$start: $alias.$sym not found in $path"
        return 1
      fi
    done < <(printf '%s' "$stripped" | grep -oE "\\b$alias\\.[A-Z][A-Za-z0-9_]*" | sort -u || true)
  done
  return 0
}

# check_file <file> : 0 clean, 1 violation.
check_file() {
  local file="$1"
  local lineno=0 in_block=0 start=0 line
  local block
  block="$(mktemp)"
  # shellcheck disable=SC2064
  trap "rm -f '$block'" RETURN
  local fence_open='^[[:space:]]*```go'
  local fence_any='^[[:space:]]*```[[:space:]]*$'
  while IFS= read -r line || [[ -n "$line" ]]; do
    lineno=$((lineno + 1))
    if ((in_block == 0)); then
      if [[ "$line" =~ $fence_open ]]; then
        in_block=1
        start="$lineno"
        : >"$block"
      fi
    else
      if [[ "$line" =~ $fence_any ]]; then
        in_block=0
        check_block "$file" "$start" "$block" || return 1
      else
        printf '%s\n' "$line" >>"$block"
      fi
    fi
  done <"$file"
  if ((in_block == 1)); then
    check_block "$file" "$start" "$block" || return 1
  fi
  return 0
}

self_test() {
  local tmp bad badsym good
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064
  trap "rm -rf '$tmp'" RETURN
  bad="$tmp/bad.md"
  badsym="$tmp/badsym.md"
  good="$tmp/good.md"
  cat >"$bad" <<'EOF'
# stale fixture
```go
import "github.com/zenta-dev/zever/cache"

var _ = cache.New
```
EOF
  cat >"$badsym" <<'EOF'
# bogus-symbol fixture (import resolves, symbol does not)
```go
import "github.com/zenta-dev/zever/config"

var _ = config.NoSuchSymbolXYZ
```
EOF
  cat >"$good" <<'EOF'
# valid fixture
```go
import (
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

c := container.New(config.Default())
```
EOF
  if check_file "$bad"; then
    err "self-test FAIL: planted stale import (zever/cache) passed the check"
    return 1
  fi
  err "self-test: stale import correctly rejected"
  if check_file "$badsym"; then
    err "self-test FAIL: bogus symbol (config.NoSuchSymbolXYZ) passed the check"
    return 1
  fi
  err "self-test: bogus symbol correctly rejected"
  if ! check_file "$good"; then
    err "self-test FAIL: valid block rejected"
    return 1
  fi
  err "self-test PASS"
  return 0
}

if (($# > 0)) && [[ "$1" == "--self-test" ]]; then
  self_test
  exit $?
fi
if (($# > 0)); then
  err "usage: $(basename "$0") [--self-test]"
  exit 2
fi

skip_file() {
  case "$1" in
    */CHANGELOG.md|CHANGELOG.md) return 0 ;;
    *docs/src/content/docs/getting-started/migration.mdx) return 0 ;;
    # Internal agent working docs (specs/plans) may reference not-yet-existing
    # packages by design; they are not published documentation.
    *docs/superpowers/*) return 0 ;;
  esac
  return 1
}

fail=0
while IFS= read -r f; do
  if skip_file "$f"; then
    continue
  fi
  if ! check_file "$f"; then
    fail=1
    break
  fi
done < <(cd "$ROOT" && find . -type d \( -name node_modules -o -name .git \) -prune -o -type f \( -name '*.md' -o -name '*.mdx' \) -print | sort)

if ((fail == 1)); then
  exit 1
fi
err "doc-snippets: all Go blocks clean"
