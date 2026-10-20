#!/usr/bin/env bash
# Tag every Go module in the repo for a release.
# Usage: tools/tag-release.sh [--dry-run|--check|--verify|--push|--verify-proxy] v0.6.0
# Creates one tag per tracked go.mod found via find, excluding nested
# worktrees, harness checkouts, and vendored copies: every module is tagged
# `<reldir>/<version>` where <reldir> is the module dir relative to the
# repo root (e.g. adapters/cache/redis/v0.5.0), plus the bare `<version>`
# release tag on the same commit. The bare tag is what
# .github/workflows/release.yml fires on (it triggers on `push: tags: ["v*"]`,
# which matches a bare tag only), so a release without it ships no binaries,
# no SBOMs and no attestation.
# Tags are emitted in dependency order over three passes so leaf tags exist
# before dependents: (1) shared/*, (2) core/* plus config, container, orm,
# dsl and cmd/*, (3) adapters/* and everything else (examples, tools, docs).
# Within each pass, modules go alphabetically by go.mod path.
# With --dry-run (alias --check), list tags in order without creating them.
# With --verify, create nothing: confirm every module's tag exists locally or
# on origin (remote tag listing; skipped when no origin is configured) and
# exit 1 listing the missing ones. The bare release tag is checked too: it
# is part of the release. Used by the release docs.
# Before any mode runs, tools/check-version.sh must pass: every version pin and
# every intra-repo go.mod require equals cmd/zever/VERSION (== <version>).
# With --push, create the tags and publish them to origin in ONE
# `git push --atomic`, so a rejected ref (hook, protection rule, network) leaves
# the remote with none of the lockstep tags rather than a half-released set
# that the module proxy would cache forever. The bare `<version>` tag rides
# along in that same atomic push: the release workflow must not start before
# every module tag is on origin, and a half-pushed release must not exist.
# Without --push the script never pushes and prints the equivalent command.
# With --verify-proxy, create nothing: ask the module proxy (GOPROXY, default
# proxy.golang.org) to resolve `<module>@<version>` for every tagged module,
# retrying while the proxy catches up (ATTEMPTS=10, DELAY=30 seconds). Run it
# after the push (the release workflow does) to catch tags the proxy cannot
# serve before users hit them. The bare tag is not a module, so the proxy
# check ignores it.
set -euo pipefail

usage="usage: $0 [--dry-run|--check|--verify|--push|--verify-proxy] <version>  (e.g. $0 v0.5.0)"

dry_run=0
verify=0
push=0
proxy=0
if [ "$#" -eq 2 ]; then
  case "$1" in
    --dry-run|--check) dry_run=1; shift ;;
    --verify) verify=1; shift ;;
    --push) push=1; shift ;;
    --verify-proxy) proxy=1; shift ;;
    *)
      echo "$usage" >&2
      exit 1
      ;;
  esac
fi

if [ "$#" -ne 1 ]; then
  echo "$usage" >&2
  exit 1
fi
ver="$1"
case "$ver" in
  v?*) ;;
  *)
    echo "error: version must start with 'v' (got '$ver')" >&2
    exit 1
    ;;
esac

created=0
skipped=0
missing=0
all_tags=()
proxy_bad=0
remote_tags=""
if [ "$verify" -eq 1 ] && git remote get-url origin >/dev/null 2>&1; then
  remote_tags=$(git ls-remote --tags origin 2>/dev/null | sed 's#.*refs/tags/##; s#\^{}$##' || true)
fi

# cmd/zever/VERSION is the CLI's fallback version (see cmd/zever/version.go).
# It must be committed before tagging so the tag carries it; update it here
# and stop so the change can be committed, then re-run.
version_file="cmd/zever/VERSION"
if [ -f "$version_file" ] && [ "$(tr -d '[:space:]' <"$version_file")" != "${ver#v}" ]; then
  if [ "$dry_run" -eq 1 ]; then
    echo "would update $version_file to ${ver#v} (commit it before tagging)"
  else
    printf '%s\n' "${ver#v}" >"$version_file"
    echo "updated $version_file to ${ver#v}; commit it, then re-run $0 $ver" >&2
    exit 1
  fi
fi

# Every hard-coded version (installers, CITATION.cff, docs pins, CHANGELOG
# top section) and every intra-repo go.mod require must equal the release
# version before anything is tagged or verified: published go.mod files
# cannot be fixed after the proxy caches them. Fix with `make release-prep`.
check_script="$(cd "$(dirname "$0")" && pwd)/check-version.sh"
if [ -f "$version_file" ] && [ -f "$check_script" ]; then
  ZEVER_ROOT="$PWD" bash "$check_script" || {
    echo "error: release pins do not match $ver; run: make release-prep VERSION=$ver" >&2
    exit 1
  }
fi

pass1=()
pass2=()
pass3=()
for f in $(find . -type f -name go.mod -not -path "*/.git/*" -not -path "*/.worktrees/*" -not -path "*/.claude/*" -not -path "*/node_modules/*" -not -path "*/testdata/*" | sort); do
  d=$(dirname "$f")
  # Strip leading ./ for pass matching; root "." falls through to the
  # tag step, which skips it (no root module).
  rd="${d#./}"
  case "$rd" in
    shared/*) pass1+=("$f") ;;
    core/*|config|container|orm|dsl|cmd/*) pass2+=("$f") ;;
    *) pass3+=("$f") ;;
  esac
done

# verify_on_proxy MODULE: succeed once the module proxy resolves
# MODULE@$ver, retrying while the proxy catches up with a fresh tag. Runs in a
# scratch directory with the workspace off so only the proxy answers.
verify_on_proxy() {
  _mod="$1"
  _n=1
  until (cd "$proxy_dir" && GOWORK=off GOFLAGS=-mod=mod go list -m "$_mod@$ver" >/dev/null 2>&1); do
    if [ "$_n" -ge "${ATTEMPTS:-10}" ]; then
      echo "proxy cannot resolve: $_mod@$ver" >&2
      proxy_bad=$((proxy_bad + 1))
      return
    fi
    _n=$((_n + 1))
    sleep "${DELAY:-30}"
  done
  created=$((created + 1))
}

proxy_dir=""
if [ "$proxy" -eq 1 ]; then
  proxy_dir="$(mktemp -d)"
  trap 'rm -rf "$proxy_dir"' EXIT
fi

tag_one() {
  f="$1"
  mod=$(sed -n 's/^module[[:space:]]\+//p' "$f" | head -n 1)
  if [ -z "$mod" ]; then
    echo "warning: no module line in $f, skipping" >&2
    return
  fi
  d=$(dirname "$f")
  if [ "$d" = "." ]; then
    return # no root module: submodules tag as <reldir>/vX only
  else
    tag="${mod#github.com/zenta-dev/zever/}/$ver"
  fi
  all_tags+=("$tag")
  if [ "$proxy" -eq 1 ]; then
    # Only modules whose path equals prefix + dir can be resolved by tag
    # (the out-of-tree consumer proofs keep deliberate exceptions).
    if [ "$mod" != "github.com/zenta-dev/zever/${d#./}" ]; then
      skipped=$((skipped + 1))
      return
    fi
    verify_on_proxy "$mod"
  elif [ "$verify" -eq 1 ]; then
    if git rev-parse -q --verify "refs/tags/$tag" >/dev/null 2>&1; then
      skipped=$((skipped + 1))
    elif printf '%s\n' "$remote_tags" | grep -qxF "$tag"; then
      skipped=$((skipped + 1))
    else
      echo "missing tag: $tag"
      missing=$((missing + 1))
    fi
  elif git rev-parse "$tag" >/dev/null 2>&1; then
    echo "exists, skipping: $tag"
    skipped=$((skipped + 1))
  elif [ "$dry_run" -eq 1 ]; then
    echo "would tag: $tag"
    created=$((created + 1))
  else
    git tag "$tag"
    echo "tagged: $tag"
    created=$((created + 1))
  fi
}

for f in "${pass1[@]}"; do
  tag_one "$f"
done
for f in "${pass2[@]}"; do
  tag_one "$f"
done
for f in "${pass3[@]}"; do
  tag_one "$f"
done

# The bare release tag: the release workflow (.github/workflows/release.yml)
# triggers on `push: tags: ["v*"]`, which matches a bare tag only, so the
# per-module tags alone would ship no artifacts. It is created on the same
# commit as the module tags and pushed in the same atomic push, so the
# workflow cannot start before every module tag is on origin.
tag_root() {
  all_tags+=("$ver")
  if [ "$proxy" -eq 1 ]; then
    return # the bare tag is not a module: the proxy has nothing to resolve
  elif [ "$verify" -eq 1 ]; then
    if git rev-parse -q --verify "refs/tags/$ver" >/dev/null 2>&1; then
      skipped=$((skipped + 1))
    elif printf '%s\n' "$remote_tags" | grep -qxF "$ver"; then
      skipped=$((skipped + 1))
    else
      echo "missing tag: $ver"
      missing=$((missing + 1))
    fi
  elif git rev-parse "$ver" >/dev/null 2>&1; then
    echo "exists, skipping: $ver"
    skipped=$((skipped + 1))
  elif [ "$dry_run" -eq 1 ]; then
    echo "would tag: $ver"
    created=$((created + 1))
  else
    git tag "$ver"
    echo "tagged: $ver"
    created=$((created + 1))
  fi
}

tag_root

if [ "$proxy" -eq 1 ]; then
  if [ "$proxy_bad" -gt 0 ]; then
    echo "verify-proxy failed: the module proxy cannot resolve $proxy_bad module(s) at $ver ($created resolved, $skipped skipped)." >&2
    exit 1
  fi
  echo "verify-proxy ok: the module proxy resolves $created module(s) at $ver ($skipped skipped)."
  exit 0
fi

if [ "$verify" -eq 1 ]; then
  if [ "$missing" -gt 0 ]; then
    echo "verify failed: $missing tag(s) missing for $ver ($skipped present)." >&2
    exit 1
  fi
  echo "verify ok: all $skipped tag(s) for $ver exist."
  exit 0
fi

if [ "$dry_run" -eq 1 ]; then
  echo "would create $created tag(s), $skipped existing (dry run: nothing created)."
else
  echo "created $created tag(s), skipped $skipped existing."
fi
if [ "$push" -eq 1 ]; then
  refs=()
  for t in "${all_tags[@]}"; do
    refs+=("refs/tags/$t")
  done
  git push --atomic origin "${refs[@]}"
  echo "pushed ${#refs[@]} tag(s) atomically to origin."
else
  echo "To publish all-or-nothing, run: $0 --push $ver"
  echo "(equivalent: git push --atomic origin <every $ver tag>; never use git push --tags)"
fi
