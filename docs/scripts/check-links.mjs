// Verifies every internal link and asset reference in dist/ resolves to a built file.
// Usage: node docs/scripts/check-links.mjs [distDir] [--self-test]  (exit 1 on broken links)
//
// The check is fully offline: it only looks at the built site, so it never
// touches the network. External (http, https, mailto) links are skipped;
// the lychee job in the docs workflow covers those.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// The script resolves its own paths, so it runs from the repo root
// (node docs/scripts/check-links.mjs) and from docs/ alike. An explicit
// distDir argument wins; a relative one is taken from the docs directory.
const here = path.dirname(fileURLToPath(import.meta.url));
const docs = path.resolve(here, '..');
const BASE = '/zever/';
const walk = (d) => fs.readdirSync(d, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(path.join(d, e.name)) : [path.join(d, e.name)]));

// findBroken returns the broken link targets in a built site, keyed by the
// target URL and carrying the pages that link to it. It is exported so the
// self-test can plant a broken link and assert that the check finds it.
export function findBroken(dist) {
  const htmlFiles = walk(dist).filter((f) => f.endsWith('.html'));

  const exists = (urlPath) => {
    const p = path.join(dist, urlPath.replace(/^\//, ''));
    if (fs.existsSync(p) && fs.statSync(p).isFile()) return true;
    return fs.existsSync(path.join(p, 'index.html')) || fs.existsSync(p + '.html');
  };

  const broken = new Map();
  const idCache = new Map();
  const idsOf = (urlPath) => {
    if (idCache.has(urlPath)) return idCache.get(urlPath);
    const p = path.join(dist, urlPath.replace(/^\//, ''));
    const f = fs.existsSync(path.join(p, 'index.html')) ? path.join(p, 'index.html') : p + '.html';
    const ids = fs.existsSync(f) ? new Set([...fs.readFileSync(f, 'utf8').matchAll(/\bid="([^"]+)"/g)].map((m) => m[1])) : null;
    idCache.set(urlPath, ids);
    return ids;
  };
  for (const file of htmlFiles) {
    const html = fs.readFileSync(file, 'utf8');
    const pageUrl = '/' + path.relative(dist, path.dirname(file)).split(path.sep).join('/') + '/';
    const base = BASE.replace(/\/$/, '') + (pageUrl === '//' ? '/' : pageUrl.replace(/^\/\.?\/?/, '/'));
    for (const m of html.matchAll(/\b(?:href|src)="([^"]+)"/g)) {
      let u = m[1].replace(/&amp;/g, '&');
      if (/^(https?:|mailto:|tel:|data:|javascript:|#)/.test(u) || u.startsWith('//')) continue;
      const frag = u.includes('#') ? u.split('#')[1] : '';
      u = u.split('#')[0].split('?')[0];
      if (!u) continue;
      let abs;
      if (u.startsWith('/')) abs = u;
      else abs = new URL(u, 'http://x' + (base.endsWith('/') ? base : base + '/')).pathname;
      if (!abs.startsWith(BASE)) { (broken.get(abs) ?? broken.set(abs, new Set()).get(abs)).add(path.relative(dist, file)); continue; }
      if (!exists(abs.slice(BASE.length - 1))) { (broken.get(abs) ?? broken.set(abs, new Set()).get(abs)).add(path.relative(dist, file)); continue; }
      if (frag) {
        const ids = idsOf(abs.slice(BASE.length - 1));
        if (ids && !ids.has(decodeURIComponent(frag))) { const k = `${abs}#${frag}`; (broken.get(k) ?? broken.set(k, new Set()).get(k)).add(path.relative(dist, file)); }
      }
    }
  }
  return { broken, pages: htmlFiles.length };
}

// selfTest builds a tiny site with one good link, one broken link, one
// broken anchor and one external link, and asserts the check reports
// exactly the two broken internal targets. It proves the gate bites and
// that the check stays offline.
function selfTest() {
  const tmp = fs.mkdtempSync(path.join(os.tmpdir(), 'zever-check-links-'));
  const write = (rel, html) => {
    const p = path.join(tmp, rel);
    fs.mkdirSync(path.dirname(p), { recursive: true });
    fs.writeFileSync(p, html);
  };
  write('index.html', '<html><body><a href="/zever/guides/queue/">queue</a><a href="/zever/guides/missing/">missing</a><a href="/zever/guides/queue/#nope">bad anchor</a><a href="https://example.com/x">external</a></body></html>');
  write('guides/queue/index.html', '<html><body><h2 id="top">Queue</h2></body></html>');

  let failed = 0;
  const { broken } = findBroken(tmp);
  const found = [...broken.keys()].sort();
  const want = ['/zever/guides/missing/', '/zever/guides/queue/#nope'];
  const ok = found.length === want.length && want.every((w) => found.includes(w));
  if (!ok) {
    failed++;
    console.error(`self-test FAIL: broken links: got [${found.join(', ')}], want [${want.join(', ')}]`);
  } else {
    console.log('self-test ok: a broken link and a broken anchor are reported');
  }
  if (found.some((f) => f.includes('example.com'))) {
    failed++;
    console.error('self-test FAIL: an external link was reported, the check must stay offline');
  } else {
    console.log('self-test ok: an external link is skipped (the check is offline)');
  }
  fs.rmSync(tmp, { recursive: true, force: true });
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

const arg = process.argv.slice(2).find((a) => !a.startsWith('--'));
const dist = arg ? (path.isAbsolute(arg) ? arg : path.resolve(docs, arg)) : path.join(docs, 'dist');
if (!fs.existsSync(dist)) {
  console.error(`check-links: no built site at ${dist}. Run "npm run build" first.`);

  process.exit(1);
}
const { broken, pages } = findBroken(dist);
for (const [u, from] of [...broken].slice(0, 60)) console.log(`BROKEN ${u}  <- ${[...from].slice(0, 3).join(', ')}${from.size > 3 ? ` (+${from.size - 3})` : ''}`);
console.log(`checked ${pages} pages, broken targets: ${broken.size}`);
process.exit(broken.size ? 1 : 0);
