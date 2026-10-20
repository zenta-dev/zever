#!/usr/bin/env bash
# sbom.sh - generate the CycloneDX SBOM of one module, hermetically.
#
# Usage: tools/sbom.sh <module-dir> <output-sbom.json>
#
# cyclonedx-gomod needs the module's whole dependency graph in the module
# cache before it can list it, so the SBOM job resolves the graph first with
# `go mod download all`. That resolution goes through the module proxy, so it
# fails while the pinned intra-repo siblings (the unreleased v0.8.0 line) are
# not published yet:
#
#   go: github.com/zenta-dev/zever/adapters/ai/anthropic@v0.8.0: invalid
#   version: unknown revision adapters/ai/anthropic/v0.8.0
#
# Two paths resolve that:
#
#   primary   `GOWORK=off go mod download all` succeeds: the SBOM is built
#             from the published versions. This is the path the release
#             workflow takes, so the released SBOM stays byte-identical to
#             the pre-fallback output.
#   fallback  the published resolution failed on an intra-repo sibling: the
#             transitive intra-repo closure is resolved LOCALLY through
#             temporary `replace` directives (the approach tools/gotidy.sh
#             already uses), the download is retried, and the temporary
#             replaces are dropped again.
#
# The fallback only triggers when the failure names an intra-repo module; any
# other failure (a third-party module that cannot be fetched) is reported and
# fails the job instead of silently degrading the SBOM.
#
# go.mod and go.sum are restored byte-for-byte afterwards, so the tree stays
# clean: the download's go.sum additions never leak into the commit and no
# temporary replace survives the run.
set -euo pipefail

root="${SBOM_REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
root="$(cd "$root" && pwd)"

go_cmd="${GO:-go}"
cyclonedx="${CYCLONEDX_GOMOD:-cyclonedx-gomod}"
own_prefix="${SBOM_OWN_PREFIX:-github.com/zenta-dev/zever}"
own_license="${SBOM_OWN_LICENSE:-Apache-2.0}"
intra_prefix="${SBOM_INTRA_PREFIX:-github.com/zenta-dev/zever/}"

die() {
	printf 'sbom: %s\n' "$*" >&2
	exit 1
}

usage() {
	printf 'usage: %s <module-dir> <output-sbom.json>\n' "$0" >&2
	exit 2
}

# prefix_re is intra_prefix as a regular expression (the dots are literal).
prefix_re="$(printf '%s' "$intra_prefix" | sed 's/[][\\.*^$]/\\&/g')"

# module_path prints the module path declared by a go.mod file.
module_path() {
	sed -En 's|^[[:space:]]*module[[:space:]]+([^[:space:]]+).*|\1|p' "$1" | head -1
}

# intra_requires prints the intra-repo module paths a go.mod requires.
# Both the single-line form and the block form are matched; the version is
# required so that replace/exclude lines never match.
intra_requires() {
	sed -En "s|^[[:space:]]*(require[[:space:]]+)?(${prefix_re}[^[:space:]]+)[[:space:]]+v[^[:space:]]*.*|\2|p" "$1" | sort -u
}

# sibling_dir maps an intra-repo module path to its directory in the repo.
# The directory must exist and must really declare that module path, so a
# path that only looks intra-repo (docs/examples uses a dashed path) is
# never resolved locally.
sibling_dir() {
	local path="$1" dir
	dir="$root/${path#"$intra_prefix"}"
	[ -f "$dir/go.mod" ] || return 1
	[ "$(module_path "$dir/go.mod")" = "$path" ] || return 1
	printf '%s' "$dir"
}

# closure prints the transitive intra-repo closure of a module directory.
# The closure is expanded by parsing the sibling go.mod files, so no module
# graph is loaded and siblings without a proxy release yet still resolve.
closure() {
	local deps dir sub s t
	deps="$(intra_requires "$1/go.mod" | tr '\n' ' ')"
	local changed=1
	while [ "$changed" = 1 ]; do
		changed=0
		for s in $deps; do
			dir="$(sibling_dir "$s")" || continue
			sub="$(intra_requires "$dir/go.mod" | tr '\n' ' ')"
			for t in $sub; do
				case " $deps " in
				*" $t "*) ;;
				*)
					deps="$deps $t"
					changed=1
					;;
				esac
			done
		done
	done
	printf '%s\n' $deps | sort -u
}

# rel_path prints the path of a repo-relative directory, relative to another
# repo-relative directory. The replacement paths must stay relative ("../"
# prefixed): cyclonedx-gomod only resolves local replacements whose path
# starts with "./" or "../" (see its ResolveLocalReplacements).
rel_path() {
	local from="$1" to="$2" depth up i
	case "$from" in
	"" | ".") depth=0 ;;
	*) depth="$(($(printf '%s' "$from" | tr -cd '/' | wc -c | tr -d '[:space:]') + 1))" ;;
	esac
	# One ".." per path component of the source directory.
	up=""
	i=0
	while [ "$i" -lt "$depth" ]; do
		up="../$up"
		i=$((i + 1))
	done
	printf '%s%s' "$up" "$to"
}

# replaced_paths prints the module paths this module already replaces.
replaced_paths() {
	(cd "$moddir" && GOWORK=off "$go_cmd" mod edit -json) |
		jq -r '.Replace[]?.Old.Path' | sort -u
}

# download runs `go mod download all` for the module.
download() {
	(cd "$moddir" && GOWORK=off "$go_cmd" mod download all)
}

# resolve downloads the module graph, falling back to a local resolution of
# the intra-repo closure when the published resolution is unavailable.
# The temporary replace directives are recorded in temp_drop so they can be
# dropped again after the SBOM was generated.
resolve() {
	local log="$tmp/primary.log" replaced s dir rel

	if download >"$tmp/primary.out" 2>"$log"; then
		printf 'sbom: %s: resolved from the module proxy\n' "$modrel" >&2
		return 0
	fi

	# Only an intra-repo sibling may trigger the fallback: any other
	# failure (a third-party module that cannot be fetched) must fail the
	# job instead of silently degrading the SBOM.
	if ! grep -Eq "${prefix_re}[^[:space:]]*@v[0-9]" "$log"; then
		cat "$log" >&2
		die "$modrel: go mod download all failed (not an intra-repo resolution failure)"
	fi

	printf 'sbom: %s: intra-repo siblings are not published; resolving the local closure\n' "$modrel" >&2

	replaced=" $(replaced_paths | tr '\n' ' ')"
	for s in $(closure "$moddir"); do
		case "$replaced" in
		*" $s "*) continue ;; # already replaced: leave the committed directive alone
		esac
		dir="${s#"$intra_prefix"}"
		rel="$(rel_path "$modrel" "$dir")"
		temp_add+=("-replace=$s=$rel")
		temp_drop+=("-dropreplace=$s")
	done

	if [ "${#temp_add[@]}" -eq 0 ]; then
		cat "$log" >&2
		die "$modrel: go mod download all failed and no intra-repo module could be replaced locally"
	fi

	(cd "$moddir" && GOWORK=off "$go_cmd" mod edit "${temp_add[@]}")
	if ! download >"$tmp/fallback.out" 2>"$tmp/fallback.log"; then
		cat "$tmp/fallback.log" >&2
		drop_replaces
		die "$modrel: go mod download all failed with the local intra-repo closure"
	fi
}

# drop_replaces removes the temporary replace directives again. The
# committed ones are never touched: only the modules that had no replace
# directive got one.
drop_replaces() {
	[ "${#temp_drop[@]}" -gt 0 ] || return 0
	(cd "$moddir" && GOWORK=off "$go_cmd" mod edit "${temp_drop[@]}") || true
	temp_drop=()
}

# generate writes the SBOM of the module and stamps the own-module license.
# The stamp mirrors the repo LICENSE: the own modules live in subdirs
# without a LICENSE file, so cyclonedx-gomod cannot detect them.
generate() {
	local rc=0
	(cd "$moddir" && GOWORK=off "$cyclonedx" mod -licenses -std -json -output "$out" .) \
		2>"$tmp/cyclonedx.log" || rc=$?
	grep -v "no licenses detected.*$own_prefix" "$tmp/cyclonedx.log" >&2 || true
	[ "$rc" -eq 0 ] || die "$modrel: cyclonedx-gomod failed (exit $rc)"
	[ -f "$out" ] || die "$modrel: cyclonedx-gomod wrote no SBOM"

	if ! jq --arg p "$own_prefix" --arg l "$own_license" \
		'(.metadata.component, .components[]?) |= (if (.name | startswith($p)) and ((.licenses // []) | length) == 0 then .licenses = [{license: {id: $l}}] else . end)' \
		"$out" >"$out.tmp"; then
		rm -f "$out.tmp"
		die "$modrel: license stamping failed"
	fi
	mv "$out.tmp" "$out"
}

[ "$#" -eq 2 ] || usage
command -v "$go_cmd" >/dev/null 2>&1 || die "go not found: $go_cmd"
command -v jq >/dev/null 2>&1 || die "jq not found: install jq to stamp own-module licenses"

moddir="$1"
out="$2"
[ -f "$moddir/go.mod" ] || die "no go.mod in $moddir"
moddir="$(cd "$moddir" && pwd)"
outdir="$(dirname "$out")"
mkdir -p "$outdir"
out="$(cd "$outdir" && pwd)/$(basename "$out")"

case "$moddir" in
"$root") modrel="." ;;
"$root"/*) modrel="${moddir#"$root"/}" ;;
*) die "module $moddir is outside the repository root $root" ;;
esac

# temp_add holds the temporary replace directives of the fallback,
# temp_drop the matching drop directives.
temp_add=()
temp_drop=()

tmp="$(mktemp -d)"
has_sum=0
cp "$moddir/go.mod" "$tmp/go.mod"
if [ -f "$moddir/go.sum" ]; then
	cp "$moddir/go.sum" "$tmp/go.sum"
	has_sum=1
fi

# restore puts go.mod and go.sum back byte-for-byte. The download adds
# go.sum entries and the fallback adds temporary replace directives; both
# must not survive the run.
restore() {
	cp "$tmp/go.mod" "$moddir/go.mod" 2>/dev/null || true
	if [ "$has_sum" = 1 ]; then
		cp "$tmp/go.sum" "$moddir/go.sum" 2>/dev/null || true
	else
		rm -f "$moddir/go.sum" 2>/dev/null || true
	fi
}
trap 'restore; rm -rf "$tmp"' EXIT

resolve
generate
