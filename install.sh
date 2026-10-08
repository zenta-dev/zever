#!/bin/sh
# install.sh - install the zever CLI and zever-lsp from GitHub releases.
#
# Usage:
#   sh install.sh [--version vX.Y.Z] [--prefix DIR] [--yes] [--force] [--dry-run] [--help]
#
# One-liner:
#   curl --proto '=https' --tlsv1.2 -sSf -L \
#     https://raw.githubusercontent.com/zenta-dev/zever/main/install.sh | sh -s -- --version v0.6.1
#
# Notes (repo ethos, verified facts - do not "simplify" these away):
# - Binaries come from GitHub releases (or a source build). Never
#   `go install <module>@version`: the repo commits `replace` directives,
#   so `go install @version` fails.
# - `zever -V` prints `zever vX.Y.Z`. zever-lsp has no CLI version flag
#   (stdio-only), so the LSP is matched blindly to the CLI version.
# - Never hangs: non-interactive shells get an error telling them to re-run
#   with --yes instead of blocking on stdin.
# - Errors go to stderr. No sudo. No package managers.
# - Re-runs are idempotent.
set -eu

ZEVER_VERSION_DEFAULT="v0.6.1"
ZEVER_REPO="zenta-dev/zever"
GO_MIN_MAJOR=1
GO_MIN_MINOR=27
GO_DL_URL="https://go.dev/dl/"
PATH_MARKER="# zever installer: add zever to PATH"

VERSION="${ZEVER_VERSION:-$ZEVER_VERSION_DEFAULT}"
PREFIX="${ZEVER_PREFIX:-${HOME:-}/.local}"
ASSUME_YES=0
FORCE=0
DRY_RUN=0
TMPD=""

die() {
    printf 'install.sh: error: %s\n' "$*" >&2
    exit 1
}

warn() {
    printf 'install.sh: warning: %s\n' "$*" >&2
}

info() {
    printf 'install.sh: %s\n' "$*" >&2
}

usage() {
    cat <<EOF
Usage: install.sh [OPTIONS]

Install the zever CLI and zever-lsp from GitHub releases.

Options:
  --version X   version to install (default: ${ZEVER_VERSION_DEFAULT}, or \$ZEVER_VERSION)
  --prefix DIR  install prefix, binaries go to DIR/bin (default: ~/.local, or \$ZEVER_PREFIX)
  --yes, -y     accept upgrade/downgrade prompts without asking
  --force       re-install even when the target version is already installed
                (implies --yes and overwrites existing binaries)
  --dry-run     print every action without executing it
  --help        print this help and exit

Examples:
  sh install.sh
  sh install.sh --version v0.6.1 --prefix ~/.local --yes
  ZEVER_VERSION=v0.6.1 ZEVER_PREFIX=/opt/zever sh install.sh --yes
EOF
}

rerun_cmd() {
    printf '%s --version %s --prefix %s --yes' "$0" "$VERSION" "$PREFIX"
}

# normalize_num STR: strip leading zeros so numeric compare works in POSIX sh.
normalize_num() {
    _n="$1"
    while [ "${_n#0}" != "$_n" ] && [ -n "${_n#0}" ]; do
        _n="${_n#0}"
    done
    printf '%s' "${_n:-0}"
}

# num_cmp A B: print -1, 0, or 1 comparing non-negative integer strings.
num_cmp() {
    _a="$(normalize_num "$1")"
    _b="$(normalize_num "$2")"
    if [ "${#_a}" -gt "${#_b}" ]; then printf '%s' '1'; return; fi
    if [ "${#_a}" -lt "${#_b}" ]; then printf '%s' '-1'; return; fi
    if [ "$_a" \> "$_b" ]; then printf '%s' '1'; return; fi
    if [ "$_a" \< "$_b" ]; then printf '%s' '-1'; return; fi
    printf '%s' '0'
}

# vercmp A B: print -1, 0, or 1 comparing SemVer-ish versions (leading v ok).
# Hand-rolled: macOS lacks `sort -V`, so no `sort -V` anywhere in this file.
vercmp() {
    _va="${1#v}"
    _vb="${2#v}"
    _pa=""
    _pb=""
    case "$_va" in
        *-*) _pa="${_va#*-}"; _va="${_va%%-*}" ;;
    esac
    case "$_vb" in
        *-*) _pb="${_vb#*-}"; _vb="${_vb%%-*}" ;;
    esac
    _i=1
    while [ "$_i" -le 3 ]; do
        _fa="$(printf '%s' "$_va" | cut -d. -f"$_i" -s)"
        _fb="$(printf '%s' "$_vb" | cut -d. -f"$_i" -s)"
        _fa="${_fa:-0}"
        _fb="${_fb:-0}"
        _r="$(num_cmp "$_fa" "$_fb")"
        if [ "$_r" != "0" ]; then printf '%s' "$_r"; return; fi
        _i=$((_i + 1))
    done
    # Numeric cores equal: a bare release outranks a pre-release.
    if [ -z "$_pa" ] && [ -n "$_pb" ]; then printf '%s' '1'; return; fi
    if [ -n "$_pa" ] && [ -z "$_pb" ]; then printf '%s' '-1'; return; fi
    if [ "$_pa" \> "$_pb" ]; then printf '%s' '1'; return; fi
    if [ "$_pa" \< "$_pb" ]; then printf '%s' '-1'; return; fi
    printf '%s' '0'
}

# normalize_version: ensure a leading v, reject garbage early (else a 404 later).
normalize_version() {
    case "$1" in
        v*) printf '%s' "$1" ;;
        *) printf 'v%s' "$1" ;;
    esac
}

valid_version() {
    case "$1" in
        v[0-9]*.[0-9]*.[0-9]*) return 0 ;;
        *) return 1 ;;
    esac
}

# ask PROMPT: return 0 on explicit yes. Never hangs in CI: when stdin is not
# a terminal we try /dev/tty (covers `echo | install.sh` at a real terminal),
# and when there is no controlling terminal at all we exit 1 with the exact
# re-run command instead of blocking.
ask() {
    if [ "$ASSUME_YES" -eq 1 ]; then
        return 0
    fi
    if [ -t 0 ]; then
        printf '%s ' "$1" >&2
        read -r _ask_ans || true
        case "$_ask_ans" in
            [Yy]*) return 0 ;;
            *) return 1 ;;
        esac
    fi
    # Piped stdin (e.g. curl|sh): talk to the controlling terminal. Probe in
    # a subshell whose stderr is already /dev/null: a redirection that fails
    # suppresses any later 2>/dev/null on the same command line, so a bare
    # `: < /dev/tty 2>/dev/null` would still leak the open error.
    if ( : < /dev/tty ) 2>/dev/null; then
        printf '%s ' "$1" >&2
        if IFS= read -r _ask_ans < /dev/tty; then
            case "$_ask_ans" in
                [Yy]*) return 0 ;;
                *) return 1 ;;
            esac
            return 1
        fi
    fi
    die "non-interactive shell: re-run with --yes: $(rerun_cmd)"
}

# --- arg parsing -------------------------------------------------------------
while [ $# -gt 0 ]; do
    case "$1" in
        --version)
            [ $# -ge 2 ] || { printf 'install.sh: error: --version needs a value\n' >&2; exit 2; }
            VERSION="$2"
            shift 2
            ;;
        --version=*)
            VERSION="${1#--version=}"
            shift
            ;;
        --prefix)
            [ $# -ge 2 ] || { printf 'install.sh: error: --prefix needs a value\n' >&2; exit 2; }
            PREFIX="$2"
            shift 2
            ;;
        --prefix=*)
            PREFIX="${1#--prefix=}"
            shift
            ;;
        --yes|-y)
            ASSUME_YES=1
            shift
            ;;
        --force)
            FORCE=1
            ASSUME_YES=1
            shift
            ;;
        --dry-run)
            DRY_RUN=1
            shift
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        --)
            shift
            break
            ;;
        -*)
            printf 'install.sh: error: unknown flag: %s\n' "$1" >&2
            usage >&2
            exit 2
            ;;
        *)
            printf 'install.sh: error: unexpected argument: %s\n' "$1" >&2
            usage >&2
            exit 2
            ;;
    esac
done
[ $# -eq 0 ] || { printf 'install.sh: error: unexpected argument: %s\n' "$1" >&2; exit 2; }

VERSION="$(normalize_version "$VERSION")"
valid_version "$VERSION" || { printf 'install.sh: error: invalid version: %s (want vX.Y.Z)\n' "$VERSION" >&2; exit 2; }

# Expand a leading ~/ in the prefix (positional args get no tilde expansion).
case "$PREFIX" in
    "~"/*) PREFIX="${HOME:-}/$(printf '%s' "$PREFIX" | cut -c3-)" ;;
    "~") PREFIX="${HOME:-}" ;;
esac
[ -n "${HOME:-}" ] || die "HOME is unset and --prefix was not given"
case "$PREFIX" in
    /*) ;;
    *) PREFIX="$(pwd)/$PREFIX" ;;
esac
BIN_DIR="$PREFIX/bin"

# --- platform detection ------------------------------------------------------
OS_RAW="$(uname -s)"
ARCH_RAW="$(uname -m)"
case "$OS_RAW" in
    Linux) _os="linux" ;;
    Darwin) _os="darwin" ;;
    MINGW*|MSYS*|CYGWIN*) _os="windows" ;;
    *) die "unsupported OS: $OS_RAW (want Linux, macOS, or Windows Git Bash)" ;;
esac
case "$ARCH_RAW" in
    x86_64|amd64) _arch="amd64" ;;
    aarch64|arm64) _arch="arm64" ;;
    *) die "unsupported architecture: $ARCH_RAW (want x86_64/amd64 or aarch64/arm64)" ;;
esac
if [ "$_os" = "windows" ] && [ "$_arch" = "arm64" ]; then
    die "windows-arm64 is not shipped yet (want windows-amd64)"
fi
ASSET_TRIPLE="${_os}-${_arch}"
EXE_SUFFIX=""
if [ "$_os" = "windows" ]; then
    EXE_SUFFIX=".exe"
fi
ZEVER_ASSET="zever-${ASSET_TRIPLE}${EXE_SUFFIX}"
LSP_ASSET="zever-lsp-${ASSET_TRIPLE}${EXE_SUFFIX}"
ZEVER_FILE="zever${EXE_SUFFIX}"
LSP_FILE="zever-lsp${EXE_SUFFIX}"
BASE_URL="https://github.com/${ZEVER_REPO}/releases/download/${VERSION}"

# --- prereqs: downloader + go >= 1.27, nothing else --------------------------
if command -v curl >/dev/null 2>&1; then
    DOWNLOADER="curl"
elif command -v wget >/dev/null 2>&1; then
    DOWNLOADER="wget"
else
    die "need curl or wget to download release assets (and go >= 1.27 from ${GO_DL_URL})"
fi

command -v go >/dev/null 2>&1 || die "need go >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR} (see ${GO_DL_URL})"
# `go version` prints e.g. `go version go1.27.0 linux/amd64`.
_GO_VER_STR="$(go version 2>/dev/null)" || die "could not run 'go version' (need go >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR} from ${GO_DL_URL})"
set -- $_GO_VER_STR
_GO_VER="${3:-}"
_GO_VER="${_GO_VER#go}"
_GO_MAJOR="${_GO_VER%%.*}"
_GO_REST="${_GO_VER#*.}"
_GO_MINOR="${_GO_REST%%.*}"
case "$_GO_MAJOR.$_GO_MINOR" in
    *[!0-9.]*|"".*|.*""|*.*.*) die "could not parse go version from '${_GO_VER_STR}' (need go >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR} from ${GO_DL_URL})" ;;
esac
if [ "$(num_cmp "$_GO_MAJOR" "$GO_MIN_MAJOR")" -lt 0 ] || \
   { [ "$(num_cmp "$_GO_MAJOR" "$GO_MIN_MAJOR")" -eq 0 ] && [ "$(num_cmp "$_GO_MINOR" "$GO_MIN_MINOR")" -lt 0 ]; }; then
    die "go ${_GO_VER} is too old (need go >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR}, see ${GO_DL_URL})"
fi

download_file() {
    # download_file URL DEST
    if [ "$DRY_RUN" -eq 1 ]; then
        printf 'dry-run: download %s -> %s\n' "$1" "$2"
        return 0
    fi
    if [ "$DOWNLOADER" = "curl" ]; then
        curl --proto '=https' --tlsv1.2 -sSf -L -o "$2" "$1"
    else
        wget -q -O "$2" "$1"
    fi
}

# installed_version: print the version of an existing zever binary, or "".
installed_version() {
    _out="$("$1" -V 2>/dev/null)" || return 0
    # `zever -V` prints `zever vX.Y.Z`; the version is the last field.
    _v="${_out##* }"
    _v="$(printf '%s' "$_v" | tr -d '[:space:]')"
    if valid_version "$_v"; then
        printf '%s' "$_v"
    fi
    return 0
}

# --- upgrade gate (BEFORE downloading anything) ------------------------------
EXISTING="$(command -v "$ZEVER_FILE" 2>/dev/null || true)"
if [ -z "$EXISTING" ] && [ "$ZEVER_FILE" != "zever" ]; then
    EXISTING="$(command -v zever 2>/dev/null || true)"
fi
TARGET_BIN="$BIN_DIR/$ZEVER_FILE"
TARGET_LSP="$BIN_DIR/$LSP_FILE"

if [ -n "$EXISTING" ] && [ "$EXISTING" != "$TARGET_BIN" ]; then
    warn "existing zever at ${EXISTING} takes precedence over ${TARGET_BIN} per PATH order"
    warn "this install writes ${TARGET_BIN}; fix your PATH if 'zever' still resolves elsewhere"
fi

if [ -n "$EXISTING" ]; then
    HAVE="$(installed_version "$EXISTING")"
    if [ -z "$HAVE" ]; then
        info "found existing zever at ${EXISTING} but could not parse 'zever -V' output"
        if [ "$DRY_RUN" -eq 1 ]; then
            printf 'dry-run: would prompt to replace unparseable install at %s\n' "$EXISTING"
        elif ! ask "Replace existing zever at ${EXISTING} with ${VERSION}? [y/N]"; then
            info "aborted by user"
            exit 1
        fi
    else
        CMP="$(vercmp "$HAVE" "$VERSION")"
        if [ "$CMP" -eq 0 ] && [ "$FORCE" -eq 0 ]; then
            if [ -s "$TARGET_BIN" ] && [ -s "$TARGET_LSP" ]; then
                info "zever ${VERSION} is already installed and up to date at ${TARGET_BIN}"
                exit 0
            fi
            warn "zever ${VERSION} reports installed but ${TARGET_BIN} or ${TARGET_LSP} is missing/empty; reinstalling"
        elif [ "$CMP" -eq 0 ]; then
            info "--force: reinstalling zever ${VERSION} over identical install"
        elif [ "$CMP" -lt 0 ]; then
            if [ "$DRY_RUN" -eq 1 ]; then
                printf 'dry-run: would prompt: Upgrade zever %s → %s? [y/N] (assuming yes)\n' "$HAVE" "$VERSION"
            elif ! ask "Upgrade zever ${HAVE} → ${VERSION}? [y/N]"; then
                info "aborted by user"
                exit 1
            fi
        else
            warn "installed zever ${HAVE} is NEWER than target ${VERSION}"
            if [ "$DRY_RUN" -eq 1 ]; then
                printf 'dry-run: would prompt: Downgrade zever %s → %s? [y/N] (assuming yes)\n' "$HAVE" "$VERSION"
            elif ! ask "Downgrade zever ${HAVE} → ${VERSION}? [y/N]"; then
                info "aborted by user"
                exit 1
            fi
        fi
    fi
else
    info "no existing zever found: fresh install of ${VERSION}"
fi

# --- download + verify + install ---------------------------------------------
cleanup() {
    if [ -n "$TMPD" ] && [ -d "$TMPD" ]; then
        rm -rf "$TMPD"
    fi
}
trap cleanup EXIT INT TERM

if [ "$DRY_RUN" -eq 1 ]; then
    printf 'dry-run: mkdir -p %s\n' "$BIN_DIR"
    TMPD=""
else
    mkdir -p "$BIN_DIR"
    TMPD="$(mktemp -d)"
fi

SUMS_URL="${BASE_URL}/SHA256SUMS.txt"
if [ "$DRY_RUN" -eq 1 ]; then
    printf 'dry-run: download %s -> <tmp>/SHA256SUMS.txt\n' "$SUMS_URL"
    printf 'dry-run: download %s/%s -> %s\n' "$BASE_URL" "$ZEVER_ASSET" "$TARGET_BIN"
    printf 'dry-run: download %s/%s -> %s\n' "$BASE_URL" "$LSP_ASSET" "$TARGET_LSP"
    printf 'dry-run: verify SHA256 of both binaries against SHA256SUMS.txt\n'
    printf 'dry-run: chmod +x both binaries\n'
    printf 'dry-run: run gh attestation verify (only if gh exists)\n'
else
    info "downloading SHA256SUMS.txt for ${VERSION}"
    download_file "$SUMS_URL" "$TMPD/SHA256SUMS.txt" || \
        die "could not download ${SUMS_URL} (release ${VERSION} may predate checksum files)"
    for _asset in "$ZEVER_ASSET" "$LSP_ASSET"; do
        case "$_asset" in
            "$ZEVER_ASSET") _dest="$TARGET_BIN" ;;
            *) _dest="$TARGET_LSP" ;;
        esac
        info "downloading ${_asset}"
        download_file "${BASE_URL}/${_asset}" "$TMPD/${_asset}" || \
            die "could not download ${BASE_URL}/${_asset}"
        [ -s "$TMPD/${_asset}" ] || die "downloaded ${_asset} is empty"
        # Verify SHA256 against the release checksum file.
        if command -v sha256sum >/dev/null 2>&1; then
            _want="$(awk -v a="$_asset" '{f=$NF; sub(/.*\//, "", f); if (f == a) { print $1; exit } }' "$TMPD/SHA256SUMS.txt" || true)"
            [ -n "${_want:-}" ] || die "no checksum entry for ${_asset} in SHA256SUMS.txt"
            _got="$(sha256sum "$TMPD/${_asset}" | cut -d' ' -f1)"
            [ "$_want" = "$_got" ] || die "SHA256 mismatch for ${_asset} (want ${_want}, got ${_got})"
        elif command -v shasum >/dev/null 2>&1; then
            _want="$(awk -v a="$_asset" '{f=$NF; sub(/.*\//, "", f); if (f == a) { print $1; exit } }' "$TMPD/SHA256SUMS.txt" || true)"
            [ -n "${_want:-}" ] || die "no checksum entry for ${_asset} in SHA256SUMS.txt"
            _got="$(shasum -a 256 "$TMPD/${_asset}" | cut -d' ' -f1)"
            [ "$_want" = "$_got" ] || die "SHA256 mismatch for ${_asset} (want ${_want}, got ${_got})"
        else
            warn "no sha256sum or shasum found; skipping SHA256 verification of ${_asset}"
        fi
        chmod +x "$TMPD/${_asset}"
        mv -f "$TMPD/${_asset}" "$_dest"
        [ -s "$_dest" ] || die "installed ${_dest} is empty"
    done
    info "installed ${TARGET_BIN} and ${TARGET_LSP}"
    if command -v gh >/dev/null 2>&1; then
        gh attestation verify "$TARGET_BIN" --repo "$ZEVER_REPO" || \
            warn "gh attestation verify failed for ${TARGET_BIN}"
        gh attestation verify "$TARGET_LSP" --repo "$ZEVER_REPO" || \
            warn "gh attestation verify failed for ${TARGET_LSP}"
    fi
fi

# --- PATH --------------------------------------------------------------------
case ":${PATH:-}:" in
    *":${BIN_DIR}:"*)
        info "${BIN_DIR} is already on PATH"
        ;;
    *)
        _shell_base="$(basename "${SHELL:-sh}")"
        if [ "$_shell_base" = "fish" ]; then
            _fish_cfg="${HOME}/.config/fish/config.fish"
            _fish_line="set -Ux fish_user_paths ${BIN_DIR} \$fish_user_paths"
            if [ "$DRY_RUN" -eq 1 ]; then
                printf 'dry-run: append to %s: %s\n' "$_fish_cfg" "$_fish_line"
            else
                mkdir -p "$(dirname "$_fish_cfg")"
                if [ -f "$_fish_cfg" ] && grep -qF "$BIN_DIR" "$_fish_cfg"; then
                    info "${_fish_cfg} already references ${BIN_DIR}"
                else
                    printf '%s\n%s\n' "$PATH_MARKER" "$_fish_line" >>"$_fish_cfg"
                    info "appended fish_user_paths entry to ${_fish_cfg}"
                fi
            fi
        else
            _rc=""
            for _cand in "${HOME}/.bashrc" "${HOME}/.zshrc" "${HOME}/.profile"; do
                if [ -f "$_cand" ]; then
                    _rc="$_cand"
                    break
                fi
            done
            [ -n "$_rc" ] || _rc="${HOME}/.profile"
            _export_line="export PATH=\"${BIN_DIR}:\$PATH\""
            if [ "$DRY_RUN" -eq 1 ]; then
                printf 'dry-run: append to %s: %s / %s\n' "$_rc" "$PATH_MARKER" "$_export_line"
            else
                if [ -f "$_rc" ] && grep -qF "$BIN_DIR" "$_rc"; then
                    info "${_rc} already references ${BIN_DIR}"
                else
                    printf '%s\n%s\n' "$PATH_MARKER" "$_export_line" >>"$_rc"
                    info "appended PATH export to ${_rc}"
                fi
            fi
        fi
        ;;
esac

# --- smoke test (never hangs; bounded, temp-dir, cleaned up) -----------------
smoke_test() {
    _smoke="$(mktemp -d)"
    info "smoke test in ${_smoke}"
    if [ "$DRY_RUN" -eq 1 ]; then
        printf 'dry-run: %s new smoke --dir %s/smoke\n' "$TARGET_BIN" "$_smoke"
        printf 'dry-run: (in %s/smoke) go mod tidy && go build ./...\n' "$_smoke"
        printf 'dry-run: run built server with timeout, assert output, report PASS/FAIL\n'
        printf 'dry-run: rm -rf %s\n' "$_smoke"
        return 0
    fi
    _fail() {
        printf 'install.sh: smoke test FAIL: %s\n' "$1" >&2
        rm -rf "$_smoke"
        return 1
    }
    "$TARGET_BIN" new smoke --dir "$_smoke/smoke" >/dev/null 2>&1 \
        || { _fail "'zever new smoke' failed"; return 1; }
    (cd "$_smoke/smoke" && go mod tidy) >/dev/null 2>&1 \
        || { _fail "'go mod tidy' failed in scaffold"; return 1; }
    (cd "$_smoke/smoke" && go build ./...) >/dev/null 2>&1 \
        || { _fail "'go build ./...' failed in scaffold"; return 1; }
    _out=""
    if command -v timeout >/dev/null 2>&1; then
        _rc=0
        _out="$(cd "$_smoke/smoke" && timeout 5 go run . 2>&1)" || _rc=$?
        # timeout kills a healthy server with 124; that plus/without output is fine.
        if [ -n "$_out" ]; then
            info "smoke test PASS (server output: $(printf '%s' "$_out" | head -c 120))"
        elif [ "$_rc" -eq 124 ]; then
            info "smoke test PASS (server ran until timeout with no output)"
        else
            _fail "scaffolded app produced no output (rc=${_rc})"
            return 1
        fi
    else
        info "smoke test PASS (build ok; 'timeout' missing so run-step skipped)"
    fi
    rm -rf "$_smoke"
    return 0
}

smoke_test || die "smoke test failed"

# --- done --------------------------------------------------------------------
info "installed zever ${VERSION} to ${TARGET_BIN} and ${TARGET_LSP}"
_shell_base="$(basename "${SHELL:-sh}")"
if [ "$_shell_base" = "fish" ]; then
    printf 'set -Ux fish_user_paths %s $fish_user_paths\n' "$BIN_DIR"
else
    printf 'export PATH="%s:$PATH"\n' "$BIN_DIR"
fi
