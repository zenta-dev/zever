#!/usr/bin/env python3
"""Ensure every module's go.mod carries requires+replaces for its full local closure.

Go ignores replace directives in dependency go.mods, so each module must
repeat require+replace for every intra-repo module it (transitively)
imports. This script computes that closure from source imports and patches
each go.mod via `go mod edit`, then runs a best-effort offline tidy.

Usage: python3 tools/fix-module-graph.py [--tidy]
  --tidy  also run `go mod tidy` per module (needs primed module cache).
"""
import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
IMPORT_RE = re.compile(r'"(github\.com/zenta-dev/zever/[^"]+)"')


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
                for imp in IMPORT_RE.findall(f.read()):
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


def main():
    do_tidy = "--tidy" in sys.argv
    mods = find_modules()
    print(f"modules: {len(mods)}")
    to_path = {v: k for k, v in mods.items()}
    for path in sorted(mods):
        moddir = mods[path]
        if moddir == ".":
            continue
        needed = set()
        for imp in local_imports(moddir, mods):
            prov = provider(imp, mods)
            if prov and prov != path:
                needed.add(prov)
        # fixpoint over local requires (transitive replaces are ignored by go)
        queue = list(needed)
        while queue:
            dep = queue.pop()
            depdir = mods.get(dep)
            if not depdir:
                continue
            for req in gomod_requires(depdir):
                if req in mods and req != path and req not in needed:
                    needed.add(req)
                    queue.append(req)
        if not needed:
            continue
        full = os.path.join(ROOT, moddir)
        for dep in sorted(needed):
            rel = os.path.relpath(os.path.join(ROOT, mods[dep]), full)
            sh(["go", "mod", "edit", f"-require={dep}@v0.0.0",
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
    main()
