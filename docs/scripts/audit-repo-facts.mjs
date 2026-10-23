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

// CLI flags, from the same source scan audit-facts.mjs uses, so a repo
// markdown claim like `zever new --bogus` is checked against the real
// flag set rather than escaping (hole #1 closure). Foreign flags that follow
// a `zever <cmd>` in prose are rare; the few that slip through are
// allowlisted below with a why.
const flags = new Set();
for (const f of fs.readdirSync(path.join(repo, 'cmd/zever')).filter((f) => f.endsWith('.go') && !f.endsWith('_test.go'))) {
  const src = read(path.join(repo, 'cmd/zever', f));
  for (const m of src.matchAll(/"(--?[a-z][a-z0-9-]*)"/g)) flags.add(m[1].replace(/^-+/, ''));
  for (const m of src.matchAll(/\.(?:String|Bool|Int|Int64|Duration|Float64|StringSlice|StringVar|BoolVar|IntVar|Var)[A-Za-z]*\((?:[^,"()]*,\s*)?"([a-z][a-z0-9-]*)"/g)) flags.add(m[1]);
}
flags.add('help'); flags.add('version');

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

const canonCacheRepo = new Map();
function canonFieldsRepo(svc) {
  if (canonCacheRepo.has(svc)) return canonCacheRepo.get(svc);
  const exact = new Set();
  const loose = new Map();
  for (const key of (truth[svc]?.keys ?? [])) {
    const envKey = key.toUpperCase();
    if (!exact.has(envKey)) exact.add(envKey);
    const stripped = envKey.replaceAll('_', '');
    if (!loose.has(stripped)) loose.set(stripped, envKey);
  }
  const out = { exact, loose };
  canonCacheRepo.set(svc, out);
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
      if (svc in truth && field.toUpperCase() !== 'ADAPTER') {
        const cf = canonFieldsRepo(svc);
        const fu = field.toUpperCase();
        if (!cf.exact.has(fu)) {
          const near = cf.loose.get(fu.replaceAll('_', ''));
          if (near)
            out.push(`env ${svc.toUpperCase()}_${near}: "${field}" is not the canonical spelling; the runtime matches it (case- and underscore-insensitive) to field ${near}, but the canonical key is ${svc.toUpperCase()}_${near}`);
        }
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
  // The serve runner reads these over the process env directly; `grpc` is
  // not a config service. serve/options.go:99/113/127 (PORT, GRPC_ADDR,
  // PPROF_ADDR -- PORT/PPROF are ambient heads anyway), serve/tls.go:64
  // (GRPC_REFLECTION), serve/serve.go:490 (listens on GRPC_ADDR).
  { dir: '', re: /^env GRPC_(?:ADDR|REFLECTION): "[a-z]+" is not a config service$/, why: 'serve-runtime keys the code reads: serve/options.go:113 lookupEnv("GRPC_ADDR"), serve/tls.go:64 lookupEnv("GRPC_REFLECTION")' },
  // The ws2 evaluation built a demo tree that was scaffolded in the
  // workspace and pasted from, never committed under this path.
  { dir: 'docs/plans/', re: /^path schema\/app\.zen: /, why: 'the ws2 record names the demo checkout built in the PR workspace (docs/plans/pr-ws2.md), not a repo file' },
  // Release-pipeline outputs generated by CI at publish time.
  { dir: 'docs/plans/', re: /^path dist\/SHA256SUMS\.txt: /, why: 'release artifact written by the publish job (docs/plans/release-cosign-sbom-draft.md), not checked in' },
  // Paths inside a freshly scaffolded zever project (cmd/zever/cmd_scaffold.go:22
  // lists "a fresh go.mod, a starter schema/app.zen, cmd/server/main.go";
  // cmd/zever/add.go:28 edits internal/app/wiring.go) — the docs name the
  // tree zever generates, which is not checked in here.
  { dir: '', re: /^path (?:schema\/app\.zen|cmd\/server\/main\.go|internal\/app\/wiring\.go): /, why: 'the layout zever scaffolds into a NEW project (cmd/zever/cmd_scaffold.go:22, cmd/zever/add.go:28)' },
  // Codegen output written by compile --out ./generated; not in the repo.
  { dir: '', re: /^path generated\/zenorm\/orm\/gen\/app\/app\.go: /, why: 'output of `go run ./cmd/zever compile --out ./generated` (README.md excerpts it), not checked in' },
  // make test-flake passes FLAKE_COUNT to `go test -count` (Makefile:267,
  // .github/workflows/flake.yml:37), so it is toolchain plumbing, not config.
  { dir: '.github/', re: /^env FLAKE_COUNT: "/, why: 'make test-flake passes it to go test -count (Makefile:267, .github/workflows/flake.yml:37)' },
  // Records of things removed on purpose keep the removed spelling.
  { dir: '.changelog/unreleased/', re: /^env VECTORSTORE_ADAPTER: "pgvector" is not a vectorstore adapter /, why: 'the fragment records the removed non-canonical spelling on purpose (fix-cli-canonical-adapters.md)' },
  // `zever db rollback --count n` documents the legacy stdlib flagset flag;
  // --count is not on the cobra help tree (see cmd_facts_allow.json).
  { dir: 'orm/', re: /^flag --count \(after "zever db"\) not in cmd\/zever$/, why: 'legacy stdlib flagset flag for `zever db rollback --count` (orm/DIALECTS.md)' },
  { dir: '.changelog/unreleased/', re: /^path \.github\/zever\.jpg: /, why: 'the fragment records deleting that asset; it is gone by design (chore-logo-update.md)' },
];

// allowlisted classifies one finding of one file.
function allowlisted(rel, msg) {
  return ALLOW.some((a) => rel.startsWith(a.dir) && a.re.test(msg));
}

// --- check functions ----------------------------------------------------

const blocks = (text) => [...text.matchAll(/```(\w+)?[^\n]*\n([\s\S]*?)```/g)].map((m) => ({ lang: m[1] ?? '', code: m[2], i: m.index }));
const spans = (text) => [...text.matchAll(/`([^`\n]+)`/g)].map((m) => ({ t: m[1], i: m.index }));
const lineOf = (text, i) => text.slice(0, i).split('\n').length;

// blockLines yields the lines of a fenced block with their absolute line
// numbers: the opener sits on lineOf(text, b.i), the code on the next line
// onward.
function blockLines(text, b) {
  const base = lineOf(text, b.i);
  return b.code.split('\n').map((code, k) => ({ code, line: base + 1 + k }));
}

// Each check takes a full md text (checkPaths additionally the file path)
// and returns [{ line, msg }], so the runner prefixes the repo-relative
// path once and every finding keeps its original file:line.

// checkEnv runs check 1 over every fenced block AND every inline code span,
// one line at a time so a finding reports the line it sits on. Spans that
// name a key without a value (`DB_DSN` alone) never match the assignment
// regex, so prose can keep naming keys freely.
export function checkEnv(text) {
  const out = [];
  const feed = (code, line) => {
    for (const p of envKeyProblems(code)) out.push({ line, msg: p });
  };
  for (const b of blocks(text).filter((b) => ['sh', 'bash', 'shell', 'env', 'text'].includes(b.lang)))
    for (const l of blockLines(text, b)) feed(l.code, l.line);
  for (const s of spans(text)) feed(s.t, lineOf(text, s.i));

  return out;
}

export function checkAccessors(text) {
  const out = [];
  for (const m of text.matchAll(/\bc\.([A-Z]\w*)\(\)/g))
    if (!accessors.has(m[1])) out.push({ line: lineOf(text, m.index), msg: `accessor c.${m[1]}() is not a container method` });

  return out;
}

// Words that follow "zever" in prose without naming a subcommand. The base
// list is the one audit-facts.mjs uses for the site; each addition below
// was read in context (a "zever <word>" that is an English sentence, a
// trigger phrase, or a version mention, not a command) during the first
// repo-wide run, and the phrase it excuses is named.
const PROSE_STOP = [
  // audit-facts.mjs's prose list
  'is', 'in', 'to', 'and', 'or', 'for', 'with', 'on', 'the', 'a', 'an', 'can',
  'will', 'never', 'does', 'not', 'resolves', 'compiles', 'generates',
  'ships', 'uses', 'runs', 'requires', 'has', 'lets', 'skips', 'wires',
  'keeps', 'treats', 'reads', 'loads', 'looks', 'checks', 'writes', 'prints',
  'as', 'from', 'that', 'by', 'at', 'if', 'it', 'so', 'now', 'only', 'stays',
  'also', 'always', 'first', 'then', 'builds', 'after', 'before', 'when',
  'but', 'new', 'config',
  // repo prose: sentences and trigger phrases
  'fits', 'compares', 'instead', 'side', 'guarantees', 'cannot', // "how zever fits together", "Pick zever instead when...", "zever guarantees"
  'framework', // "built on the zever framework" (example READMEs)
  'app', 'skill', 'application', 'binary', 'modules', // "scaffold zever app", "# zever skill", "won't work for zever modules"
  'v', 'vs', 'import', 'versions', 'needed', // "(zever v0.7.1)", "zever vs hand-written SQL", "published zever versions"
  'schema', // "the zever schema language" (dsl/README.md)
  'repo', // "the runner image the zever repo uses" (changelog fragment)
  'differs', 'scaffolds', 'works', 'packages', // README.md prose ("zever differs by ...", "How zever works")
  'version', // "a published zever version"; the CLI's version is the --version/-V flag, not a subcommand
];
// cobra registers `help` implicitly; it is runnable even with no Use line.
commands.add('help');

// fenceMask marks every offset that sits inside a ``` fence, so the prose
// scans (plain-text commands, spans, paths, versions) skip what the block
// scans already checked, keeping the ORIGINAL offsets -- and therefore the
// ORIGINAL line numbers -- intact.
function fenceMask(text) {
  const fence = new Uint8Array(text.length);
  for (const m of text.matchAll(/^```[^\n]*\n([\s\S]*?)^```/gm))
    for (let i = m.index; i < m.index + m[0].length; i++) fence[i] = 1;
  return (i) => i >= 0 && i < text.length && fence[i] === 1;
}

export function checkCommands(text) {
  const out = [];
  const reject = (sub, line) => {
    // A token ending in ':' is an output prefix ("zever compile: ...").
    if (sub.endsWith(':')) return;
    if (!commands.has(sub) && !PROSE_STOP.includes(sub)) out.push({ line, msg: `command "zever ${sub}" not in cmd/zever` });
  };
  for (const b of blocks(text).filter((b) => ['sh', 'bash', 'shell', 'console'].includes(b.lang)))
    for (const l of blockLines(text, b)) {
      const m = l.code.match(/^\s*\$?\s*(?:[A-Z_]+=\S+\s+)*zever\s+([a-z][a-z:-]*)/);
      if (m) reject(m[1], l.line);
    }
  const fenced = fenceMask(text);
  for (const m of text.matchAll(/(?:^|[\s`$(])zever ([a-z][a-z:-]*)/gm)) {
    if (fenced(m.index)) continue; // the block loop owns fenced lines
    reject(m[1], lineOf(text, m.index));
  }

  return out;
}

// checkFlags validates `--flag` tokens that follow a `zever <sub>` mention
// against the real flag set extracted from cmd/zever (hole #1 closure: repo
// markdown such as examples/*/README.md now fails on a bogus flag, instead
// of escaping because cmd_facts_docs_test only reads three files).
export function checkFlags(text) {
  const out = [];
  for (const m of text.matchAll(/(?:^|[\s`$(])zever ([a-z][a-z:-]*)((?:[ \t]+(?:--?[a-z][a-z0-9-]*(?:=\S+)?|[a-z:-]+))*)/gm)) {
    const sub = m[1];
    if (sub.endsWith(':')) continue;
    if (!commands.has(sub)) continue;
    for (const f of m[2].matchAll(/\s--?([a-z][a-z0-9-]*)/g))
      if (!flags.has(f[1])) out.push({ line: lineOf(text, m.index), msg: `flag --${f[1]} (after "zever ${sub}") not in cmd/zever` });
  }
  for (const b of blocks(text).filter((b) => ['sh', 'bash', 'shell', 'console'].includes(b.lang)))
    for (const l of blockLines(text, b)) {
      const m = l.code.match(/^\s*\$?\s*(?:[A-Z_]+=\S+\s+)*zever\s+([a-z][a-z:-]*)(.*)$/);
      if (!m) continue;
      for (const f of m[2].matchAll(/\s--?([a-z][a-z0-9-]*)/g))
        if (!flags.has(f[1])) out.push({ line: l.line, msg: `sh: flag --${f[1]} not found (zever ${m[1]})` });
    }
  return out;
}

// Shapes that look like paths but are not single resolvable files. Named
// here so the rule stays mechanical: globs (*), copy-templates (<battery>)
// and alternation lists (a|b) name many files, and .pb.go tokens are
// monorepo lore for generated protoc output.
const PATH_EXCLUDE = [
  /\.\.pb\.go$/,
  /(?:^|\/)data\/[^/]*\.db$/, // example apps create their sqlite at runtime
];

// checkPaths resolves `/`-containing spans whose last segment ends in a
// lowercase extension (a file name). Skipped out of hand: tokens with
// template/glob characters (`*`, `<`, `>`) name many files, not one.
export function checkPaths(text, file) {
  const out = [];
  const dir = path.dirname(file);
  const fenced = fenceMask(text);
  for (const s of spans(text)) {
    if (fenced(s.i)) continue; // fenced code is not repo-relative prose
    const t = s.t;
    if (!t.includes('/') || /\s/.test(t)) continue;
    if (/[*<>|]/.test(t)) continue; // a glob, a copy-template, an alternation: not one file
    if (t.includes('=')) continue; // an env assignment (DB_PATH=data/app.db), not a path
    if (/\/\./.test(t)) continue; // a slash-separated extension group (`zever.yaml/.yml/.json`)
    if (/^\w+:\/\//.test(t) || t.startsWith('/')) continue; // URLs, absolute paths
    // Extension-bearing: the LAST segment must end in a lowercase
    // extension, or the span is a Go symbol (`orm/fts/sqlite.Match`) or a
    // bare path with no file at the end.
    const last = t.split('/').pop();
    if (!/\.[a-z0-9]{1,6}$/.test(last)) continue;
    if (PATH_EXCLUDE.some((r) => r.test(t))) continue;
    // First beside the md file (a doc may carry its own files), then at
    // the repo root (module READMEs mostly name repo paths).
    if ([dir, repo].some((base) => fs.existsSync(path.resolve(base, t)))) continue;
    out.push({ line: lineOf(text, s.i), msg: `path ${t}: not found beside this file or at the repo root` });
  }

  return out;
}

// checkVersions audits every `go 1.2x` / `1.2x` toolchain claim in prose.
// A mention matching the go.work minor passes; ANY other minor is drift,
// floor claims (`1.25+`) included -- the repo builds with go.work's pin,
// so "supported from 1.25" is not true of this tree. A `1.27+`-style
// un-pinned mention of the current minor passes too.
export function checkVersions(text) {
  const out = [];
  const fenced = fenceMask(text);
  for (const m of text.matchAll(/\bgo(?:1\.| )?1\.(2[0-9])(\.[0-9]+)?/gi)) {
    if (fenced(m.index)) continue;
    const minor = Number(m[1]);
    if (minor === Number(goMinor[2])) continue; // matches the pinned toolchain
    out.push({
      line: lineOf(text, m.index),
      msg: `toolchain pin: reads 1.${m[1]} but go.work pins go ${goVersion}`,
    });
  }

  return out;
}

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
    { name: 'env: a non-canonical field spelling is rejected', run: () => checkEnv('```sh\nDB_MAXCONNS=20\n```'), expect: 1 },
    { name: 'env: the canonical field spelling passes', run: () => checkEnv('```sh\nDB_MAX_CONNS=20\n```'), expect: 0 },
    { name: 'accessor: a bogus method is rejected', run: () => checkAccessors('call `c.Nonexistent()` at boot'), expect: 1 },
    { name: 'accessor: a real method passes', run: () => checkAccessors('the handler receives `c.DB()`'), expect: 0 },
    { name: 'command: a bogus subcommand is rejected', run: () => checkCommands('```sh\nzever frobnicate\n```'), expect: 1 },
    { name: 'command: a real subcommand passes', run: () => checkCommands('```sh\n$ zever serve\n```'), expect: 0 },
    { name: 'flag: a bogus flag after zever is rejected', run: () => checkFlags('run `zever new --nonexistent-xyz`'), expect: 1 },
    { name: 'flag: a real flag passes', run: () => checkFlags('run `zever compile --backend go`'), expect: 0 },
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
  emit(checkFlags(text));
  emit(checkPaths(text, file));
  emit(checkVersions(text));
}
for (const p of problems) console.log(p);
console.log(`${files.length} files scanned, ${accessors.size} accessors, ${commands.size} commands, go ${goVersion}`);
console.log(`${problems.length} problems`);
process.exit(problems.length ? 1 : 0);
