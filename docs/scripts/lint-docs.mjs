// Docs style lint. Usage: node scripts/lint-docs.mjs [--words]
// Errors (exit 1): em/en dashes in prose, lowercase "zever" in prose, missing/long description, > 2 Asides.
// Warnings: headings that look title-cased.
import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve('src/content/docs');
const walk = (d) => fs.readdirSync(d, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(path.join(d, e.name)) : [path.join(d, e.name)]));
const files = walk(root).filter((f) => /\.mdx?$/.test(f));
let errors = 0, warns = 0, words = 0;
const per = [];

for (const file of files) {
  const rel = path.relative(root, file);
  const raw = fs.readFileSync(file, 'utf8');
  const fm = raw.match(/^---\n([\s\S]*?)\n---/);
  const body = fm ? raw.slice(fm[0].length) : raw;
  const prose = body.replace(/```[\s\S]*?```/g, '').replace(/`[^`\n]*`/g, '').replace(/\]\([^)]*\)/g, ']').replace(/<[^>]+>/g, ' ');
  const err = (m) => { errors++; console.log(`ERROR ${rel}: ${m}`); };
  const warn = (m) => { warns++; console.log(`warn  ${rel}: ${m}`); };
  const front = fm ? fm[1] : '';
  const desc = front.match(/^description:\s*(.*)$/m)?.[1]?.replace(/^['"]|['"]$/g, '');
  if (!desc) err('missing description');
  else if (desc.length > 160) err(`description is ${desc.length} chars (max 160)`);
  if (/[—–]/.test(prose + front)) err('em/en dash in prose');
  const lower = prose.match(/(^|[\s(*_"'])zever(?![\w./@:-])/gm);
  if (lower) err(`lowercase "zever" in prose (${lower.length}x)`);
  const asides = (body.match(/<Aside\b/g) ?? []).length;
  if (asides > 2) err(`${asides} Asides (max 2)`);
  for (const h of body.replace(/```[\s\S]*?```/g, '').matchAll(/^#{2,3} (.+)$/gm)) {
    const t = h[1].replace(/`[^`]*`/g, '');
    const caps = t.split(/\s+/).slice(1).filter((w) => /^[A-Z][a-z]+$/.test(w) && !['Zever', 'Go', 'Redis', 'Postgres', 'SQL', 'JSON', 'YAML', 'Docker', 'Kubernetes', 'Claude', 'MCP', 'OpenAPI', 'Stripe', 'Paddle', 'Twilio', 'Vault', 'GitHub'].includes(w));
    if (caps.length >= 2) warn(`heading may not be sentence case: "${h[1]}"`);
  }
  const w = prose.split(/\s+/).filter(Boolean).length;
  words += w; per.push([w, rel]);
}
if (process.argv.includes('--words')) per.sort((a, b) => b[0] - a[0]).forEach(([w, r]) => console.log(String(w).padStart(6), r));
console.log(`${files.length} pages, ${words} prose words, ${errors} errors, ${warns} warnings`);
process.exit(errors ? 1 : 0);
