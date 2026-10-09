#!/bin/sh
# install.sh - install the zever CLI and zever-lsp from GitHub releases.
#
# Usage:
#   sh install.sh [--version vX.Y.Z] [--prefix DIR] [--yes] [--force] [--dry-run]
#                 [--quiet] [--no-color] [--help]
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
# - All UI goes to stderr. Colour only on a TTY, never when NO_COLOR is set
#   or TERM=dumb (FORCE_COLOR overrides the TTY check, --no-color wins).
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
QUIET=0
NO_COLOR_FLAG=0
TMPD=""
START_TS="$(date +%s 2>/dev/null || echo 0)"
STEP_N=0
STEP_TOTAL=5

# --- UI ------------------------------------------------------------------------
# ui_init sets colour + symbol variables. Safe to call repeatedly (flags are
# parsed after the first call, so errors raised during parsing still render).
ui_init() {
    C_RESET=""; C_BOLD=""; C_DIM=""; C_RED=""; C_GREEN=""; C_YELLOW=""; C_CYAN=""
    _use_color=0
    if [ "$NO_COLOR_FLAG" -eq 0 ] && [ -z "${NO_COLOR:-}" ] && [ "${TERM:-}" != "dumb" ]; then
        if [ -t 2 ] || [ -n "${FORCE_COLOR:-}" ]; then
            _use_color=1
        fi
    fi
    if [ "$_use_color" -eq 1 ]; then
        _esc="$(printf '\033')"
        C_RESET="${_esc}[0m"; C_BOLD="${_esc}[1m"; C_DIM="${_esc}[2m"
        C_RED="${_esc}[31m"; C_GREEN="${_esc}[32m"; C_YELLOW="${_esc}[33m"; C_CYAN="${_esc}[36m"
    fi
    case "${LC_ALL:-${LC_CTYPE:-${LANG:-}}}" in
        *UTF-8*|*utf-8*|*UTF8*|*utf8*)
            S_OK="✓"; S_FAIL="✗"; S_WARN="!"; S_STEP="→"; S_DOT="•"; S_WOULD="○"; S_ARROW="→" ;;
        *)
            S_OK="ok"; S_FAIL="x"; S_WARN="!"; S_STEP=">"; S_DOT="-"; S_WOULD="o"; S_ARROW="->" ;;
    esac
}
ui_init

# say MSG: plain stderr line, suppressed by --quiet.
say() {
    [ "$QUIET" -eq 1 ] || printf '%s\n' "$*" >&2
}

info() {
    say "  ${C_DIM}${S_DOT}${C_RESET} $*"
}

ok() {
    say "  ${C_GREEN}${S_OK}${C_RESET} $*"
}

step() {
    STEP_N=$((STEP_N + 1))
    say ""
    say "${C_BOLD}${C_CYAN}${S_STEP} [${STEP_N}/${STEP_TOTAL}]${C_RESET} ${C_BOLD}$*${C_RESET}"
}

kv() {
    # kv KEY VALUE
    say "$(printf '  %s%-10s%s %s' "$C_DIM" "$1" "$C_RESET" "$2")"
}

would() {
    say "  ${C_DIM}${S_WOULD} would $*${C_RESET}"
}

hint() {
    printf '  %shint:%s %s\n' "$C_DIM" "$C_RESET" "$*" >&2
}

warn() {
    printf '%s%s warning:%s %s\n' "$C_YELLOW" "$S_WARN" "$C_RESET" "$*" >&2
}

err() {
    printf '%s%s error:%s %s\n' "$C_RED" "$S_FAIL" "$C_RESET" "$*" >&2
}

die() {
    err "$*"
    exit 1
}

# die_hint MSG HINT: failure with an actionable next step.
die_hint() {
    err "$1"
    hint "$2"
    exit 1
}

usage_err() {
    err "$*"
    usage >&2
    exit 2
}

usage() {
    cat <<EOF
Install the zever CLI and zever-lsp from GitHub releases.

Usage: install.sh [OPTIONS]

Options:
  --version X   version to install (default: ${ZEVER_VERSION_DEFAULT}, or \$ZEVER_VERSION)
  --prefix DIR  install prefix, binaries go to DIR/bin (default: ~/.local, or \$ZEVER_PREFIX)
  --yes, -y     accept upgrade/downgrade prompts without asking
  --force       re-install even when the target version is already installed
                (implies --yes and overwrites existing binaries)
  --dry-run     show every action without executing it
  --quiet, -q   only print warnings, errors, and the PATH snippet
  --no-color    disable colour output (also honours \$NO_COLOR and TERM=dumb)
  --help, -h    print this help and exit

Environment:
  ZEVER_VERSION  same as --version
  ZEVER_PREFIX   same as --prefix
  NO_COLOR       disable colour output
  FORCE_COLOR    force colour output even when stderr is not a terminal

Examples:
  sh install.sh
  sh install.sh --version v0.6.1 --prefix ~/.local --yes
  ZEVER_VERSION=v0.6.1 ZEVER_PREFIX=/opt/zever sh install.sh --yes
EOF
}

rerun_cmd() {
    printf '%s --version %s --prefix %s --yes' "$0" "$VERSION" "$PREFIX"
}

# human_size FILE: print a human-readable size ("3.4 MB", "812 KB").
human_size() {
    _bytes="$(wc -c <"$1" | tr -d '[:space:]')"
    if [ "$_bytes" -ge 1048576 ]; then
        _t=$((_bytes * 10 / 1048576))
        printf '%d.%d MB' $((_t / 10)) $((_t % 10))
    elif [ "$_bytes" -ge 1024 ]; then
        printf '%d KB' $((_bytes / 1024))
    else
        printf '%d B' "$_bytes"
    fi
}

# elapsed: seconds since script start.
elapsed() {
    _now="$(date +%s 2>/dev/null || echo 0)"
    printf '%ds' $((_now - START_TS))
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
    _ask_prompt="${C_BOLD}${C_CYAN}?${C_RESET} $1"
    if [ -t 0 ]; then
        printf '%s ' "$_ask_prompt" >&2
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
        printf '%s ' "$_ask_prompt" >&2
        if IFS= read -r _ask_ans < /dev/tty; then
            case "$_ask_ans" in
                [Yy]*) return 0 ;;
                *) return 1 ;;
            esac
            return 1
        fi
    fi
    die_hint "non-interactive shell: confirmation required" "re-run with --yes: $(rerun_cmd)"
}

# --- arg parsing -------------------------------------------------------------
while [ $# -gt 0 ]; do
    case "$1" in
        --version)
            [ $# -ge 2 ] || { err "--version needs a value"; exit 2; }
            VERSION="$2"
            shift 2
            ;;
        --version=*)
            VERSION="${1#--version=}"
            shift
            ;;
        --prefix)
            [ $# -ge 2 ] || { err "--prefix needs a value"; exit 2; }
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
        --quiet|-q)
            QUIET=1
            shift
            ;;
        --no-color)
            NO_COLOR_FLAG=1
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
            ui_init
            usage_err "unknown flag: $1"
            ;;
        *)
            ui_init
            usage_err "unexpected argument: $1"
            ;;
    esac
done
ui_init
[ $# -eq 0 ] || usage_err "unexpected argument: $1"

VERSION="$(normalize_version "$VERSION")"
valid_version "$VERSION" || { err "invalid version: $VERSION (want vX.Y.Z)"; exit 2; }

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

say ""
say "${C_BOLD}zever installer${C_RESET} ${C_DIM}${VERSION}${C_RESET}"
if [ "$DRY_RUN" -eq 1 ]; then
    say "${C_YELLOW}dry run${C_RESET} ${C_DIM}- nothing will be changed${C_RESET}"
fi

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
    die_hint "need curl or wget to download release assets" "install one of them, plus go >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR} from ${GO_DL_URL}"
fi

command -v go >/dev/null 2>&1 || die_hint "go not found (need go >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR})" "install Go from ${GO_DL_URL}, then re-run"
# `go version` prints e.g. `go version go1.27.0 linux/amd64`.
_GO_VER_STR="$(go version 2>/dev/null)" || die_hint "could not run 'go version'" "need go >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR} from ${GO_DL_URL}"
set -- $_GO_VER_STR
_GO_VER="${3:-}"
_GO_VER="${_GO_VER#go}"
_GO_MAJOR="${_GO_VER%%.*}"
_GO_REST="${_GO_VER#*.}"
_GO_MINOR="${_GO_REST%%.*}"
case "$_GO_MAJOR.$_GO_MINOR" in
    *[!0-9.]*|"".*|.*""|*.*.*) die_hint "could not parse go version from '${_GO_VER_STR}'" "need go >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR} from ${GO_DL_URL}" ;;
esac
if [ "$(num_cmp "$_GO_MAJOR" "$GO_MIN_MAJOR")" -lt 0 ] || \
   { [ "$(num_cmp "$_GO_MAJOR" "$GO_MIN_MAJOR")" -eq 0 ] && [ "$(num_cmp "$_GO_MINOR" "$GO_MIN_MINOR")" -lt 0 ]; }; then
    die_hint "go ${_GO_VER} is too old (need go >= ${GO_MIN_MAJOR}.${GO_MIN_MINOR})" "upgrade Go from ${GO_DL_URL}, then re-run"
fi

download_file() {
    # download_file URL DEST - shows a progress bar only on an interactive stderr.
    if [ "$DOWNLOADER" = "curl" ]; then
        if [ -t 2 ] && [ "$QUIET" -eq 0 ]; then
            curl --proto '=https' --tlsv1.2 -fL --progress-bar -o "$2" "$1"
        else
            curl --proto '=https' --tlsv1.2 -sSf -L -o "$2" "$1"
        fi
    else
        wget -q -O "$2" "$1"
    fi
}

# sha256_of FILE: print the hex digest, or nothing when no hashing tool exists.
sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | cut -d' ' -f1
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

# Decide the action first so the plan block can show it, then prompt/exit.
HAVE=""
CMP=""
ACTION="fresh install"
GATE="fresh"
if [ -n "$EXISTING" ]; then
    HAVE="$(installed_version "$EXISTING")"
    if [ -z "$HAVE" ]; then
        ACTION="replace unrecognized install"
        GATE="unparseable"
    else
        CMP="$(vercmp "$HAVE" "$VERSION")"
        if [ "$CMP" -eq 0 ] && [ "$FORCE" -eq 0 ]; then
            if [ -s "$TARGET_BIN" ] && [ -s "$TARGET_LSP" ]; then
                ACTION="already up to date"
                GATE="uptodate"
            else
                ACTION="repair (binaries missing or empty)"
                GATE="repair"
            fi
        elif [ "$CMP" -eq 0 ]; then
            ACTION="reinstall (--force)"
            GATE="force"
        elif [ "$CMP" -lt 0 ]; then
            ACTION="upgrade ${HAVE} ${S_ARROW} ${VERSION}"
            GATE="upgrade"
        else
            ACTION="downgrade ${HAVE} ${S_ARROW} ${VERSION}"
            GATE="downgrade"
        fi
    fi
fi

say ""
say "${C_BOLD}Plan${C_RESET}"
kv "version" "$VERSION"
kv "platform" "$ASSET_TRIPLE"
kv "install to" "$BIN_DIR"
kv "go" "${_GO_VER} ${C_DIM}(>= ${GO_MIN_MAJOR}.${GO_MIN_MINOR} required)${C_RESET}"
kv "download" "$DOWNLOADER"
if [ -n "$EXISTING" ]; then
    kv "existing" "${EXISTING}${HAVE:+ (${HAVE})}"
fi
kv "action" "${C_BOLD}${ACTION}${C_RESET}"

if [ -n "$EXISTING" ] && [ "$EXISTING" != "$TARGET_BIN" ]; then
    warn "existing zever at ${EXISTING} takes precedence over ${TARGET_BIN} per PATH order"
    hint "this install writes ${TARGET_BIN}; fix your PATH if 'zever' still resolves elsewhere"
fi

case "$GATE" in
    uptodate)
        say ""
        ok "zever ${VERSION} is already installed and up to date at ${TARGET_BIN}"
        hint "use --force to reinstall"
        exit 0
        ;;
    repair)
        warn "zever ${VERSION} reports installed but ${TARGET_BIN} or ${TARGET_LSP} is missing/empty; reinstalling"
        ;;
    unparseable)
        if [ "$DRY_RUN" -eq 1 ]; then
            would "prompt to replace unparseable install at ${EXISTING}"
        elif ! ask "Replace existing zever at ${EXISTING} with ${VERSION}? [y/N]"; then
            die "aborted by user"
        fi
        ;;
    upgrade)
        if [ "$DRY_RUN" -eq 1 ]; then
            would "prompt: Upgrade zever ${HAVE} ${S_ARROW} ${VERSION}? (assuming yes)"
        elif ! ask "Upgrade zever ${HAVE} ${S_ARROW} ${VERSION}? [y/N]"; then
            die "aborted by user"
        fi
        ;;
    downgrade)
        warn "installed zever ${HAVE} is NEWER than target ${VERSION}"
        if [ "$DRY_RUN" -eq 1 ]; then
            would "prompt: Downgrade zever ${HAVE} ${S_ARROW} ${VERSION}? (assuming yes)"
        elif ! ask "Downgrade zever ${HAVE} ${S_ARROW} ${VERSION}? [y/N]"; then
            die "aborted by user"
        fi
        ;;
esac

# --- download + verify + install ---------------------------------------------
cleanup() {
    if [ -n "$TMPD" ] && [ -d "$TMPD" ]; then
        rm -rf "$TMPD"
    fi
}
trap cleanup EXIT INT TERM

if [ "$DRY_RUN" -eq 0 ]; then
    mkdir -p "$BIN_DIR"
    TMPD="$(mktemp -d)"
fi

SUMS_URL="${BASE_URL}/SHA256SUMS.txt"
RELEASE_HINT="check that release ${VERSION} exists: https://github.com/${ZEVER_REPO}/releases"

step "Fetching checksums"
if [ "$DRY_RUN" -eq 1 ]; then
    would "download ${SUMS_URL}"
else
    download_file "$SUMS_URL" "$TMPD/SHA256SUMS.txt" || \
        die_hint "could not download ${SUMS_URL}" "${RELEASE_HINT} (older releases may predate checksum files)"
    ok "SHA256SUMS.txt for ${VERSION}"
fi

# install_asset ASSET DEST LABEL: download, verify against SHA256SUMS.txt, install.
install_asset() {
    _asset="$1"
    _dest="$2"
    step "Installing $3"
    if [ "$DRY_RUN" -eq 1 ]; then
        would "download ${BASE_URL}/${_asset}"
        would "verify SHA256 against SHA256SUMS.txt"
        would "install to ${_dest}"
        return 0
    fi
    info "downloading ${_asset}"
    download_file "${BASE_URL}/${_asset}" "$TMPD/${_asset}" || \
        die_hint "could not download ${BASE_URL}/${_asset}" "$RELEASE_HINT"
    [ -s "$TMPD/${_asset}" ] || die "downloaded ${_asset} is empty"
    _size="$(human_size "$TMPD/${_asset}")"
    _got="$(sha256_of "$TMPD/${_asset}")"
    if [ -n "$_got" ]; then
        _want="$(awk -v a="$_asset" '{f=$NF; sub(/.*\//, "", f); if (f == a) { print $1; exit } }' "$TMPD/SHA256SUMS.txt" || true)"
        [ -n "${_want:-}" ] || die "no checksum entry for ${_asset} in SHA256SUMS.txt"
        [ "$_want" = "$_got" ] || die_hint "SHA256 mismatch for ${_asset} (want ${_want}, got ${_got})" "do NOT use this binary; retry, and report it if it persists"
        ok "verified ${_asset} ${C_DIM}(${_size}, sha256 $(printf '%s' "$_got" | cut -c1-12)...)${C_RESET}"
    else
        warn "no sha256sum or shasum found; skipping SHA256 verification of ${_asset}"
    fi
    chmod +x "$TMPD/${_asset}"
    mv -f "$TMPD/${_asset}" "$_dest"
    [ -s "$_dest" ] || die "installed ${_dest} is empty"
    ok "installed ${_dest}"
}

install_asset "$ZEVER_ASSET" "$TARGET_BIN" "zever"
install_asset "$LSP_ASSET" "$TARGET_LSP" "zever-lsp"

if [ "$DRY_RUN" -eq 1 ]; then
    would "run gh attestation verify (only if gh exists)"
elif command -v gh >/dev/null 2>&1; then
    info "verifying build provenance with gh"
    for _bin in "$TARGET_BIN" "$TARGET_LSP"; do
        if gh attestation verify "$_bin" --repo "$ZEVER_REPO" >/dev/null 2>&1; then
            ok "attestation verified for ${_bin##*/}"
        else
            warn "gh attestation verify failed for ${_bin}"
        fi
    done
fi

# --- PATH --------------------------------------------------------------------
step "Configuring PATH"
PATH_NEEDS_RELOAD=0
case ":${PATH:-}:" in
    *":${BIN_DIR}:"*)
        ok "${BIN_DIR} is already on PATH"
        ;;
    *)
        PATH_NEEDS_RELOAD=1
        _shell_base="$(basename "${SHELL:-sh}")"
        if [ "$_shell_base" = "fish" ]; then
            _fish_cfg="${HOME}/.config/fish/config.fish"
            _fish_line="set -Ux fish_user_paths ${BIN_DIR} \$fish_user_paths"
            if [ "$DRY_RUN" -eq 1 ]; then
                would "append to ${_fish_cfg}: ${_fish_line}"
            else
                mkdir -p "$(dirname "$_fish_cfg")"
                if [ -f "$_fish_cfg" ] && grep -qF "$BIN_DIR" "$_fish_cfg"; then
                    ok "${_fish_cfg} already references ${BIN_DIR}"
                else
                    printf '%s\n%s\n' "$PATH_MARKER" "$_fish_line" >>"$_fish_cfg"
                    ok "appended fish_user_paths entry to ${_fish_cfg}"
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
                would "append to ${_rc}: ${_export_line}"
            else
                if [ -f "$_rc" ] && grep -qF "$BIN_DIR" "$_rc"; then
                    ok "${_rc} already references ${BIN_DIR}"
                else
                    printf '%s\n%s\n' "$PATH_MARKER" "$_export_line" >>"$_rc"
                    ok "appended PATH export to ${_rc}"
                fi
            fi
        fi
        ;;
esac

# --- smoke test (never hangs; bounded, temp-dir, cleaned up) -----------------
smoke_test() {
    _smoke="$(mktemp -d)"
    info "scaffolding a throwaway app in ${_smoke} (this can take a minute)"
    _fail() {
        err "smoke test failed: $1"
        rm -rf "$_smoke"
        return 1
    }
    "$TARGET_BIN" new smoke --dir "$_smoke/smoke" >/dev/null 2>&1 \
        || { _fail "'zever new smoke' failed"; return 1; }
    ok "zever new"
    (cd "$_smoke/smoke" && go mod tidy) >/dev/null 2>&1 \
        || { _fail "'go mod tidy' failed in scaffold"; return 1; }
    (cd "$_smoke/smoke" && go build ./...) >/dev/null 2>&1 \
        || { _fail "'go build ./...' failed in scaffold"; return 1; }
    ok "go build"
    _out=""
    if command -v timeout >/dev/null 2>&1; then
        _rc=0
        _out="$(cd "$_smoke/smoke" && timeout 5 go run . 2>&1)" || _rc=$?
        # timeout kills a healthy server with 124; that plus/without output is fine.
        if [ -n "$_out" ]; then
            ok "smoke test passed ${C_DIM}(server output: $(printf '%s' "$_out" | head -c 120))${C_RESET}"
        elif [ "$_rc" -eq 124 ]; then
            ok "smoke test passed ${C_DIM}(server ran until timeout with no output)${C_RESET}"
        else
            _fail "scaffolded app produced no output (rc=${_rc})"
            return 1
        fi
    else
        ok "smoke test passed ${C_DIM}(build ok; 'timeout' missing so run-step skipped)${C_RESET}"
    fi
    rm -rf "$_smoke"
    return 0
}

step "Smoke test"
if [ "$DRY_RUN" -eq 1 ]; then
    would "${TARGET_BIN} new smoke --dir <tmp>/smoke"
    would "go mod tidy && go build ./... in the scaffold"
    would "run the built server with a timeout and assert output"
    would "remove the temp directory"
else
    smoke_test || die_hint "smoke test failed" "binaries are installed at ${TARGET_BIN}; run 'zever doctor' to diagnose"
fi

# --- done --------------------------------------------------------------------
say ""
if [ "$DRY_RUN" -eq 1 ]; then
    say "${C_BOLD}${C_YELLOW}${S_WOULD} Dry run complete${C_RESET} ${C_DIM}- zever ${VERSION} was not installed${C_RESET}"
    exit 0
fi
say "${C_BOLD}${C_GREEN}${S_OK} zever ${VERSION} installed${C_RESET} ${C_DIM}in $(elapsed)${C_RESET}"
say ""
kv "zever" "$TARGET_BIN"
kv "zever-lsp" "$TARGET_LSP"
say ""
say "${C_BOLD}Next steps${C_RESET}"
if [ "$PATH_NEEDS_RELOAD" -eq 1 ]; then
    say "  ${C_DIM}1.${C_RESET} Reload your shell ${C_DIM}(open a new terminal, or run the snippet below)${C_RESET}"
    say "  ${C_DIM}2.${C_RESET} ${C_CYAN}zever -V${C_RESET}              ${C_DIM}check the install${C_RESET}"
    say "  ${C_DIM}3.${C_RESET} ${C_CYAN}zever new myapp${C_RESET}       ${C_DIM}scaffold a project${C_RESET}"
else
    say "  ${C_DIM}1.${C_RESET} ${C_CYAN}zever -V${C_RESET}              ${C_DIM}check the install${C_RESET}"
    say "  ${C_DIM}2.${C_RESET} ${C_CYAN}zever new myapp${C_RESET}       ${C_DIM}scaffold a project${C_RESET}"
fi
say "  ${C_DIM}${S_DOT}${C_RESET} ${C_CYAN}zever upgrade${C_RESET} / ${C_CYAN}zever uninstall${C_RESET}  ${C_DIM}manage this install later${C_RESET}"
if [ "$PATH_NEEDS_RELOAD" -eq 1 ]; then
    say ""
    _shell_base="$(basename "${SHELL:-sh}")"
    if [ "$_shell_base" = "fish" ]; then
        printf 'set -Ux fish_user_paths %s $fish_user_paths\n' "$BIN_DIR"
    else
        printf 'export PATH="%s:$PATH"\n' "$BIN_DIR"
    fi
fi
