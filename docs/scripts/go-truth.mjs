// Builds ground truth for config services from the Go source (config.go + core/<svc> Options structs).
// Usage: node scripts/go-truth.mjs > truth.json   (imported by audit-facts.mjs)
import fs from 'node:fs';
import path from 'node:path';

const repo = path.resolve('..');
const read = (p) => fs.readFileSync(p, 'utf8');

// camelToSnake mirrors config.camelToSnake (config/env.go): the runtime
// resolves a field name from the json tag when present, else from the Go
// field name in snake_case (config.jsonFieldName).
function camelToSnake(s) {
  let out = '';
  for (let i = 0; i < s.length; i++) {
    const c = s[i];
    if (c >= 'A' && c <= 'Z') {
      if (i > 0) {
        const prev = s[i - 1];
        const next = i + 1 < s.length ? s[i + 1] : '';
        if ((prev >= 'a' && prev <= 'z') || (next >= 'a' && next <= 'z')) out += '_';
      }
      out += c.toLowerCase();
    } else {
      out += c;
    }
  }
  return out;
}

export function goTruth() {
  const cfg = read(path.join(repo, 'config/config.go'));
  const services = {};
  // The Config struct lists one `Service[<pkg>.Options]` field per service.
  // The field name is the service key (lowercased); the fields carry no
  // struct tags, so there is no json name to read. The Plugins field is a
  // map, so it does not match this shape.
  for (const m of cfg.matchAll(/^\t(\w+)\s+Service\[(\w+)\.Options\]/gm)) services[m[1].toLowerCase()] = { pkg: m[2], keys: new Set() };
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
      const m = l.match(/^\s*(\w+)\s+([\w.*\[\]]+)\s*(?:`([^`]*)`)?/);
      if (!m) continue;
      const [, name, type, tag] = m;
      // A field is settable unless it is tagged `json:"-"` (excluded from
      // the option map the runtime redacts and resolves).
      if (tag && /json:"-"/.test(tag)) continue;
      const tagged = tag && /json:"([a-z0-9_]+)/.exec(tag);
      const jsonName = tagged ? tagged[1] : camelToSnake(name);
      if (!jsonName) continue;
      keys.push(prefix + jsonName);
      const t = type.replace(/^[*[\]]+/, '');
      if (depth < 2 && all[t] && !seen.has(t)) keys.push(...keysOf(all[t], all, depth + 1, prefix + jsonName + '.', new Set([...seen, t])));
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
