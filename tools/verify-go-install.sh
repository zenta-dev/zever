#!/usr/bin/env bash
# Prove `go install github.com/zenta-dev/zever/cmd/zever@<version>` works the
# way a user runs it: outside the repo, workspace off, resolving every sibling
# module through the module proxy. Used by the release workflow after the tags
# are pushed; the proxy can lag a fresh tag, so the install is retried.
# Usage: tools/verify-go-install.sh vX.Y.Z   (env: ATTEMPTS=10 DELAY=30)
set -euo pipefail

ver="${1:-}"
if ! printf '%s' "$ver" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "usage: $0 vX.Y.Z" >&2
  exit 1
fi
attempts="${ATTEMPTS:-10}"
delay="${DELAY:-30}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cd "$tmp" # outside any go.work / go.mod

n=1
until GOWORK=off GOBIN="$tmp/bin" go install "github.com/zenta-dev/zever/cmd/zever@$ver"; do
  if [ "$n" -ge "$attempts" ]; then
    echo "error: go install cmd/zever@$ver still failing after $attempts attempt(s)" >&2
    exit 1
  fi
  echo "go install attempt $n failed; retrying in ${delay}s (proxy may lag the new tag)" >&2
  n=$((n + 1))
  sleep "$delay"
done

got="$("$tmp/bin/zever" -V)"
case "$got" in
  *"${ver#v}"*) echo "verify-go-install ok: $got" ;;
  *)
    echo "error: installed binary reports '$got', want ${ver#v}" >&2
    exit 1
    ;;
esac
