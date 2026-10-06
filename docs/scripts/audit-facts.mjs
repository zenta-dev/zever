// Cross-checks docs claims against the code. Usage: node scripts/audit-facts.mjs
// Checks: YAML config keys vs JSON schema, c.<Accessor>() vs container, `zever <cmd>`/flags vs cmd/zever,
// Go import paths, env var prefixes, GitHub blob links.
import fs from 'node:fs';
import path from 'node:path';
import yaml from 'js-yaml';
import { goTruth } from './go-truth.mjs';

const repo = path.resolve('..');
const root = path.resolve('src/content/docs');
const walk = (d) => fs.readdirSync(d, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? (e.name === 'node_modules' || e.name === '.git' ? [] : walk(path.join(d, e.name))) : [path.join(d, e.name)]));
const pages = walk(root).filter((f) => /\.mdx?$/.test(f));
const read = (p) => fs.readFileSync(p, 'utf8');

// sources of truth
const schema = JSON.parse(read(path.join(repo, 'config/schema/zever.schema.json')));
const services = { ...schema.properties };
for (const k of Object.keys(goTruth())) services[k] ??= { properties: {} };
const defs = schema.$defs;
const deref = (n) => (n?.$ref ? defs[n.$ref.split('/').pop()] : n);
const truth = goTruth();
// Known option keys = Go structs (core + adapters) union the JSON schema (which may be stale).
const optionKeys = (svc) => {
  const o = deref(services[svc]?.properties?.options);
  const keys = new Set([...(truth[svc]?.keys ?? []), ...(o?.properties ? Object.keys(o.properties) : [])]);
  return keys.size ? keys : null;
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

  for (const b of blocks.filter((b) => ['sh', 'bash', 'shell', 'env', 'text'].includes(b.lang)))
    for (const m of b.code.matchAll(/^\s*([A-Z][A-Z0-9]*)_([A-Z][A-Z0-9_]*)=/gm)) {
      const svc = m[1].toLowerCase();
      if (!(svc in services) && !['go', 'cgo', 'postgres', 'redis', 'zever', 'pg', 'http', 'smtp'].includes(svc)) bad(file, `env ${m[1]}_${m[2]}: "${svc}" is not a config service`);
    }

  for (const m of text.matchAll(/github\.com\/zenta-dev\/zever\/(?:blob|tree)\/main\/([^)\s"#]+)/g)) {
    const p = m[1].replace(/\/$/, '');
    if (!fs.existsSync(path.join(repo, p))) bad(file, `GitHub link path missing in repo: ${p}`);
  }
}
console.log(`${pages.length} pages, ${yamlBlocks} yaml blocks (${yamlSkipped} unparsable, skipped)`);
console.log(`known: ${Object.keys(services).length} services, ${accessors.size} accessors, ${commands.size} commands, ${flags.size} flags`);
// Known false positives: prose that looks like a command/flag/env, and app-specific env names.
const ignore = [/flag --old \(after "zever breaking"\)/, /BOOKINGS_WEBHOOK_/, /accessor c\.Codec\(\)/, /command "zever (command|v|dev:)"/];
const real = problems.filter((p) => !ignore.some((re) => re.test(p)));
for (const p of real) console.log(p);
console.log(`${real.length} problems (${problems.length - real.length} known false positives ignored)`);
process.exit(real.length ? 1 : 0);
