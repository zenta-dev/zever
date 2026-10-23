// Fact-checks the ~150 repository markdown files against the Go code.
// Usage: node docs/scripts/audit-repo-facts.mjs [--self-test]  (exit 1 on problems)
//
// The docs SITE (docs/src/content/docs) is audited by audit-facts.mjs; the
// repo's own READMEs, contributor docs, changelog fragments and design notes
// were not audited by anything, so commands, env keys, container accessors,
// file paths and Go-version pins drift there freely. This script gives them
// the same treatment, against the same ground truth.
//
// Checks, all reported with file:line:
//   1. env keys        -- fenced sh/bash/shell/env/text blocks AND inline code
//                         spans; the same key-shape rules audit-facts uses
//   2. c.X() accessors -- the method must be a container method
//   3. zever <sub>     -- the subcommand must exist in cmd/zever
//   4. backticked paths-- a `/`-containing, extension-bearing token must
//                         resolve beside the md file or at the repo root
//   5. version pins    -- a `1.2x` toolchain claim must not drift from
//                         go.work's `go` directive
//
// Deliberately NOT checked: `zever <sub> --flags`. cmd/zever/cmd_facts_docs_test.go
// (PR #740) already pins the CLI flag surface as a golden and gates the SITE
// docs' flags against it, and those generated command pages are linked from
// the site, not from the repo's markdown. Validating flags here too would
// duplicate that golden's upkeep for no extra coverage.
//
// The script resolves its own paths, so it runs from the repo root
// (node docs/scripts/audit-repo-facts.mjs) and from docs/ alike.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { goTruth } from './go-truth.mjs';

const here = path.dirname(fileURLToPath(import.meta.url));
const repo = path.resolve(here, '..', '..');
const read = (p) => fs.readFileSync(p, 'utf8');

// sources of truth. goTruth() is the shared ground truth (service list, per-
// service option keys, adapter names) derived from config/config.go and the
// core/<svc> Options structs; audit-facts.mjs feeds the same object to its
// site checks, so this checker cannot disagree with it on what is real.
const truth = goTruth();

// The container accessor names, from the non-test source only: a mention in
// a *_test.go file is not something repo markdown should have to match.
const accessors = new Set();
for (const f of fs.readdirSync(path.join(repo, 'container')).filter((f) => f.endsWith('.go') && !f.endsWith('_test.go')))
  for (const m of read(path.join(repo, 'container', f)).matchAll(/^func \(c \*Container\) ([A-Z]\w*)\(/gm)) accessors.add(m[1]);

// The CLI subcommands, from the cobra Use lines (the same scan audit-facts
// uses). Flags are not collected here -- see the header note above.
const commands = new Set();
for (const f of fs.readdirSync(path.join(repo, 'cmd/zever')).filter((f) => f.endsWith('.go') && !f.endsWith('_test.go')))
  for (const m of read(path.join(repo, 'cmd/zever', f)).matchAll(/Use:\s+"([a-z][a-z:-]*)/g)) commands.add(m[1]);

// go.work's `go` directive is the only pinned toolchain claim in the repo.
const goVersion = /^go (\S+)/m.exec(read(path.join(repo, 'go.work')))?.[1];
if (!goVersion) throw new Error('go.work: no `go` directive found');
const goMinor = /(\d+)\.(\d+)/.exec(goVersion);

// --- env keys -----------------------------------------------------------

// Env heads that are not config services but legitimately appear in docs.
// Copied from audit-facts.mjs so both checkers excuse the same heads.
const AMBIENT_HEADS = [
  'go', 'cgo', 'postgres', 'redis', 'zever', 'pg', 'http', 'smtp', 'pprof',
  'path', 'home', 'user', 'pwd', 'shell', 'lang', 'tmpdir', 'hostname', 'term',
];

// envKeyProblems / defaultInstanceProblems are a verbatim port of the
// exported helpers in audit-facts.mjs (lines 61-97). They cannot simply be
// imported from there: that module runs its whole site audit at top level
// and ends with process.exit (line 238), so importing it would terminate
// this script after running the other check; and it imports js-yaml, which
// lives in the untracked docs/node_modules, so a fresh worktree cannot even
// resolve it. The service/adapter/adapters ground truth still comes from the
// shared goTruth(), so both checkers read from one source.
//
// Env key shapes (config/env.go, container/provide.go):
//   SERVICE_FIELD          the default instance
//   SERVICE__NAME__FIELD   a named instance, wired only by a matching Ref
//
// defaultInstanceProblems flags the SERVICE__DEFAULT__FIELD spelling: that
// is a NAMED instance called DEFAULT, so the value lands on an instance
// nothing resolves.
export function defaultInstanceProblems(code) {
  const out = [];
  for (const tok of tokens(code))
    for (const m of tok.matchAll(/^([A-Z][A-Z0-9]*)__([A-Z][A-Z0-9]*)__([^=\s]+)(=\S*)?$/g))
      if (m[2].toUpperCase() === 'DEFAULT')
        out.push(`env ${m[1]}__${m[2]}__${m[3]}: "__DEFAULT__" is a named instance, not the default one; the default instance reads ${m[1]}_${m[3]}`);

  return out;
}

export function envKeyProblems(code) {
  const out = [];
  for (const tok of tokens(code))
    for (const m of tok.matchAll(/^([A-Z][A-Z0-9]*)(?:(?:__[A-Z][A-Z0-9]*__)|_)([A-Z][A-Z0-9_]*)=(\S*)$/g)) {
      const svc = m[1].toLowerCase();
      const field = m[2];
      const value = m[3];
      if (!(svc in truth) && !AMBIENT_HEADS.includes(svc)) {
        out.push(`env ${m[1]}_${field}: "${svc}" is not a config service`);

        continue;
      }
      if (field.toUpperCase() === 'ADAPTER' && svc in truth) {
        const known = [...(truth[svc]?.adapters ?? [])];
        if (known.length && !known.includes(value))
          out.push(`env ${m[1]}_ADAPTER: "${value}" is not a ${svc} adapter (known: ${known.join(', ')})`);
      }
    }

  return [...out, ...defaultInstanceProblems(code)];
}

// tokens splits a shell-ish block into the whitespace-separated words that
// can carry an env assignment (`DB_DSN=... zever serve` holds two).
function tokens(code) {
  return code.split('\n').flatMap((l) => l.trim().split(/\s+/)).filter(Boolean);
}

// --- the allowlist ------------------------------------------------------

// Findings this checker excuses. Every entry is FILE-SCOPED (the regex must
// also be tested against the path of the file the finding came from, so a
// general finding is not silenced repo-wide) and carries a one-line why.
// Entries are seeded only with hits the real run on the merged main tree
// proved legitimate and that the docs cannot express more narrowly; a hit
// that has a code citation goes in as a documented entry, everything else
// got fixed in the docs instead.
const ALLOW = [
  // examples/bookings is a self-standing app, not a zever service: its
  // webhook target/secret names are read by that app's own code.
  { dir: 'examples/', re: /^env BOOKINGS_WEBHOOK_(?:CREATED|CANCELLED)_(?:TARGET|SECRET): "[a-z]+" is not a config service$/, why: 'the bookings example app reads its own BOOKINGS_WEBHOOK_* namespace (examples/bookings/README.md)' },
];

// allowlisted classifies one finding of one file.
function allowlisted(rel, msg) {
  return ALLOW.some((a) => rel.startsWith(a.dir) && a.re.test(msg));
}

// --- check functions ----------------------------------------------------

// Each check takes (text, fileDir) and returns [{ line, msg }], so the
// runner can prefix the repo-relative path once.
const blocks = (text) => [...text.matchAll(/```(\w+)?[^\n]*\n([\s\S]*?)```/g)].map((m) => ({ lang: m[1] ?? '', code: m[2], i: m.index }));
const spans = (text) => [...text.matchAll(/`([^`\n]+)`/g)].map((m) => ({ t: m[1], i: m.index }));
const lineOf = (text, i) => text.slice(0, i).split('\n').length;

// checkEnv runs check 1 over every fenced block AND every inline code span.
// Spans that name a key without a value (`DB_DSN` alone) never match the
// assignment regex, so prose can keep naming keys freely.
export function checkEnv(text) {
  const out = [];
  const feed = (code, i) => {
    for (const p of envKeyProblems(code)) out.push({ line: lineOf(text, i), msg: p });
  };
  for (const b of blocks(text).filter((b) => ['sh', 'bash', 'shell', 'env', 'text'].includes(b.lang))) feed(b.code, b.i);
  for (const s of spans(text)) feed(s.t, s.i);

  return out;
}

export function checkAccessors(text) {
  const out = [];
  for (const m of text.matchAll(/\bc\.([A-Z]\w*)\(\)/g))
    if (!accessors.has(m[1])) out.push({ line: lineOf(text, m.index), msg: `accessor c.${m[1]}() is not a container method` });

  return out;
}

// Words that follow "zever" in prose without being a subcommand. Same list
// audit-facts.mjs uses for the site.
const PROSE_STOP = ['is', 'in', 'to', 'and', 'or', 'for', 'with', 'on', 'the', 'a', 'an', 'can', 'will', 'never', 'does', 'not', 'resolves', 'compiles', 'generates', 'ships', 'uses', 'runs', 'requires', 'has', 'lets', 'skips', 'wires', 'keeps', 'treats', 'reads', 'loads', 'looks', 'checks', 'writes', 'prints', 'as', 'from', 'that', 'by', 'at', 'if', 'it', 'so', 'now', 'only', 'stays', 'also', 'always', 'first', 'then', 'builds', 'after', 'before', 'when', 'but', 'new', 'config'];

export function checkCommands(text) {
  const out = [];
  const reject = (sub, line) => {
    // A token ending in ':' is an output prefix ("zever compile: ...").
    if (sub.endsWith(':')) return;
    if (!commands.has(sub) && !PROSE_STOP.includes(sub)) out.push({ line, msg: `command "zever ${sub}" not in cmd/zever` });
  };
  for (const b of blocks(text).filter((b) => ['sh', 'bash', 'shell', 'console'].includes(b.lang)))
    for (const l of b.code.split('\n')) {
      const m = l.match(/^\s*\$?\s*(?:[A-Z_]+=\S+\s+)*zever\s+([a-z][a-z:-]*)/);
      if (m) reject(m[1], lineOf(text, b.i));
    }
  const prose = text.replace(/```[\s\S]*?```/g, '');
  for (const m of prose.matchAll(/(?:^|[\s`$(])zever ([a-z][a-z:-]*)/gm)) reject(m[1], lineOf(prose, m.index));

  return out;
}

// Monorepo lore and runtime artefacts that are not files this checker can
// resolve. Named here because the path rule must stay mechanical.
const PATH_EXCLUDE = [
  /\.\.pb\.go$/, // protoc output referenced historically, not checked in
  /(?:^|\*\/)data\/[^/]*\.db$/, // example apps create their sqlite at runtime
];

export function checkPaths(text, file) {
  const out = [];
  const dir = path.dirname(file);
  const prose = text.replace(/```[\s\S]*?```/g, '');
  for (const s of spans(prose)) {
    const t = s.t;
    if (!t.includes('/') || /\s/.test(t)) continue;
    if (/^\w+:\/\//.test(t) || t.startsWith('/')) continue; // URLs, absolute paths
    if (!/\.\w{1,6}$/.test(t) || PATH_EXCLUDE.some((r) => r.test(t))) continue;
    // First beside the md file (a doc may carry its own files), then at
    // the repo root (module READMEs mostly name repo paths).
    if ([dir, repo].some((base) => fs.existsSync(path.resolve(base, t)))) continue;
    out.push({ line: lineOf(prose, s.i), msg: `path ${t}: not found beside this file or at the repo root` });
  }

  return out;
}

export function checkVersions(text) {
  const out = [];
  const prose = text.replace(/```[\s\S]*?```/g, '');
  for (const m of prose.matchAll(/\bgo(?:1\.| )?1\.(2[0-9])(\.[0-9]+)?(?:\.\.)?/gi)) {
    const minor = Number(m[1]);
    if (minor === Number(goMinor[2])) continue; // matches the pinned toolchain
    // A floor claim (`go 1.25+`, `1.26 and later`) still pins an older
    // minor than go.work requires, so it is drift here too.
    out.push({
      line: lineOf(prose, m.index),
      msg: `toolchain pin ${goVersion}: reads 1.${m[1]} but go.work pins go ${goVersion}`,
    });
  }

  return out;
}

// `1.27+`-style un-pinned mentions pass: the regex above stops before the
// operator, and the `+` is left unconsumed on purpose.

// --- the scanned set ----------------------------------------------------

// All repo markdown, minus the trees that are audited elsewhere or are
// build output: the SITE (docs/src) is audit-facts.mjs's job, docs/dist is
// generated, docs/superpowers and CHANGELOG.md are records by design, and
// testdata holds fixtures any whole-repo walk must never read.
const SKIP_DIRS = new Set(['.git', '.worktrees', 'node_modules', 'testdata']);
const SKIP_PREFIX = ['docs/src', 'docs/dist', 'docs/superpowers'];

export function scannedFiles() {
  const out = [];
  const walk = (d) =>
    fs.readdirSync(d, { withFileTypes: true }).flatMap((e) => {
      if (e.isDirectory()) return SKIP_DIRS.has(e.name) ? [] : walk(path.join(d, e.name));
      return e.name.endsWith('.md') ? [path.join(d, e.name)] : [];
    });
  for (const f of walk(repo)) {
    const rel = path.relative(repo, f);
    if (rel === 'CHANGELOG.md' || SKIP_PREFIX.some((p) => rel.startsWith(p))) continue;
    out.push(rel);
  }

  return out.sort();
}

// --- the self-test ------------------------------------------------------

// selfTest plants the mistakes each check exists to catch and asserts they
// are rejected, plus one clean case per check that must stay silent. It
// proves the guard bites instead of trusting that the regexes still match.
// Pattern: audits-facts.mjs and no-legacy-config.mjs export their check
// functions for exactly this reason; no-legacy-config.mjs's self-test (its
// "the checker bites" comment) is the shape this follows.
function selfTest() {
  let failed = 0;
  const cases = [
    { name: 'env: a bogus service head is rejected', run: () => checkEnv('```sh\nNOPEX_ADAPTER=x\n```'), expect: 1 },
    { name: 'env: a real default-instance key passes', run: () => checkEnv('```sh\nDB_ADAPTER=postgres\n```'), expect: 0 },
    { name: 'env: a __DEFAULT__ spelling is rejected', run: () => checkEnv('set `DB__DEFAULT__DSN=x`'), expect: 1 },
    { name: 'env: a named instance in prose passes', run: () => checkEnv('set `DB__REPORTING__DSN=...`'), expect: 0 },
    { name: 'env: a valueless key named in prose passes', run: () => checkEnv('set `DB_DSN` yourself'), expect: 0 },
    { name: 'accessor: a bogus method is rejected', run: () => checkAccessors('call `c.Nonexistent()` at boot'), expect: 1 },
    { name: 'accessor: a real method passes', run: () => checkAccessors('the handler receives `c.DB()`'), expect: 0 },
    { name: 'command: a bogus subcommand is rejected', run: () => checkCommands('```sh\nzever frobnicate\n```'), expect: 1 },
    { name: 'command: a real subcommand passes', run: () => checkCommands('```sh\n$ zever serve\n```'), expect: 0 },
    { name: 'path: a broken path is rejected', run: () => checkPaths('see `nope/nope.go`', import.meta.filename), expect: 1 },
    { name: 'path: a real repo path passes', run: () => checkPaths('see `config/config.go`', import.meta.filename), expect: 0 },
    { name: 'path: an import-path span without an extension passes', run: () => checkPaths('import `github.com/zenta-dev/zever/orm`', import.meta.filename), expect: 0 },
    { name: 'version: an old pin is rejected', run: () => checkVersions('needs go 1.25 or newer'), expect: 1 },
    { name: 'version: the pinned version passes', run: () => checkVersions(`built with go ${goVersion}`), expect: 0 },
    { name: 'version: a floor of the pinned minor passes', run: () => checkVersions('requires go 1.27+'), expect: 0 },
  ];
  // The __DEFAULT__ case above has expect=1 (defaultInstanceProblems fires
  // even without a value); the named-instance prose span must pass.
  for (const c of cases) {
    const got = c.run().length;
    if (got !== c.expect) {
      failed++;
      console.error(`self-test FAIL: ${c.name}: got ${got} problems, want ${c.expect}`);
    } else {
      console.log(`self-test ok: ${c.name}`);
    }
  }
  if (failed) {
    console.error(`self-test FAIL: ${failed} case(s) wrong`);

    return 1;
  }
  console.log('self-test PASS');

  return 0;
}

if (process.argv.includes('--self-test')) process.exit(selfTest());

// --- the run ------------------------------------------------------------

const files = scannedFiles();
const problems = [];
for (const rel of files) {
  const file = path.join(repo, rel);
  const text = read(file);
  const emit = (c) => {
    for (const p of c) {
      if (allowlisted(rel, `${p.msg}`)) continue;
      problems.push(`${rel}:${p.line}: ${p.msg}`);
    }
  };
  emit(checkEnv(text));
  emit(checkAccessors(text));
  emit(checkCommands(text));
  emit(checkPaths(text, file));
  emit(checkVersions(text));
}
for (const p of problems) console.log(p);
console.log(`${files.length} files scanned, ${accessors.size} accessors, ${commands.size} commands, go ${goVersion}`);
console.log(`${problems.length} problems`);
process.exit(problems.length ? 1 : 0);
