#!/usr/bin/env bash
# sbom-test.sh - offline tests for tools/sbom.sh.
#
# The SBOM job resolves every module's dependency graph before
# cyclonedx-gomod can list it. The resolution has two paths: the published
# one (primary) and the local intra-repo closure (fallback, used while the
# pinned siblings are not published yet). Both are exercised here in a
# throwaway module tree under a temp dir:
#
#   - GOPROXY=off makes the published resolution fail exactly like an
#     unpublished sibling does, without touching the network.
#   - a stub cyclonedx-gomod writes a minimal BOM, so the test needs
#     neither the real tool nor a license database.
#
# Every scenario asserts that go.mod/go.sum are byte-identical afterwards:
# the go.sum additions of the download and the temporary replaces must not
# survive the run.
#
# Usage: bash tools/sbom-test.sh
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SBOM="$ROOT/tools/sbom.sh"

TMPD="$(mktemp -d)"
trap 'rm -rf "$TMPD"' EXIT INT TERM

PASS=0
FAIL=0

say() { printf '%s\n' "$*" >&2; }
ok() {
	say "ok - $1"
	PASS=$((PASS + 1))
}
bad() {
	say "FAIL - $1"
	FAIL=$((FAIL + 1))
}

assert_rc() {
	# assert_rc DESC EXPECTED_RC ACTUAL_RC
	if [ "$2" -eq "$3" ]; then ok "$1"; else bad "$1 (want rc=$2, got rc=$3)"; fi
}

assert_file() {
	# assert_file DESC PATH
	if [ -f "$2" ]; then ok "$1"; else bad "$1 (missing $2)"; fi
}

assert_absent() {
	# assert_absent DESC PATH
	if [ ! -e "$2" ]; then ok "$1"; else bad "$1 (unexpected $2)"; fi
}

assert_same() {
	# assert_same DESC FILE_A FILE_B
	if cmp -s "$2" "$3"; then ok "$1"; else bad "$1 ($2 differs from $3)"; fi
}

assert_grep() {
	# assert_grep DESC PATTERN FILE
	if grep -qE "$2" "$3"; then ok "$1"; else bad "$1 (no /$2/ in $3)"; fi
}

assert_not_grep() {
	# assert_not_grep DESC PATTERN FILE
	if grep -qE "$2" "$3"; then bad "$1 (found /$2/ in $3)"; else ok "$1"; fi
}

# gomod DIR MODULE_PATH [REQUIRE ...] writes a module file.
gomod() {
	local dir="$1" path="$2"
	shift 2
	mkdir -p "$dir"
	{
		printf 'module %s\n\ngo 1.21\n' "$path"
		if [ "$#" -gt 0 ]; then
			printf '\nrequire (\n'
			local r
			for r in "$@"; do printf '\t%s\n' "$r"; done
			printf ')\n'
		fi
	} >"$dir/go.mod"
}

# gopkg DIR PACKAGE [IMPORT ...] writes a package file.
gopkg() {
	local dir="$1" pkg="$2"
	shift 2
	mkdir -p "$dir"
	{
		printf 'package %s\n' "$pkg"
		if [ "$#" -gt 0 ]; then
			printf '\nimport (\n'
			local i
			for i in "$@"; do printf '\t_ "%s"\n' "$i"; done
			printf ')\n'
		fi
	} >"$dir/$pkg.go"
}

# stub writes a minimal BOM and mimics the "no licenses detected" warning
# of the real tool, so the license stamping of tools/sbom.sh is covered.
STUB="$TMPD/cyclonedx-gomod"
cat >"$STUB" <<'STUBEOF'
#!/usr/bin/env bash
set -eu
out=""
while [ "$#" -gt 0 ]; do
	case "$1" in
	-output)
		out="$2"
		shift 2
		;;
	*)
		shift
		;;
	esac
done
printf '%s\n' "WRN no licenses detected module=github.com/zenta-dev/zever/alpha" >&2
cat >"$out" <<'JSON'
{
  "bomFormat": "CycloneDX",
  "specVersion": "1.6",
  "version": 1,
  "metadata": {"component": {"type": "library", "name": "github.com/zenta-dev/zever/solo"}},
  "components": [
    {"type": "library", "name": "github.com/zenta-dev/zever/alpha"}
  ]
}
JSON
STUBEOF
chmod +x "$STUB"

# run_sbom REPO MODULE OUT LOG -> prints the exit code
#
# The proxy is an empty file:// directory, so the published resolution
# fails exactly like an unpublished sibling does ("...@v0.0.0: reading
# file://...: no such file or directory") without touching the network.
run_sbom() {
	local repo="$1" mod="$2" out="$3" log="$4" rc=0
	mkdir -p "$TMPD/emptyproxy"
	SBOM_REPO_ROOT="$repo" \
		CYCLONEDX_GOMOD="$STUB" \
		GO="${GO:-go}" \
		GOPROXY="file://$TMPD/emptyproxy" GOSUMDB=off GOFLAGS= \
		bash "$SBOM" "$repo/$mod" "$out" >"$log" 2>&1 || rc=$?
	printf '%s' "$rc"
}

# --- scenario 1: the published resolution works -------------------------
repo="$TMPD/repo1"
mkdir -p "$repo"
gomod "$repo/solo" "github.com/zenta-dev/zever/solo"
gopkg "$repo/solo" solo
cp "$repo/solo/go.mod" "$TMPD/solo.go.mod"
rc="$(run_sbom "$repo" solo "$TMPD/solo.json" "$TMPD/solo.log")"
assert_rc "primary path: exit 0" 0 "$rc"
assert_file "primary path: SBOM written" "$TMPD/solo.json"
assert_grep "primary path: resolved from the module proxy" "resolved from the module proxy" "$TMPD/solo.log"
assert_not_grep "primary path: no local fallback" "local closure" "$TMPD/solo.log"
assert_same "primary path: go.mod unchanged" "$TMPD/solo.go.mod" "$repo/solo/go.mod"
assert_absent "primary path: no go.sum left behind" "$repo/solo/go.sum"
assert_grep "primary path: own-module license stamped" '"Apache-2.0"' "$TMPD/solo.json"

# --- scenario 2: the sibling is not published yet ------------------------
repo="$TMPD/repo2"
mkdir -p "$repo"
gomod "$repo/beta" "github.com/zenta-dev/zever/beta"
gopkg "$repo/beta" beta
gomod "$repo/alpha" "github.com/zenta-dev/zever/alpha" "github.com/zenta-dev/zever/beta v0.0.0"
gopkg "$repo/alpha" alpha "github.com/zenta-dev/zever/beta"
gomod "$repo/app" "github.com/zenta-dev/zever/app" "github.com/zenta-dev/zever/alpha v0.0.0"
gopkg "$repo/app" app "github.com/zenta-dev/zever/alpha"
cp "$repo/app/go.mod" "$TMPD/app.go.mod"
rc="$(run_sbom "$repo" app "$TMPD/app.json" "$TMPD/app.log")"
assert_rc "fallback path: exit 0" 0 "$rc"
assert_file "fallback path: SBOM written" "$TMPD/app.json"
assert_grep "fallback path: local closure used" "resolving the local closure" "$TMPD/app.log"
assert_same "fallback path: go.mod unchanged" "$TMPD/app.go.mod" "$repo/app/go.mod"
assert_not_grep "fallback path: no temporary replace left" "=>" "$repo/app/go.mod"
assert_absent "fallback path: no go.sum left behind" "$repo/app/go.sum"

# --- scenario 3: a committed replace survives the fallback ---------------
repo="$TMPD/repo3"
mkdir -p "$repo"
gomod "$repo/gamma" "github.com/zenta-dev/zever/gamma"
gopkg "$repo/gamma" gamma
gomod "$repo/alpha" "github.com/zenta-dev/zever/alpha"
gopkg "$repo/alpha" alpha
gomod "$repo/app" "github.com/zenta-dev/zever/app" \
	"github.com/zenta-dev/zever/alpha v0.0.0" "github.com/zenta-dev/zever/gamma v0.0.0"
gopkg "$repo/app" app "github.com/zenta-dev/zever/alpha" "github.com/zenta-dev/zever/gamma"
printf '\nreplace github.com/zenta-dev/zever/alpha => ../alpha\n' >>"$repo/app/go.mod"
cp "$repo/app/go.mod" "$TMPD/app3.go.mod"
rc="$(run_sbom "$repo" app "$TMPD/app3.json" "$TMPD/app3.log")"
assert_rc "committed replace: exit 0" 0 "$rc"
assert_grep "committed replace: local closure used" "resolving the local closure" "$TMPD/app3.log"
assert_same "committed replace: go.mod unchanged" "$TMPD/app3.go.mod" "$repo/app/go.mod"
assert_grep "committed replace: still committed" "zenta-dev/zever/alpha => ../alpha" "$repo/app/go.mod"
assert_not_grep "committed replace: no replace for the sibling" "zenta-dev/zever/gamma =>" "$repo/app/go.mod"

# --- scenario 4: a third-party failure fails loudly -----------------------
repo="$TMPD/repo4"
mkdir -p "$repo"
gomod "$repo/broken" "github.com/zenta-dev/zever/broken" "example.com/nope v1.0.0"
gopkg "$repo/broken" broken "example.com/nope"
cp "$repo/broken/go.mod" "$TMPD/broken.go.mod"
rc="$(run_sbom "$repo" broken "$TMPD/broken.json" "$TMPD/broken.log")"
if [ "$rc" -ne 0 ]; then ok "third-party failure: exit != 0"; else bad "third-party failure: exit != 0 (got 0)"; fi
assert_absent "third-party failure: no SBOM written" "$TMPD/broken.json"
assert_same "third-party failure: go.mod unchanged" "$TMPD/broken.go.mod" "$repo/broken/go.mod"
assert_not_grep "third-party failure: no local fallback" "local closure" "$TMPD/broken.log"

if [ "$FAIL" -eq 0 ]; then
	say "all $PASS assertions passed"
	exit 0
fi
say "$FAIL of $((PASS + FAIL)) assertions failed"
exit 1
