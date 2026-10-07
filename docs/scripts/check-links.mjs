// Verifies every internal link and asset reference in dist/ resolves to a built file.
// Usage: node scripts/check-links.mjs [distDir]  (exit 1 on broken links)
import fs from 'node:fs';
import path from 'node:path';

const dist = path.resolve(process.argv[2] ?? 'dist');
const BASE = '/zever/';
const walk = (d) => fs.readdirSync(d, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(path.join(d, e.name)) : [path.join(d, e.name)]));
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
for (const [u, from] of [...broken].slice(0, 60)) console.log(`BROKEN ${u}  <- ${[...from].slice(0, 3).join(', ')}${from.size > 3 ? ` (+${from.size - 3})` : ''}`);
console.log(`checked ${htmlFiles.length} pages, broken targets: ${broken.size}`);
process.exit(broken.size ? 1 : 0);
