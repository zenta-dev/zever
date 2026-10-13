#!/bin/sh
# installer-test.sh - offline tests for the fail-closed verification in
# install.sh / install.ps1 (hardening plan P2-9).
#
# Everything runs offline: the downloader is a stub curl that serves a local
# fixture "release", `go` is a stub, and PATH is an isolated dir with symlinks
# to just the system tools the installer needs. No sha256sum/shasum symlink =
# an environment where checksum verification is impossible.
#
# Usage: sh tools/installer-test.sh
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMPD="$(mktemp -d)"
trap 'rm -rf "$TMPD"' EXIT INT TERM

PASS=0
FAIL=0
SKIP=0

say() { printf '%s\n' "$*" >&2; }
ok() { say "ok - $1"; PASS=$((PASS + 1)); }
bad() { say "FAIL - $1"; FAIL=$((FAIL + 1)); }
skip() { say "skip - $1"; SKIP=$((SKIP + 1)); }

# --- assertion helpers ---------------------------------------------------------
assert_rc() {
    # assert_rc DESC EXPECTED_RC ACTUAL_RC
    if [ "$2" -eq "$3" ]; then ok "$1"; else bad "$1 (want rc=$2, got rc=$3)"; fi
}

assert_contains() {
    # assert_contains DESC HAYSTACK NEEDLE
    case "$2" in
        *"$3"*) ok "$1" ;;
        *) bad "$1 (output does not contain: $3)" ;;
    esac
}

assert_not_contains() {
    # assert_not_contains DESC HAYSTACK NEEDLE
    case "$2" in
        *"$3"*) bad "$1 (output unexpectedly contains: $3)" ;;
        *) ok "$1" ;;
    esac
}

# --- isolated PATH -------------------------------------------------------------
STUB="$TMPD/stub"
mkdir -p "$STUB"

link_tool() {
    # link_tool NAME: symlink the system tool into the stub PATH, if present.
    _p="$(command -v "$1" 2>/dev/null)" || return 0
    ln -s "$_p" "$STUB/$1"
}

for _t in sh uname date wc tr awk cut grep mkdir mktemp rm chmod mv cat basename pwd head cp; do
    link_tool "$_t"
done

# sha256sum is linked into the stub PATH only for tests that exercise real
# verification; its absence models an environment where verification is
# impossible.
HAVE_SHA256=0
link_sha256() {
    if [ "$HAVE_SHA256" -eq 0 ] && command -v sha256sum >/dev/null 2>&1; then
        link_tool sha256sum
        HAVE_SHA256=1
    fi
}
unlink_sha256() {
    rm -f "$STUB/sha256sum"
    HAVE_SHA256=0
}

# --- stub curl: serve the fixture release --------------------------------------
FIXTURES="$TMPD/fixtures"
RELEASE_DIR="$FIXTURES/releases/download/v0.6.1"
mkdir -p "$RELEASE_DIR"

cat >"$STUB/curl" <<'EOF'
#!/bin/sh
# stub curl: copy $STUB_RELEASE_DIR/<path> to the -o destination.
dest=""
url=""
prev=""
for a in "$@"; do
    if [ "$prev" = "-o" ]; then dest="$a"; fi
    case "$a" in http://*|https://*) url="$a" ;;
    esac
    prev="$a"
done
if [ -z "$dest" ] || [ -z "$url" ]; then
    echo "stub curl: cannot parse args: $*" >&2
    exit 1
fi
rel="${url#*releases/download/}"
rel="${rel#*/}"
src="$STUB_RELEASE_DIR/$rel"
if [ ! -f "$src" ]; then
    echo "stub curl: no fixture for $url" >&2
    exit 1
fi
cp "$src" "$dest"
EOF
chmod +x "$STUB/curl"

# --- stub go: version ok, every other subcommand succeeds ----------------------
cat >"$STUB/go" <<'EOF'
#!/bin/sh
case "$1" in
    version) echo "go version go1.27.0 linux/amd64" ;;
    *) exit 0 ;;
esac
EOF
chmod +x "$STUB/go"

# --- fake assets ---------------------------------------------------------------
# zever answers -V / --help / new so the smoke test passes against stubs.
cat >"$RELEASE_DIR/zever-linux-amd64" <<'EOF'
#!/bin/sh
case "$1" in
    -V) echo "zever v0.6.1" ;;
    --help) exit 0 ;;
    new)
        _dir="."
        while [ $# -gt 0 ]; do
            if [ "$1" = "--dir" ]; then _dir="$2"; shift 2; else shift; fi
        done
        mkdir -p "$_dir"
        ;;
    *) exit 0 ;;
esac
EOF
chmod +x "$RELEASE_DIR/zever-linux-amd64"
echo "fake lsp" >"$RELEASE_DIR/zever-lsp-linux-amd64"
chmod +x "$RELEASE_DIR/zever-lsp-linux-amd64"

# Correct checksums by default; mismatch test overwrites SHA256SUMS.txt.
make_sums() {
    # make_sums [BAD] - write SHA256SUMS.txt for the fixture assets.
    if [ "${1:-}" = "BAD" ]; then
        printf '%s  zever-linux-amd64\n' "$(printf '0%.0s' $(seq 1 64))" >"$RELEASE_DIR/SHA256SUMS.txt"
        printf '%s  zever-lsp-linux-amd64\n' "$(printf '0%.0s' $(seq 1 64))" >>"$RELEASE_DIR/SHA256SUMS.txt"
    else
        (cd "$RELEASE_DIR" && sha256sum zever-linux-amd64 zever-lsp-linux-amd64) >"$RELEASE_DIR/SHA256SUMS.txt"
    fi
}
make_sums

# --- run installer in the stub environment -------------------------------------
run_install_sh() {
    # run_install_sh [ENV=V ...] -- [args...]
    # Prints the captured output on stdout, sets RC. Output goes to stderr so
    # test progress stays readable.
    _envs=""
    while [ $# -gt 0 ]; do
        [ "$1" = "--" ] && { shift; break; }
        _envs="$_envs $1"
        shift
    done
    _out="$TMPD/last_out"
    env -i \
        PATH="$STUB" \
        HOME="$TMPD/home" \
        NO_COLOR=1 \
        STUB_RELEASE_DIR="$RELEASE_DIR" \
        $_envs \
        sh "$ROOT/install.sh" "$@" >"$_out" 2>&1
    RC=$?
    cat "$_out"
    exit "$RC"
}

# --- install.sh tests ----------------------------------------------------------

say ""
say "install.sh: fail-closed verification"

# 1. No sha256 tool, no escape hatch: must fail with a clear message.
make_sums
_out="$(run_install_sh -- --yes)"; _rc=$?
assert_rc "no-sha256 fails" 1 "$_rc"
assert_contains "no-sha256 message names the missing tool" "$_out" "sha256sum or shasum"
assert_contains "no-sha256 message offers escape hatch" "$_out" "--insecure-skip-verify"

# 2. Escape hatch via env var: proceeds (warning expected, install succeeds).
_out="$(run_install_sh ZEVER_INSTALL_INSECURE=1 -- --yes)"; _rc=$?
assert_rc "env escape hatch proceeds" 0 "$_rc"
assert_contains "env escape hatch warns" "$_out" "insecure"

# 3. Escape hatch via flag: proceeds.
_out="$(run_install_sh -- --yes --insecure-skip-verify)"; _rc=$?
assert_rc "flag escape hatch proceeds" 0 "$_rc"
assert_contains "flag escape hatch warns" "$_out" "insecure"

# 4. Checksum mismatch with a working sha256 tool: must fail, never install.
link_sha256
make_sums BAD
_out="$(run_install_sh -- --yes)"; _rc=$?
assert_rc "sha256 mismatch fails" 1 "$_rc"
assert_contains "mismatch message" "$_out" "SHA256 mismatch"
assert_not_contains "mismatch does not install" "$_out" "installed $TMPD/home/.local/bin/zever"

# 5. Happy path: correct checksums verify and install.
make_sums
_out="$(run_install_sh -- --yes)"; _rc=$?
assert_rc "happy path proceeds" 0 "$_rc"
assert_contains "happy path verifies" "$_out" "verified zever-linux-amd64"
assert_contains "happy path installs" "$_out" "installed $TMPD/home/.local/bin/zever"

# 6. gh absent: the manual attestation command is printed.
_out="$(run_install_sh -- --yes)"; _rc=$?
assert_rc "gh-absent path proceeds" 0 "$_rc"
assert_contains "gh-absent prints manual command" "$_out" "gh attestation verify"
assert_contains "gh-absent names the repo" "$_out" "zenta-dev/zever"

# 7. Dry run without a sha256 tool also fails closed.
unlink_sha256
_out="$(run_install_sh -- --yes --dry-run)"; _rc=$?
assert_rc "dry-run without sha256 fails" 1 "$_rc"
assert_contains "dry-run fail-closed message" "$_out" "sha256sum or shasum"

# 8. Dry run with escape hatch lists the verification steps.
_out="$(run_install_sh ZEVER_INSTALL_INSECURE=1 -- --yes --dry-run)"; _rc=$?
assert_rc "dry-run with escape hatch proceeds" 0 "$_rc"
assert_contains "dry-run lists checksum verification" "$_out" "verify SHA256 against SHA256SUMS.txt"

# 9. New flag is documented in --help.
_out="$(run_install_sh -- --help)"; _rc=$?
assert_rc "help exits 0" 0 "$_rc"
assert_contains "help documents flag" "$_out" "--insecure-skip-verify"
assert_contains "help documents env var" "$_out" "ZEVER_INSTALL_INSECURE"

# --- install.ps1 structural tests (behavioral tests need pwsh) ------------------

say ""
say "install.ps1: fail-closed parity (static checks)"

PS1="$ROOT/install.ps1"

_out="$(sed -n 's/^\s*//p' "$PS1")"
assert_contains "ps1 has switch" "$_out" "InsecureSkipVerify"
assert_contains "ps1 honors env var" "$_out" 'ZEVER_INSTALL_INSECURE'
assert_contains "ps1 prints manual attestation command" "$_out" "gh attestation verify"
assert_contains "ps1 guards skip behind flag" "$_out" 'if ($InsecureSkipVerify)'
assert_contains "ps1 fails closed when hashing fails" "$_out" "could not hash"

if command -v pwsh >/dev/null 2>&1; then
    say ""
    say "install.ps1: pwsh behavioral tests"
    skip "ps1 behavioral tests (not yet wired: install.ps1 is Windows-only)"
else
    skip "install.ps1 behavioral tests (pwsh not installed)"
fi

# --- summary -------------------------------------------------------------------
say ""
say "installer tests: $PASS passed, $FAIL failed, $SKIP skipped"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
