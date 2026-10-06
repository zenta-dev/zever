// Builds ground truth for config services from the Go source (config.go + core/<svc> Options structs).
// Usage: node scripts/go-truth.mjs > truth.json   (imported by audit-facts.mjs)
import fs from 'node:fs';
import path from 'node:path';

const repo = path.resolve('..');
const read = (p) => fs.readFileSync(p, 'utf8');

export function goTruth() {
  const cfg = read(path.join(repo, 'config/config.go'));
  const services = {};
  for (const m of cfg.matchAll(/^\s*\w+\s+Service\[(\w+)\.Options\]\s+`json:"([a-z0-9_]+)"/gm)) services[m[2]] = { pkg: m[1], keys: new Set() };
  const structs = (pkg) => {
    const dir = path.join(repo, 'core', pkg);
    const out = {};
    if (!fs.existsSync(dir)) return out;
    for (const f of fs.readdirSync(dir).filter((f) => f.endsWith('.go') && !f.endsWith('_test.go')))
      for (const m of read(path.join(dir, f)).matchAll(/type (\w+) struct \{([\s\S]*?)\n\}/g)) out[m[1]] = m[2];
    return out;
  };
  const keysOf = (body, all, depth, prefix, seen) => {
    const keys = [];
    for (const l of body.split('\n')) {
      const m = l.match(/^\s*(\w+)\s+([\w.*\[\]]+)\s+`[^`]*json:"([a-z0-9_]+)/);
      if (!m) continue;
      const [, , type, name] = m;
      keys.push(prefix + name);
      const t = type.replace(/^[*\[\]]+/, '');
      if (depth < 2 && all[t] && !seen.has(t)) keys.push(...keysOf(all[t], all, depth + 1, prefix + name + '.', new Set([...seen, t])));
    }
    return keys;
  };
  for (const [svc, v] of Object.entries(services)) {
    const all = structs(v.pkg);
    if (all.Options) for (const k of keysOf(all.Options, all, 0, '', new Set(['Options']))) v.keys.add(k);
  }
  // Adapter-level options live in adapters/<svc>/<adapter>/*.go: union every json tag found there.
  for (const [svc, v] of Object.entries(services)) {
    const base = path.join(repo, 'adapters', v.pkg);
    if (!fs.existsSync(base)) continue;
    const scan = (d) => {
      for (const e of fs.readdirSync(d, { withFileTypes: true })) {
        const p = path.join(d, e.name);
        if (e.isDirectory()) { if (e.name !== 'internal' && e.name !== 'testdata') scan(p); continue; }
        if (!e.name.endsWith('.go') || e.name.endsWith('_test.go')) continue;
        for (const m of read(p).matchAll(/`[^`]*json:"([a-z0-9_]+)/g)) v.keys.add(m[1]);
      }
    };
    scan(base);
  }
  return services;
}
if (process.argv[1].endsWith('go-truth.mjs')) {
  const t = goTruth();
  console.log(JSON.stringify(Object.fromEntries(Object.entries(t).map(([k, v]) => [k, { pkg: v.pkg, keys: [...v.keys] }])), null, 1));
}
