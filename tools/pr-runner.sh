#!/usr/bin/env bash
#
# pr-runner.sh — commit, push, open/update a PR, watch CI, and enable squash
# auto-merge when the gate is green.
#
# One invocation == one attempt. On red, the caller (subagent) inspects the
# failing checks, fixes the code, and re-invokes. This keeps CI waiting off the
# main thread: the worker owns the watch/fix loop.
#
# Usage:
#   tools/pr-runner.sh <branch> <commit-message> <pr-title> <pr-body> [watch-timeout]
#
# Exit codes:
#   0  gate green, auto-merge enabled (or already merging)
#   2  checks failed (fix and re-run)
#   1  usage / infrastructure error
#
set -euo pipefail

if [ "$#" -lt 4 ]; then
  echo "usage: $0 <branch> <commit-message> <pr-title> <pr-body> [watch-timeout]" >&2
  exit 1
fi

BRANCH="$1"
MSG="$2"
TITLE="$3"
BODY="$4"
WATCH_TIMEOUT="${5:-3600}"

REPO="$(gh repo view --json nameWithOwner --jq .nameWithOwner)"
CURRENT="$(git branch --show-current)"

if [ "$CURRENT" != "$BRANCH" ]; then
  if git show-ref --verify --quiet "refs/heads/$BRANCH"; then
    git switch "$BRANCH"
  else
    git switch -c "$BRANCH"
  fi
fi

# Ensure owner identity (never amend/overwrite an existing config).
git config user.name  >/dev/null 2>&1 || git config user.name  "Rahmat Hidayatullah"
git config user.email >/dev/null 2>&1 || git config user.email "bokirsianpar95@gmail.com"

if ! git diff --quiet || ! git diff --cached --quiet || [ -n "$(git status --porcelain)" ]; then
  git add -A
  git commit -m "$MSG"
fi

git push -u origin "$BRANCH"

if ! gh pr view "$BRANCH" --repo "$REPO" >/dev/null 2>&1; then
  gh pr create --repo "$REPO" --base main --head "$BRANCH" \
    --title "$TITLE" --body "$BODY"
fi

# Wait for check runs to register after a fresh push.
for _ in $(seq 1 30); do
  count="$(gh pr checks "$BRANCH" --repo "$REPO" --json name --jq 'length' 2>/dev/null || echo 0)"
  [ "$count" -gt 0 ] && break
  sleep 5
done

set +e
timeout "$WATCH_TIMEOUT" gh pr checks "$BRANCH" --repo "$REPO" --watch --fail-fast
watch_rc=$?
set -e

# Re-read the final conclusion set.
summary="$(gh pr checks "$BRANCH" --repo "$REPO" --json name,state,bucket \
  --jq '[.[] | {name, bucket}]' 2>/dev/null || echo '[]')"
failing="$(printf '%s' "$summary" | jq -r '.[] | select(.bucket=="fail" or .bucket=="cancel") | .name' 2>/dev/null || true)"

if [ -n "$failing" ]; then
  echo "pr-runner: checks FAILED for $BRANCH:" >&2
  echo "$failing" >&2
  exit 2
fi

if [ "$watch_rc" -ne 0 ]; then
  echo "pr-runner: watch exited $watch_rc with no failing bucket yet; re-run to continue" >&2
  exit 2
fi

echo "pr-runner: gate green for $BRANCH, enabling squash auto-merge"
gh pr merge "$BRANCH" --repo "$REPO" --squash --auto --delete-branch || {
  echo "pr-runner: auto-merge request failed; PR left open" >&2
  exit 1
}

echo "pr-runner: auto-merge enabled"
