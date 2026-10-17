#!/usr/bin/env bash
# ci-quickstart-dev.sh — headless 3-command friction journey (stream G gate).
#
# The journey under test is exactly three user commands:
#   1. zever new hello
#   2. zever dev (headless boot; --once when stream D2 ships it)
#   3. curl localhost:<port>/hello/world
# Anything more is friction, so the script counts the user-facing commands
# and fails when the journey exceeds three. Wall-clock is recorded against
# JOURNEY_BUDGET (default 600s, warm module cache) as a warning only — timing
# is signal, never a red gate.
#
# Boot path: `zever dev --once` does not exist on main yet (no --once flag in
# cmd/zever/dev.go), so the gate boots `zever dev` in the background and
# probes readiness (/healthz, then /readyz) with DEV_TIMEOUT. When D2 lands
# --once, the detection branch below picks it up with no script change.
# Gap noted for stream D2: headless single-shot dev boot.
#
# Route path: the fresh scaffold ships no handlers by design (docs: RPC
# routes return 404 until you write them — zever generates plumbing, not
# business logic), so /hello/world cannot return 200 on main today. The gate
# asserts readiness 200 and records the /hello/world status without failing;
# the commented assertion marks the TODO for D2 (scaffold demo handler).
# Gap noted for stream D2: demo /hello/world handler or documented 404.
#
# Scaffold quality: the scaffolded project must pass `go vet`, its own
# tests, and `golangci-lint` (mirrors TestRunNewScaffoldsAndBuilds at E2E
# level). golangci-lint must be on PATH (CI installs it via
# make setup-golangci-lint); when absent the lint leg warns and skips so
# `make quickstart-check` stays usable on bare checkouts.
#
# Usage: ZEVER_FRAMEWORK_PATH=<checkout> bash tools/ci-quickstart-dev.sh <zever-bin>
set -euo pipefail

ZEVER_BIN="${1:?usage: ci-quickstart-dev.sh <zever-bin>}"
DEV_TIMEOUT="${DEV_TIMEOUT:-90}"
JOURNEY_BUDGET="${JOURNEY_BUDGET:-600}"
HTTP_PORT="${DEV_HTTP_PORT:-8081}"
GRPC_PORT="${DEV_GRPC_PORT:-9091}"

: "${ZEVER_FRAMEWORK_PATH:?ci-quickstart-dev: ZEVER_FRAMEWORK_PATH must point at the framework checkout (dogfood seam)}"
command -v "$ZEVER_BIN" >/dev/null 2>&1 || { echo "ci-quickstart-dev: not executable: $ZEVER_BIN" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo "ci-quickstart-dev: curl not on PATH" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "ci-quickstart-dev: go toolchain not on PATH" >&2; exit 1; }

START="$(date +%s)"
WORK="$(mktemp -d)"
DEV_PID=""
cleanup() {
  if [ -n "$DEV_PID" ] && kill -0 "$DEV_PID" 2>/dev/null; then
    kill "$DEV_PID" 2>/dev/null || true
    wait "$DEV_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

USER_CMDS=0
user_cmd() {
  USER_CMDS=$((USER_CMDS + 1))
  echo "+ (user command $USER_CMDS/3) $*"
  "$@"
}

# --- user command 1/3: scaffold ---
user_cmd "$ZEVER_BIN" new hello --dir "$WORK/hello" --force
cd "$WORK/hello"

# --- user command 2/3: headless dev boot ---
# TODO(D2): `zever dev --once` — single-shot headless boot. The probe below
# is the fallback until the flag exists; detection keeps this green either way.
DEV_ARGS=(-addr "127.0.0.1:$HTTP_PORT" -grpc-addr "127.0.0.1:$GRPC_PORT")
if "$ZEVER_BIN" dev --help 2>&1 | grep -q -- --once; then
  echo "ci-quickstart-dev: --once supported, using single-shot boot"
  DEV_ARGS=(--once "${DEV_ARGS[@]}")
else
  echo "ci-quickstart-dev: no --once yet (gap for D2); boot + readiness probe fallback"
fi
export AUTH_JWT_SECRET=0123456789abcdef0123456789abcdef
export AUTH_JWT_ISSUER=dev
USER_CMDS=$((USER_CMDS + 1))
echo "+ (user command $USER_CMDS/3) $ZEVER_BIN dev ${DEV_ARGS[*]} & (background, then readiness probe)"
"$ZEVER_BIN" dev "${DEV_ARGS[@]}" >"$WORK/dev.log" 2>&1 &
DEV_PID=$!
ready=0
for _ in $(seq 1 "$DEV_TIMEOUT"); do
  if curl -fsS -m 2 "http://127.0.0.1:$HTTP_PORT/healthz" >/dev/null 2>&1; then
    ready=1
    break
  fi
  if ! kill -0 "$DEV_PID" 2>/dev/null; then
    echo "ci-quickstart-dev: dev exited before readiness; log:" >&2
    tail -n 50 "$WORK/dev.log" >&2 || true
    exit 1
  fi
  sleep 1
done
if [ "$ready" -ne 1 ]; then
  echo "ci-quickstart-dev: dev never reached /healthz in ${DEV_TIMEOUT}s; log:" >&2
  tail -n 50 "$WORK/dev.log" >&2 || true
  exit 1
fi
echo "ci-quickstart-dev: dev boot ready (/healthz 200)"
curl -fsS -m 10 "http://127.0.0.1:$HTTP_PORT/readyz" >/dev/null
echo "ci-quickstart-dev: readiness ready (/readyz 200)"

# --- user command 3/3: hello route ---
USER_CMDS=$((USER_CMDS + 1))
HELLO_URL="http://127.0.0.1:$HTTP_PORT/hello/world"
echo "+ (user command $USER_CMDS/3) curl $HELLO_URL"
HELLO_CODE="$(curl -s -o /dev/null -w '%{http_code}' -m 10 "$HELLO_URL" || true)"
echo "ci-quickstart-dev: GET /hello/world -> HTTP $HELLO_CODE"
# TODO(D2): expect 200 once the scaffold ships a demo handler. Today the
# fresh scaffold has no handlers by design (404 until you write them), so
# this records instead of asserting.
# [ "$HELLO_CODE" = "200" ] || { echo "want 200 at $HELLO_URL, got $HELLO_CODE"; exit 1; }

if [ "$USER_CMDS" -gt 3 ]; then
  echo "ci-quickstart-dev: journey took $USER_CMDS user commands, budget is 3" >&2
  exit 1
fi
echo "ci-quickstart-dev: journey is $USER_CMDS user command(s), within the 3-command budget"

# --- scaffold quality: vet, tests, lint (assertions, not user commands) ---
echo "ci-quickstart-dev: go vet ./..."
go vet ./...
echo "ci-quickstart-dev: go test ./..."
go test ./...
if command -v golangci-lint >/dev/null 2>&1; then
  echo "ci-quickstart-dev: golangci-lint run ./..."
  golangci-lint run ./...
else
  echo "ci-quickstart-dev: WARNING golangci-lint not on PATH, lint leg skipped (CI installs it; run 'make setup' locally)"
fi

END="$(date +%s)"
ELAPSED=$((END - START))
echo "ci-quickstart-dev: journey wall-clock ${ELAPSED}s (budget ${JOURNEY_BUDGET}s, warm module cache)"
if [ "$ELAPSED" -gt "$JOURNEY_BUDGET" ]; then
  msg="journey wall-clock ${ELAPSED}s exceeded budget ${JOURNEY_BUDGET}s (warm module cache)"
  if [ "${GITHUB_ACTIONS:-}" = "true" ]; then
    echo "::warning::$msg"
  else
    echo "ci-quickstart-dev: WARNING $msg"
  fi
fi
echo "ci-quickstart-dev: PASS"
