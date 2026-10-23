// Builds ground truth for config services from the Go source (config.go + core/<svc> Options structs).
// Usage: node scripts/go-truth.mjs > truth.json   (imported by audit-facts.mjs)
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// The script resolves its own paths, so it runs from the repo root
// (node docs/scripts/go-truth.mjs) and from docs/ alike.
const here = path.dirname(fileURLToPath(import.meta.url));
const repo = path.resolve(here, '..', '..');
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

// Adapter const spellings in adapters/<svc>/<adapter>/register.go. All three
// declare the same `const Adapter <pkg>.Adapter`; the first two yield a name,
// the reference form points at a core const that the core scanner already
// captures, so it (and a sigil-free non-declaration) yields nothing.
const ADAPTER_CONST_FORMS = [
  [/=\s*[\w.]*Adapter\s*\(\s*"([^"]+)"\s*\)$/, (m) => m[1]], // = x.Adapter("name")
  [/=\s*"([^"]+)"$/, (m) => m[1]], // = "name"
  [/=\s*[\w.]+$/, () => null], // = x.DB (core const reference)
];

// adapterNamesFromRegister extracts the adapter names a register.go declares:
// a list from the string-literal and conversion forms above, possibly empty
// (an adapter may legitimately name itself in core instead; a register.go can
// also lack the const altogether). The const may sit on one line or in a
// `const (...)` header block; `//` comments never count. Fail closed: a line
// that starts a `const Adapter` declaration and matches no known form throws,
// so an unparsable register.go can never silently widen or shrink the set.
export function adapterNamesFromRegister(src) {
  const names = [];
  let inBlock = false;
  for (const raw of src.split('\n')) {
    const code = raw.replace(/\/\/.*$/, '').trim();
    if (!inBlock && /^const\s*\(\s*$/.test(code)) { inBlock = true; continue; }
    if (inBlock && /^\)\s*$/.test(code)) { inBlock = false; continue; }
    if (!(inBlock && code.startsWith('Adapter')) && !/^const\s+Adapter\b/.test(code)) continue;
    const form = ADAPTER_CONST_FORMS.find(([re]) => re.test(code));
    if (!form) throw new Error(`go-truth: unparsable adapter const in register.go: ${raw.trim()}`);
    const name = form[1](code.match(form[0]));
    if (name !== null && name !== undefined) names.push(name);
  }
  return names;
}

export function goTruth() {
  const cfg = read(path.join(repo, 'config/config.go'));
  const services = {};
  // The Config struct lists one `Service[<pkg>.Options]` field per service.
  // The json tag names the service key; the Go field name is the fallback.
  // The Plugins field is a map, so it does not match this shape.
  for (const m of cfg.matchAll(/^\t(\w+)\s+Service\[(\w+)\.Options\]\s*(?:`([^`]*)`)?/gm)) {
    const tagged = m[3] && /json:"([a-z0-9_]+)/.exec(m[3]);
    services[tagged ? tagged[1] : m[1].toLowerCase()] = { pkg: m[2], keys: new Set(), adapters: new Set() };
  }
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
  // Adapter names: the canonical consts in core/<svc>/adapter.go plus the
  // per-adapter consts in adapters/<svc>/<adapter>/register.go (an adapter
  // that ships its own name, such as the postgres scheduler, declares no
  // const in core). register.go may spell the const as a string literal or
  // as a conversion of the service's Adapter type; adapterNamesFromRegister
  // parses both and throws on a shape it does not know (fail closed).
  for (const [svc, v] of Object.entries(services)) {
    const core = path.join(repo, 'core', v.pkg, 'adapter.go');
    if (fs.existsSync(core))
      for (const m of read(core).matchAll(/^\s*\w+\s+Adapter\s*=\s*"([^"]+)"/gm)) v.adapters.add(m[1]);
    const base = path.join(repo, 'adapters', v.pkg);
    if (!fs.existsSync(base)) continue;
    for (const e of fs.readdirSync(base, { withFileTypes: true })) {
      if (!e.isDirectory()) continue;
      const f = path.join(base, e.name, 'register.go');
      if (!fs.existsSync(f)) continue;
      for (const n of adapterNamesFromRegister(read(f))) v.adapters.add(n);
    }
  }
  return services;
}
if (process.argv[1] && process.argv[1].endsWith('go-truth.mjs')) {
  const t = goTruth();
  console.log(JSON.stringify(Object.fromEntries(Object.entries(t).map(([k, v]) => [k, { pkg: v.pkg, keys: [...v.keys], adapters: [...v.adapters] }])), null, 1));
}
