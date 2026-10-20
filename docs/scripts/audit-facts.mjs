// Cross-checks docs claims against the code. Usage: node docs/scripts/audit-facts.mjs
// Checks: env config keys vs the Go Options structs, c.<Accessor>() vs container,
// `zever <cmd>`/flags vs cmd/zever, Go import paths, env var prefixes, GitHub blob links.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import yaml from 'js-yaml';
import { goTruth } from './go-truth.mjs';

// The script resolves its own paths, so it runs from the repo root
// (node docs/scripts/audit-facts.mjs) and from docs/ alike.
const here = path.dirname(fileURLToPath(import.meta.url));
const repo = path.resolve(here, '..', '..');
const root = path.join(here, '..', 'src', 'content', 'docs');
const walk = (d) => fs.readdirSync(d, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? (e.name === 'node_modules' || e.name === '.git' ? [] : walk(path.join(d, e.name))) : [path.join(d, e.name)]));
const pages = walk(root).filter((f) => /\.mdx?$/.test(f));
const read = (p) => fs.readFileSync(p, 'utf8');

// sources of truth: the Go source only. Config is env-only, so there is no
// generated config schema to read; the service list and the option keys
// come from config/config.go plus the core/<svc> Options structs.
const truth = goTruth();
const services = Object.fromEntries(Object.keys(truth).map((k) => [k, { properties: {} }]));
// Known option keys = Go structs (core + adapters).
const optionKeys = (svc) => {
  const keys = truth[svc]?.keys ?? [];
  return keys.length ? new Set(keys) : null;
};
const accessors = new Set();
for (const f of fs.readdirSync(path.join(repo, 'container')).filter((f) => f.endsWith('.go') && !f.endsWith('_test.go')))
  for (const m of read(path.join(repo, 'container', f)).matchAll(/^func \(c \*Container\) ([A-Z]\w*)\(/gm)) accessors.add(m[1]);
const cmdSrc = fs.readdirSync(path.join(repo, 'cmd/zever')).filter((f) => f.endsWith('.go') && !f.endsWith('_test.go')).map((f) => read(path.join(repo, 'cmd/zever', f))).join('\n');
const commands = new Set([...cmdSrc.matchAll(/Use:\s+"([a-z][a-z:-]*)/g)].map((m) => m[1]));
const flags = new Set([...cmdSrc.matchAll(/"(--?[a-z][a-z0-9-]*)"/g)].map((m) => m[1].replace(/^-+/, '')));
for (const m of cmdSrc.matchAll(/\.(?:String|Bool|Int|Int64|Duration|Float64|StringSlice|StringVar|BoolVar|IntVar|Var)[A-Za-z]*\((?:[^,"()]*,\s*)?"([a-z][a-z0-9-]*)"/g)) flags.add(m[1]);
flags.add('help'); flags.add('version');

const problems = [];
const bad = (file, what) => problems.push(`${path.relative(root, file)}: ${what}`);
let yamlBlocks = 0, yamlSkipped = 0;

// Env heads that are not config services but legitimately appear in docs:
// toolchain names, test DSNs, and ambient shell variables.
const AMBIENT_HEADS = [
  'go', 'cgo', 'postgres', 'redis', 'zever', 'pg', 'http', 'smtp', 'pprof',
  'path', 'home', 'user', 'pwd', 'shell', 'lang', 'tmpdir', 'hostname', 'term',
];

// Env key shapes (config/env.go, container/provide.go):
//   SERVICE_FIELD          the default instance; container/provide.go maps
//                          ref.DefaultName to this flat namespace
//   SERVICE__NAME__FIELD   a named instance, wired only by a matching Ref
//
// defaultInstanceProblems flags the SERVICE__DEFAULT__FIELD spelling: a
// "__DEFAULT__" segment is a NAMED instance called DEFAULT, never the
// default one, so such a key lands on an instance nothing resolves.
//
// envKeyProblems adds the service-head and adapter-name checks on top. Both
// are exported so the self-test can plant the mistakes the checker must
// reject and assert that it does.
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
      if (!(svc in services) && !AMBIENT_HEADS.includes(svc)) {
        out.push(`env ${m[1]}_${field}: "${svc}" is not a config service`);

        continue;
      }
      if (field.toUpperCase() === 'ADAPTER' && svc in services) {
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

// selfTest plants the env-key mistakes this checker exists to catch and
// asserts it rejects them, and that the correct spellings pass. It proves
// the guard bites instead of trusting that the regex still matches.
function selfTest() {
  const cases = [
    {
      name: 'the default instance spelled flat passes',
      code: 'DB_ADAPTER=postgres\nDB_DSN=postgres://app:secret@db:5432/app\n',
      expect: 0,
    },
    {
      name: 'a named instance passes',
      code: 'DB__REPORTING__DSN=postgres://app:secret@db:5432/reporting\n',
      expect: 0,
    },
    {
      name: 'the default instance spelled __DEFAULT__ is rejected',
      code: 'DB__DEFAULT__DSN=postgres://app:secret@db:5432/app\n',
      expect: 1,
    },
    {
      name: 'the default instance spelled __DEFAULT__ with an adapter is rejected',
      code: 'AUTH__DEFAULT__ADAPTER=jwt\n',
      expect: 1,
    },
    {
      name: 'an unknown adapter name is rejected',
      code: 'CACHE_ADAPTER=postgres\n',
      expect: 1,
    },
    {
      name: 'an unknown service head is rejected',
      code: 'NOPE_ADAPTER=memory\n',
      expect: 1,
    },
  ];

  let failed = 0;
  for (const c of cases) {
    const got = envKeyProblems(c.code).length;
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

if (process.argv.includes('--self-test')) {
  process.exit(selfTest());
}

for (const file of pages) {
  const text = read(file);
  const blocks = [...text.matchAll(/```(\w+)?[^\n]*\n([\s\S]*?)```/g)].map((m) => ({ lang: m[1] ?? '', code: m[2] }));
  const prose = text.replace(/```[\s\S]*?```/g, '');

  for (const b of blocks.filter((b) => b.lang === 'yaml')) {
    yamlBlocks++;
    let doc;
    try { doc = yaml.load(b.code.replace(/\$\{[^}]+\}/g, 'x')); } catch { yamlSkipped++; continue; }
    if (!doc || typeof doc !== 'object') continue;
    for (const [svc, conf] of Object.entries(doc)) {
      if (!(svc in services)) {
        if (/^[a-z_]+$/.test(svc) && conf && typeof conf === 'object' && ('adapter' in conf || 'options' in conf)) bad(file, `yaml: unknown service "${svc}"`);
        continue;
      }
      if (!conf || typeof conf !== 'object') continue;
      const en = services[svc].properties?.adapter?.enum;
      if (conf.adapter && en && !en.includes(conf.adapter)) bad(file, `yaml: ${svc}.adapter "${conf.adapter}" not in [${en.join(', ')}]`);
      const keys = optionKeys(svc);
      if (keys && conf.options && typeof conf.options === 'object')
        for (const k of Object.keys(conf.options)) if (!keys.has(k)) bad(file, `yaml: ${svc}.options.${k} not in schema`);
    }
  }

  for (const m of text.matchAll(/\bc\.([A-Z]\w*)\(\)/g)) if (!accessors.has(m[1])) bad(file, `accessor c.${m[1]}() not in container`);

  for (const m of text.matchAll(/(?:^|[\s`$(])zever ([a-z][a-z:-]*)((?:[ \t]+(?:--?[a-z][a-z0-9-]*(?:=\S+)?|[a-z:-]+))*)/gm)) {
    const sub = m[1];
    // A token ending in ':' is an output prefix ("zever compile: ..."), not a command.
    if (sub.endsWith(':')) continue;
    if (!commands.has(sub) && !['is', 'in', 'to', 'and', 'or', 'for', 'with', 'on', 'the', 'a', 'can', 'will', 'never', 'does', 'not', 'resolves', 'compiles', 'generates', 'ships', 'uses', 'runs', 'requires', 'has', 'lets', 'skips', 'wires', 'keeps', 'treats', 'reads', 'loads', 'looks', 'checks', 'writes', 'prints', 'docs', 'as', 'from', 'that', 'by', 'at', 'if', 'it', 'so', 'now', 'only', 'stays', 'also', 'always', 'first', 'then', 'builds', 'after', 'before', 'when', 'but', 'new', 'config'].includes(sub)) bad(file, `command "zever ${sub}" not in cmd/zever`);
    for (const f of m[2].matchAll(/--?([a-z][a-z0-9-]*)/g)) if (!flags.has(f[1])) bad(file, `flag --${f[1]} (after "zever ${sub}") not found in cmd/zever`);
  }
  for (const b of blocks.filter((b) => ['sh', 'bash', 'shell', 'console'].includes(b.lang)))
    for (const l of b.code.split('\n')) {
      const m = l.match(/^\s*\$?\s*(?:[A-Z_]+=\S+\s+)*zever\s+([a-z][a-z:-]*)(.*)$/);
      if (!m) continue;
      if (!commands.has(m[1])) bad(file, `sh: "zever ${m[1]}" not in cmd/zever`);
      for (const f of m[2].matchAll(/\s--?([a-z][a-z0-9-]*)/g)) if (!flags.has(f[1])) bad(file, `sh: flag --${f[1]} not found (zever ${m[1]})`);
    }

  for (const b of blocks.filter((b) => b.lang === 'go'))
    for (const m of b.code.matchAll(/"github\.com\/zenta-dev\/zever\/([^"]+)"/g)) {
      const dir = path.join(repo, m[1]);
      if (!fs.existsSync(dir) && !fs.existsSync(path.join(repo, 'core', m[1])) ) bad(file, `go import path not found: ${m[1]}`);
    }

  // Env key shapes: the default instance is flat, a named instance uses the
  // double-underscore form, and "__DEFAULT__" is never the default one.
  for (const b of blocks.filter((b) => ['sh', 'bash', 'shell', 'env', 'text'].includes(b.lang)))
    for (const p of envKeyProblems(b.code)) bad(file, p);

  // Prose names keys as inline code spans (`DB__DEFAULT__DSN`): check the
  // namespace there too, so a wrong key cannot hide in a sentence. Only the
  // namespace is checked: prose may name a plugin head or a hypothetical
  // adapter that the code-block checks would (wrongly) reject.
  for (const span of prose.matchAll(/`([^`\n]+)`/g))
    for (const p of defaultInstanceProblems(span[1])) bad(file, p);

  for (const m of text.matchAll(/github\.com\/zenta-dev\/zever\/(?:blob|tree)\/main\/([^)\s"#]+)/g)) {
    const p = m[1].replace(/\/$/, '');
    if (!fs.existsSync(path.join(repo, p))) bad(file, `GitHub link path missing in repo: ${p}`);
  }
}
console.log(`${pages.length} pages, ${yamlBlocks} yaml blocks (${yamlSkipped} unparsable, skipped)`);
console.log(`known: ${Object.keys(services).length} services, ${accessors.size} accessors, ${commands.size} commands, ${flags.size} flags`);
// Known false positives: prose that looks like a command/flag/env, and app-specific env names.
const ignore = [
  /flag --old \(after "zever breaking"\)/,
  /BOOKINGS_WEBHOOK_/,
  /accessor c\.Codec\(\)/,
  /command "zever (command|v|dev:)"/,
  // The v0.7 -> v0.8 migration table names the removed --config flag on purpose.
  /flag --config \(after "zever doctor"\)/,
];
const real = problems.filter((p) => !ignore.some((re) => re.test(p)));
for (const p of real) console.log(p);
console.log(`${real.length} problems (${problems.length - real.length} known false positives ignored)`);
process.exit(real.length ? 1 : 0);
