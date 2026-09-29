#!/usr/bin/env python3
"""Ensure every module's go.mod requires+replaces what it directly imports.

Go ignores replace directives in dependency go.mods, so each module must
repeat require+replace for every intra-repo module it directly imports
(transitives resolve via each dep's own go.mod). This script computes that
set from source imports and patches each go.mod via `go mod edit`.

Direct-only is deliberate: `go mod tidy -diff` drops requires nothing
imports, so a transitive closure here would fight tidy forever.

Usage: python3 tools/fix-module-graph.py [--tidy] [--check] [--version=vX.Y.Z] [--root DIR]
  --tidy  also run `go mod tidy` per module (needs primed module cache).
  --check compute the required require+replace closure per module and exit 1
          with a drift listing when a go.mod lacks entries; exit 0 when
          clean. Makes no changes.
  --root DIR  check/fix the repo rooted at DIR (default: script's parent).
          Used to exercise drift detection against a scratch copy.
"""
import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ZEVER_PREFIX = "github.com/zenta-dev/zever/"


def scan_imports(text):
    """Intra-repo paths from real import statements only.

    A mini-lexer: raw strings (backtick, incl. stray backticks in
    comments or interpreted strings, which break naive splitting),
    interpreted strings, rune literals, and both comment forms are
    skipped; only the `import` declaration grammar is interpreted.
    Code-generator templates (cmd/zever's server/worker templates,
    dsl's backend emitters) and test fixtures therefore never count,
    while real imports — block, single-line, aliased — always do.
    """
    found = set()
    n = len(text)

    def skip_ws(i):
        while i < n and text[i] in " \t\r\n":
            i += 1
        return i

    def read_quoted(i):
        vals = []
        j = i + 1
        while j < n and text[j] != '"' and text[j] != '\n':
            if text[j] == "\\" and j + 1 < n:
                vals.append(text[j + 1])
                j += 2
            else:
                vals.append(text[j])
                j += 1
        if j < n and text[j] == '"':
            return "".join(vals), j + 1
        return None, j

    def skip_string(i):
        if text[i] == "`":
            j = text.find("`", i + 1)
            return n if j == -1 else j + 1
        if text[i] == '"':
            return read_quoted(i)[1]
        if text[i] == "'":
            j = i + 1
            while j < n and text[j] != "'" and text[j] != '\n':
                j += 2 if text[j] == "\\" else 1
            return j + 1 if j < n and text[j] == "'" else j
        return i

    def skip_comment(i):
        if text[i + 1] == "/":
            j = text.find("\n", i)
            return n if j == -1 else j
        j = text.find("*/", i + 2)
        return n if j == -1 else j + 2

    def take_path(i):
        i = skip_ws(i)
        if i < n and text[i] == '"':
            path, i = read_quoted(i)
            if path is not None and path.startswith(ZEVER_PREFIX):
                found.add(path)
        return i

    def parse_import(i):
        i = skip_ws(i)
        if i < n and text[i] == "(":
            i += 1
            while i < n:
                c = text[i]
                if c == "`" or c == '"' or c == "'":
                    if c == '"':
                        i = take_path(i)
                    else:
                        i = skip_string(i)
                    continue
                if c == "/" and i + 1 < n and text[i + 1] in "/*":
                    i = skip_comment(i)
                    continue
                if c == ")":
                    return i + 1
                i += 1
            return i
        if i < n and (text[i].isalpha() or text[i] == "_" or text[i] == "."):
            if text[i] == ".":
                i += 1
            else:
                while i < n and (text[i].isalnum() or text[i] == "_"):
                    i += 1
            return take_path(i)
        return take_path(i)

    i = 0
    while i < n:
        c = text[i]
        if c == "`" or c == '"' or c == "'":
            i = skip_string(i)
            continue
        if c == "/" and i + 1 < n and text[i + 1] in "/*":
            i = skip_comment(i)
            continue
        if c.isalpha() or c == "_":
            j = i
            while j < n and (text[j].isalnum() or text[j] == "_"):
                j += 1
            if text[i:j] == "import":
                i = parse_import(j)
                continue
            i = j
            continue
        i += 1
    return found


def sh(cmd, cwd, **kw):
    return subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, **kw)


def find_modules():
    mods = {}  # import path -> dir (relative, "" for none... always relative here)
    for dirpath, dirnames, filenames in os.walk(ROOT):
        if ".git" in dirnames:
            dirnames.remove(".git")
        if "node_modules" in dirnames:
            dirnames.remove("node_modules")
        if "go.mod" in filenames:
            rel = os.path.relpath(dirpath, ROOT)
            out = sh(["go", "mod", "edit", "-json", "go.mod"], cwd=dirpath)
            import re as _re
            m = _re.search(r'"Module"\s*:\s*\{\s*"Path"\s*:\s*"([^"]+)"', out.stdout)
            if m:
                mods[m.group(1)] = rel
    return mods


def local_imports(moddir, mods):
    """All zever import paths in .go files under moddir, excluding nested modules."""
    nested = set()
    for dirpath, dirnames, filenames in os.walk(os.path.join(ROOT, moddir)):
        if "go.mod" in filenames and os.path.relpath(dirpath, ROOT) != moddir:
            dirnames[:] = []
            continue
    found = set()
    for dirpath, dirnames, filenames in os.walk(os.path.join(ROOT, moddir)):
        if "go.mod" in filenames and os.path.relpath(dirpath, ROOT) != moddir:
            dirnames[:] = []
            continue
        for fn in filenames:
            if not fn.endswith(".go"):
                continue
            with open(os.path.join(dirpath, fn)) as f:
                for imp in scan_imports(f.read()):
                    found.add(imp)
    return found


def provider(imp, mods):
    best = None
    for path in mods:
        if imp == path or imp.startswith(path + "/"):
            if best is None or len(path) > len(best):
                best = path
    return best


def gomod_requires(moddir):
    """Local zever paths required by moddir/go.mod (single-line and block forms)."""
    reqs = set()
    try:
        with open(os.path.join(ROOT, moddir, "go.mod")) as f:
            text = f.read()
    except OSError:
        return reqs
    for m in re.finditer(r'require\s+(github\.com/zenta-dev/zever/\S+)\s+\S+', text):
        reqs.add(m.group(1))
    for m in re.finditer(r'require\s*\((.*?)\)', text, re.S):
        for line in m.group(1).split("\n"):
            line = line.strip()
            if line.startswith("github.com/zenta-dev/zever/"):
                reqs.add(line.split()[0])
    return reqs


def gomod_replaces(moddir):
    """Local zever paths replaced by moddir/go.mod (single-line and block forms)."""
    reps = set()
    try:
        with open(os.path.join(ROOT, moddir, "go.mod")) as f:
            text = f.read()
    except OSError:
        return reps
    for m in re.finditer(r'replace\s+(github\.com/zenta-dev/zever/\S+)\s+=>', text):
        reps.add(m.group(1))
    for m in re.finditer(r'replace\s*\((.*?)\)', text, re.S):
        for line in m.group(1).split("\n"):
            line = line.strip()
            if line.startswith("github.com/zenta-dev/zever/"):
                reps.add(line.split()[0])
    return reps


def needed_closure(path, moddir, mods):
    """Direct intra-repo imports of one module (no transitive fixpoint).

    Deliberately direct-only: `go mod tidy -diff` (CI fast gate) drops
    requires nothing imports, so a transitive closure here would fight
    tidy forever. Transitive deps resolve externally via each dep's own
    go.mod (MVS); the require+replace pair here covers local/workspace
    resolution of what this module actually imports.
    """
    needed = set()
    for imp in local_imports(moddir, mods):
        prov = provider(imp, mods)
        if prov and prov != path:
            needed.add(prov)
    return needed


def lockstep_version():
    """Current lockstep version from CHANGELOG (`## [vX.Y.Z]`), for fix mode.

    New requires must carry a resolvable version (v0.0.0 placeholders
    break external `go get`); the changelog header is the one place the
    in-progress release version is written before tagging.
    """
    try:
        with open(os.path.join(ROOT, "CHANGELOG.md")) as f:
            for line in f:
                m = re.match(r"## \[(v\d+\.\d+\.\d+)\]", line.strip())
                if m:
                    return m.group(1)
    except OSError:
        pass
    return "v0.0.0"


def check_modules(mods):
    """Return {moddir: {'require': [...], 'replace': [...]}} of missing entries."""
    drift = {}
    for path in sorted(mods):
        moddir = mods[path]
        if moddir == ".":
            continue
        needed = needed_closure(path, moddir, mods)
        if not needed:
            continue
        missing_req = sorted(d for d in needed if d not in gomod_requires(moddir))
        missing_rep = sorted(d for d in needed if d not in gomod_replaces(moddir))
        if missing_req or missing_rep:
            drift[moddir] = {"require": missing_req, "replace": missing_rep}
    return drift


def main():
    global ROOT
    args = sys.argv[1:]
    do_tidy = "--tidy" in args
    do_check = "--check" in args
    if "--root" in args:
        ROOT = os.path.abspath(args[args.index("--root") + 1])
    if do_check:
        mods = find_modules()
        drift = check_modules(mods)
        if not drift:
            print(f"modules: {len(mods)}; module-graph clean")
            return 0
        print(f"modules: {len(mods)}; drift in {len(drift)} module(s):")
        for moddir in sorted(drift):
            missing = drift[moddir]
            if missing["require"]:
                print(f"{moddir}: missing require: {' '.join(missing['require'])}")
            if missing["replace"]:
                print(f"{moddir}: missing replace: {' '.join(missing['replace'])}")
        return 1
    mods = find_modules()
    print(f"modules: {len(mods)}")
    ver = lockstep_version()
    for a in args:
        if a.startswith("--version="):
            ver = a.split("=", 1)[1]
    for path in sorted(mods):
        moddir = mods[path]
        if moddir == ".":
            continue
        needed = needed_closure(path, moddir, mods)
        if not needed:
            continue
        full = os.path.join(ROOT, moddir)
        for dep in sorted(needed):
            rel = os.path.relpath(os.path.join(ROOT, mods[dep]), full)
            sh(["go", "mod", "edit", f"-require={dep}@{ver}",
                f"-replace={dep}={rel}", "go.mod"], cwd=full)
        print(f"{moddir}: +{len(needed)}")
    if do_tidy:
        env = dict(os.environ, GOWORK="off", GOPROXY="off", GOSUMDB="off")
        for path in sorted(mods):
            full = os.path.join(ROOT, mods[path])
            r = subprocess.run(["go", "mod", "tidy"], cwd=full, capture_output=True,
                               text=True, env=env)
            status = "ok" if r.returncode == 0 else "FAIL"
            extra = "" if r.returncode == 0 else " :: " + (r.stderr.strip().split("\n")[-1] if r.stderr.strip() else "?")
            print(f"tidy {mods[path]}: {status}{extra}")


if __name__ == "__main__":
    sys.exit(main())
