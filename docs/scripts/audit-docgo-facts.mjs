// Fact-checks the package doc.go corpus against the code. Usage:
//   node docs/scripts/audit-docgo-facts.mjs [--self-test]
//
// Every battery and module ships a doc.go, and those comments make factual
// claims: env keys ("DB_MAX_CONNS caps the pool"), file cross-references
// ("See config/README.md", "see ExampleOpen in example_test.go"), adapter
// names. Nothing checked them, so config/doc.go documented a DB_MAXCONNS
// key that does not exist under that spelling, and the mistake drifted
// until the manual audit caught it (merged as PR #737). This script turns
// that class of mistake into a permanent CI failure.
//
// The scan is node-only and offline: only comment lines (`^\s*//`) of the
// doc.go files are read. Go comments hold no fences or inline-code spans,
// so tokens are taken raw. Checks:
//
// 1. Env-key-shaped tokens: `HEAD_FIELD`, `HEAD__NAME__FIELD`,
//    `HEAD_FIELD=value`.
//    - The head must be a real config service (service list, option keys
//      and adapter sets extracted by go-truth.mjs) or an ambient/
//      placeholder head admitted through the cited allowlist below. A new
//      head needs a citation into the real code, or it fails.
//    - The `__DEFAULT__` form always fails: it names an instance called
//      DEFAULT, never the default one (the flat `DB_...` spelling is the
//      default instance). The rule reuses defaultInstanceProblems from
//      audit-facts.mjs.
//    - Option fields are checked for the CANONICAL spelling, not the
//      forgiving one: the runtime matches DB_MAXCONNS and DB_MAX_CONNS
//      alike (matching ignores case and `_`), so a doc comment naming the
//      forgiving spelling still compiles and runs - and still fails here,
//      with the canonical key in the message. This is exactly the DB_MAXCONNS
//      class.
//    - `*_ADAPTER=value` values (via envKeyProblems) must be in the
//      service's extracted adapter set. The set is built from BOTH
//      registration const spellings (string literal and conversion of the
//      battery type; merged in PR #738), so IDEMPOTENCY_ADAPTER=db and
//      SESSION_ADAPTER=db are validated genuinely - no allowlist hacks.
// 2. Cross-references: file references (`config/README.md`,
//    `example_test.go`) must resolve - package dir first, then repo root -
//    and a "see SYMBOL in FILE" reference must have SYMBOL appear verbatim
//    in the package's .go files. Bare upper-case symbol prose ("Main is
//    Run plus the doctor dispatch") is NOT symbol-checked (documented
//    limitation; the stronger symbol check for markdown files lives in
//    tools/check-doc-snippets.sh), and this checker never shells out to
//    go, so it stays deterministic and offline.
//
// Findings are one-sentence, cite-backed where an exemption exists, and
// the self-test plants the mistakes the checker must reject so the guard
// is proven to bite rather than trusted to still match.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { adapterNamesFromRegister, goTruth } from './go-truth.mjs';

// The script resolves its own paths, so it runs from the repo root
// (node docs/scripts/audit-docgo-facts.mjs) and from docs/ alike.
const here = path.dirname(fileURLToPath(import.meta.url));
const repo = path.resolve(here, '..', '..');
const read = (p) => fs.readFileSync(p, 'utf8');
const exists = (p) => fs.existsSync(p);

// Sources of truth: the Go source only, exactly as audit-facts.mjs sees it.
const truth = goTruth();
const services = Object.keys(truth);

// audit-facts.mjs executes its whole docs audit at import and ends it with
// process.exit(), so a plain `import './audit-facts.mjs'` would run the
// audit inside this process and kill it before any check runs. Its
// exported envKeyProblems/defaultInstanceProblems are still imported (not
// duplicated), but lazily and behind three temporary shims, all undone in
// `finally`: console.log muted (the audit prints its counts at the end),
// process.exit a no-op (the audit's terminal exit must not kill this
// process; the real exit paths below run after the shim is gone), and
// --self-test stripped from argv (it would otherwise trigger
// audit-facts's own self-test). A failed load is reported as a problem,
// never swallowed: fail closed, so a broken helper import can never
// silently skip the env checks those helpers own.
let auditFacts = null;
let auditFactsError = null;
try {
  const realLog = console.log;
  const realExit = process.exit;
  const realArgv = process.argv;
  console.log = () => {};
  process.exit = () => {};
  process.argv = realArgv.filter((a) => a !== '--self-test');
  try {
    auditFacts = await import('./audit-facts.mjs');
  } finally {
    console.log = realLog;
    process.exit = realExit;
    process.argv = realArgv;
  }
} catch (err) {
  auditFactsError = err?.message ?? String(err);
}

// Env heads/variables that are not config services but legitimately appear
// in the corpus. Each entry admits it with a citation into the real code
// the doc.go documents, so an exemption always has a paper trail: an
// uncited head fails, and a new head needs a new citation (same seed style
// as audit-repo-facts will use). `head` admits every token under that
// service-head shape (`GRPC_*`); `token` admits the whole environment
// variable name (NO_COLOR is one shell variable, not a head/field pair).
const AMBIENT_HEADS = [
  { token: 'NO_COLOR', cite: 'adapters/log/pretty/pretty.go:52 reads NO_COLOR/FORCE_COLOR/TERM (ambient, exempt from the env-var rule)' },
  { head: 'grpc', cite: 'serve/options.go:113 (GRPC_ADDR via lookupEnv) and serve/tls.go:64 (GRPC_REFLECTION)' },
  { head: 'pprof', cite: 'serve/options.go:127 (PPROF_ADDR via lookupEnv)' },
  { head: 'zever', cite: 'serve/tls.go:69 and serve/serve.go:158 (ZEVER_ENV process globals)' },
  { head: 'postgres', cite: 'adapters/db/postgres/postgres_test.go:800 (POSTGRES_DSN integration-test gate)' },
  { head: 'prefix', cite: 'adapters/secrets/env/env.go:34 boundary()+name (PREFIX_NAME placeholder pattern in core/secrets/doc.go)' },
];
const ambient = new Set(AMBIENT_HEADS.filter((e) => e.head).map((e) => e.head));
const ambientTokens = new Set(AMBIENT_HEADS.filter((e) => e.token).map((e) => e.token));

// First file reference in a citation: used by the self-test to prove every
// allowed head and file exemption still points at real code.
const citeFile = (cite) => (cite.match(/^([\w./-]*\.\w+)/) ?? [])[1];

// File references that legitimately do not resolve to a repo path, each
// with its citation.
const FILE_ALLOWLIST = [
  { token: 'main.go', cite: 'serve/doc.go "Generated main.go contract": generated at `zever new` time, not committed (contract test serve/main_test.go)' },
  { token: 'en.json', cite: 'adapters/i18n/embed/embed.go:46 fs.ReadDir: app-supplied locale catalogs, the file names are the app\'s' },
  { token: 'fr-CA.json', cite: 'adapters/i18n/embed/embed.go:46 fs.ReadDir: app-supplied locale catalogs, the file names are the app\'s' },
  { token: 'i18n/remote/remote.go', cite: 'adapters/i18n/remote/remote.go: battery-path shorthand, the file lives under adapters/' },
];
const fileAllowed = new Map(FILE_ALLOWLIST.map((e) => [e.token, e.cite]));

// The file extensions a doc.go may name. Deliberately not "any extension":
// an unexpected one must fail instead of silently passing.
const FILE_EXTS = /^(md|go|txt|sh|mjs|ts|js|py|zen|json|ya?ml|mod|sql|tmpl|example)$/;

// clean strips the punctuation a Go comment puts around a token
// ("PERMISSION_ADAPTER).", "GRPC_ADDR,"), so the shapes key off the token.
const clean = (tok) => tok.replace(/^[^A-Za-z0-9_./-]+/, '').replace(/[^A-Za-z0-9_./-]+$/, '');

// envShape parses one cleaned token into { kind, head, name, field, value }
// or null. The first `=` separates lhs and value; the flat form is
// HEAD_FIELD and the named form is HEAD__NAME__FIELD (config/env.go's two
// namespaces).
function envShape(tok) {
  const eq = tok.indexOf('=');
  const lhs = eq === -1 ? tok : tok.slice(0, eq);
  const value = eq === -1 ? undefined : tok.slice(eq + 1);

  let m = lhs.match(/^([A-Z][A-Z0-9]*)__([A-Z][A-Z0-9]*)__([A-Z][A-Z0-9_]*)$/);
  if (m) return { kind: 'named', head: m[1], name: m[2], field: m[3], value, lhs };
  m = lhs.match(/^([A-Z][A-Z0-9]*)_([A-Z0-9_]+)$/);
  if (m) return { kind: 'flat', head: m[1], field: m[2], value, lhs };
  return null;
}

// canonFields builds, per service, two views of the extracted option keys:
// the canonical env spelling (DB_MAX_CONNS for key "max_conns", dotted
// keys flattened the way docs spell them: AUTH_JWT_TTL for "jwt.max_ttl")
// and a near-match map on the underscore-insensitive form, so a wrong
// spelling is reported differently from a wrong field.
const canonCache = new Map();
function canonFields(svc) {
  if (canonCache.has(svc)) return canonCache.get(svc);
  const exact = new Map();
  const loose = new Map();
  for (const key of truth[svc].keys ?? []) {
    const envKey = key.toUpperCase();
    if (!exact.has(envKey)) exact.set(envKey, key);
    if (!loose.has(envKey.replaceAll('_', ''))) loose.set(envKey.replaceAll('_', ''), key);
  }
  const out = { exact, loose };
  canonCache.set(svc, out);
  return out;
}

// pkgText concatenates the package's .go files (verbatim symbol presence).
// Cached per directory: the corpus spans dozens of packages and the check
// runs per doc.go file.
const pkgTextCache = new Map();
function pkgText(dir) {
  if (!pkgTextCache.has(dir)) {
    let text = '';
    try {
      for (const f of fs.readdirSync(dir).filter((f) => f.endsWith('.go')))
        text += read(path.join(dir, f)) + '\n';
    } catch {
      text = '';
    }
    pkgTextCache.set(dir, text);
  }
  return pkgTextCache.get(dir);
}

// The single exported check: pure over a source string plus the doc.go
// path relative to the repo root, reading the real tree for resolution.
// Tests feed strings, not temp dirs; every problem is
// "relFile:line: message".
export function docgoProblems(src, relFile) {
  const out = [];
  const bad = (line, msg) => out.push(`${relFile}:${line}: ${msg}`);
  if (auditFactsError) return [`${relFile}:0: audit-facts.mjs failed to import (docs node_modules missing?): ${auditFactsError}`];

  const { envKeyProblems } = auditFacts;
  const dir = path.dirname(path.join(repo, relFile));
  const dirRel = path.relative(repo, dir) || '.';

  src.split('\n').forEach((raw, idx) => {
    const line = idx + 1;
    if (!/^\s*\/\//.test(raw)) return;
    const toks = raw.split(/\s+/).map(clean).filter(Boolean);

    for (const tok of toks) {
      const shape = envShape(tok);
      if (!shape) continue;

      // Whole-token environment variables (NO_COLOR): one shell variable,
      // not a head/field pair, so no field validation either.
      if (ambientTokens.has(shape.lhs)) continue;
      const svc = shape.head.toLowerCase();

      // Canonical field spelling. audit-facts's envKeyProblems now carries
      // the same check for `=value` shapes, so this loop only owns the
      // bare-token case (no `=`) to avoid duplicate findings.
      if (svc in truth && shape.field !== 'ADAPTER' && shape.value === undefined) {
        const { exact, loose } = canonFields(svc);
        if (!exact.has(shape.field)) {
          const near = loose.get(shape.field.replaceAll('_', ''));
          bad(line, near
            ? `env ${svc.toUpperCase()}_${near.toUpperCase()}: "${shape.field}" is not the canonical spelling; the runtime matches it (case- and underscore-insensitive) to field ${near}, but the canonical key is ${svc.toUpperCase()}_${near.toUpperCase()}`
            : `env ${tok}: "${shape.field}" is not a known ${svc} option field`);
        }
      }

      // Unknown heads on BARE tokens: audit-facts's envKeyProblems only
      // sees heads on `=value` shapes, so the bare mention would otherwise
      // escape entirely.
      if (shape.value === undefined && !(svc in truth) && !ambient.has(svc))
        bad(line, shape.kind === 'named'
          ? `env ${shape.head}__${shape.name}__${shape.field}: "${svc}" is not a config service`
          : `env ${shape.head}_${shape.field}: "${svc}" is not a config service`);
    }

    // `=value` shapes: audit-facts's domain (head validity, adapter values,
    // and the __DEFAULT__ misuse from defaultInstanceProblems, which is
    // part of its output). Findings whose env token is whole-token
    // allowlisted above (NO_COLOR) or whose head is a cited ambient head
    // are dropped, in favor of the cited entries above.
    for (const p of envKeyProblems(raw)) {
      const tok = (p.match(/^env ([A-Z][A-Z0-9_]*)/) ?? [])[1];
      if (tok && ambientTokens.has(tok)) continue;
      const m = p.match(/"(\w+)" is not a config service/);
      if (m && ambient.has(m[1])) continue;
      bad(line, p);
    }

    // File references resolve against the package dir first, then the repo
    // root; allowlisted names carry citations instead of paths.
    for (const tok of toks) {
      if (tok.includes('://')) continue; // web URLs are check-links.mjs's domain
      const ext = tok.match(/(?:^|\/)([\w-]+)\.(\w+)$/);
      if (!ext || !FILE_EXTS.test(ext[2])) continue;
      if (exists(path.join(dir, tok)) || exists(path.join(repo, tok))) continue;
      if (fileAllowed.has(tok)) continue;
      bad(line, `file reference ${tok} does not resolve (tried ${dirRel}/ and the repo root)`);
    }

    // "see SYMBOL in FILE": SYMBOL must appear verbatim in some .go file
    // of the package (node-only limitation, see tools/check-doc-snippets.sh
    // for the markdown-side symbol check).
    for (const m of raw.matchAll(/(?:[Ss]ee) ([A-Z]\w*) in ([A-Za-z0-9_./-]+)\.(\w+)\b/g)) {
      const sym = m[1];
      const file = `${m[2]}.${m[3]}`;
      if (!exists(path.join(dir, file)) && !exists(path.join(repo, file))) continue; // already flagged
      const text = pkgText(dir);
      if (text && !text.includes(sym))
        bad(line, `"see ${sym} in ${file}": ${sym} does not appear verbatim in any .go file of this package`);
    }
  });

  return out;
}

// selfTest plants the mistakes this checker exists to catch and asserts it
// rejects them, plus the allowlist citations and the go-truth adapter
// sets. Same idea as audit-facts's own self-test: prove the guard bites
// instead of trusting that the regexes still match.
function selfTest() {
  const cases = [
    { name: 'a clean battery doc comment passes', relFile: 'core/cache/doc.go', src: [
        '// Package cache pools typed values.',
        '//',
        '// DB_MAX_CONNS caps the pool; DB__REPORTING__DSN names an instance and',
        '// DB_MAX_CONN_LIFETIME bounds connection age. CACHE_ADAPTER=memory selects',
        '// the default adapter: see ExampleOpen in example_test.go and',
        '// config/README.md.',
      ].join('\n'), expect: [] },
    { name: 'ambient and placeholder heads with citations pass', relFile: 'core/secrets/doc.go', src: [
        '// NO_COLOR, GRPC_ADDR, GRPC_REFLECTION, PPROF_ADDR, ZEVER_ENV are',
        '// ambient; POSTGRES_DSN gates integration tests; PREFIX_NAME is the',
        '// env backend placeholder.',
      ].join('\n'), expect: [] },
    { name: 'an impossible head (bare) is rejected', relFile: 'core/cache/doc.go', src: '// XYZ_ADAPTER\n', expect: ['"xyz" is not a config service'] },
    { name: 'an impossible head (with value) is rejected', relFile: 'core/cache/doc.go', src: '// XYZ_ADAPTER=x\n', expect: ['"xyz" is not a config service'] },
    { name: '__DEFAULT__ is rejected as a named-instance misuse', relFile: 'core/cache/doc.go', src: '// SOME__DEFAULT__FIELD\n', expect: [
        '"some" is not a config service',
        '"__DEFAULT__" is a named instance',
      ] },
    { name: 'a wrong adapter value is rejected', relFile: 'core/cache/doc.go', src: '// CACHE_ADAPTER=nope\n', expect: ['"nope" is not a cache adapter'] },
    { name: 'a broken file reference is rejected', relFile: 'core/cache/doc.go', src: '// See nope/README.md\n', expect: ['file reference nope/README.md does not resolve'] },
    { name: 'an env claim backed by a missing file is rejected', relFile: 'core/cache/doc.go', src: '// CACHE_ADAPTER is documented in docs/env-keys.md\n', expect: ['file reference docs/env-keys.md does not resolve'] },
    { name: 'DB_MAXCONNS (the #737 class) is rejected as a non-canonical spelling', relFile: 'config/doc.go', src: '// DB_MAXCONNS caps the pool.\n', expect: ['is not the canonical spelling'] },
    { name: 'an unknown option field is rejected', relFile: 'config/doc.go', src: '// DB_FROBNICATE tunes nothing.\n', expect: ['is not a known db option field'] },
  ];

  const checks = [
    { name: 'the conversion-form const feeds the session adapter set on the real tree', ok: truth.session?.adapters?.has('db') === true },
    { name: 'the conversion-form const feeds the idempotency adapter set on the real tree', ok: truth.idempotency?.adapters?.has('db') === true },
    {
      name: 'adapterNamesFromRegister parses both registration spellings',
      ok: adapterNamesFromRegister('package p\n\nconst Adapter a.Adapter = "a"\nconst Adapter b.Adapter = b.Adapter("b")\n').join(',') === 'a,b',
    },
    {
      name: 'adapterNamesFromRegister keeps a const-block conversion form with mixed case verbatim',
      ok: adapterNamesFromRegister('type Adapter string\n\nconst (\n\t// Adapter is the battery name.\n\tAdapter a.Adapter = a.Adapter("Postgres")\n)\n').join() === 'Postgres',
    },
    { name: 'the reference form ( = a.DB, from core) contributes no name', ok: adapterNamesFromRegister('const Adapter a.Adapter = a.DB\n').length === 0 },
    {
      name: 'an unparsable const Adapter throws instead of returning []',
      ok: (() => { try { adapterNamesFromRegister('const Adapter = "db"\n'); return false; } catch { return true; } })(),
    },
    { name: 'defaultInstanceProblems flags the __DEFAULT__ spelling directly', ok: auditFacts.defaultInstanceProblems('DB__DEFAULT__DSN=x\n')[0]?.includes('"__DEFAULT__" is a named instance') === true },
    { name: 'defaultInstanceProblems leaves correct flat and named keys alone', ok: auditFacts.defaultInstanceProblems('DB_DSN=x\nDB__REPORTING__DSN=y\n').length === 0 },
    { name: 'every ambient head citation names a file that exists', ok: AMBIENT_HEADS.every((e) => exists(path.join(repo, citeFile(e.cite)))) },
    { name: 'every file-allowlist citation names a file that exists', ok: FILE_ALLOWLIST.every((e) => exists(path.join(repo, citeFile(e.cite)))) },
    { name: 'service count is sane', ok: services.length >= 35 },
  ];

  let failed = 0;
  for (const c of cases) {
    const unexpected = docgoProblems(c.src, c.relFile).filter((p) => c.expect.every((want) => !p.includes(want)));
    if (unexpected.length) {
      failed++;
      console.error(`self-test FAIL: ${c.name} (unexpected: ${unexpected.join(' | ')})`);
    } else {
      console.log(`self-test ok: ${c.name}`);
    }
  }
  for (const c of checks) {
    if (!c.ok) {
      failed++;
      console.error(`self-test FAIL: ${c.name}`);
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

if (process.argv.includes('--self-test')) {
  process.exit(selfTest());
}

// The doc.go corpus: the whole tree, minus the checkout plumbing and
// dependency dirs.
function walk(d) {
  return fs.readdirSync(d, { withFileTypes: true }).flatMap((e) => {
    if (e.name === '.git' || e.name === 'node_modules' || e.name === '.worktrees') return [];
    if (!e.isDirectory()) return e.name === 'doc.go' ? [path.relative(repo, path.join(d, e.name))] : [];
    return walk(path.join(d, e.name));
  });
}

const files = walk(repo).sort();
const problems = [];
for (const rel of files) {
  let src;
  try {
    src = read(path.join(repo, rel));
  } catch {
    problems.push(`${rel}:0: unreadable`);
    continue;
  }
  problems.push(...docgoProblems(src, rel));
}
if (auditFactsError)
  problems.unshift(`audit-facts.mjs failed to import (docs node_modules missing?): ${auditFactsError}`);
// Guard the truth extraction itself: if the BOTH-forms adapter const scan
// (merged in PR #738) ever stops yielding the db adapters, *_ADAPTER values
// could no longer be validated genuinely - report that instead of failing
// to fail.
if (!truth.session?.adapters?.has('db') || !truth.idempotency?.adapters?.has('db'))
  problems.unshift('go-truth: the session/idempotency "db" adapter consts are not extracted; the BOTH-forms registration scan (PR #738) broke, so *_ADAPTER values cannot be validated genuinely');

console.log(`${files.length} doc.go files`);
for (const p of problems) console.log(p);
console.log(`${problems.length} problems`);
process.exit(problems.length ? 1 : 0);
