// Docs style lint. Usage: node docs/scripts/lint-docs.mjs [--words] [--self-test]
// Errors (exit 1): em/en dashes in prose, lowercase "zever" in prose, missing/long description, > 2 Asides.
// Warnings: headings that look title-cased.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// The script resolves its own paths, so it runs from the repo root
// (node docs/scripts/lint-docs.mjs) and from docs/ alike.
const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.join(here, '..', 'src', 'content', 'docs');
const walk = (d) => fs.readdirSync(d, { withFileTypes: true }).flatMap((e) => (e.isDirectory() ? walk(path.join(d, e.name)) : [path.join(d, e.name)]));
const files = walk(root).filter((f) => /\.mdx?$/.test(f));

// Proper nouns that keep their capitals in a sentence-case heading, so the
// title-case heuristic does not fire on them.
const KNOWN_CAPS = ['Zever', 'Go', 'Redis', 'Postgres', 'SQL', 'JSON', 'YAML', 'Docker', 'Kubernetes', 'Claude', 'MCP', 'OpenAPI', 'Stripe', 'Paddle', 'Twilio', 'Vault', 'GitHub'];

// lintPage returns the style findings for one page's raw text. It is
// exported so the self-test can plant the mistakes this checker exists to
// catch and assert that it rejects them.
export function lintPage(raw) {
  const errors = [];
  const warns = [];
  const fm = raw.match(/^---\n([\s\S]*?)\n---/);
  const body = fm ? raw.slice(fm[0].length) : raw;
  // The prose scope excludes code, so the rules about the brand name and
  // dashes only see running text. Fenced blocks and backticked spans are
  // code; so are the bodies of <code>, <kbd>, <samp> and <pre> elements,
  // which a page may use where markdown is not allowed (inside a
  // <summary>, for example).
  const prose = body
    .replace(/```[\s\S]*?```/g, '')
    .replace(/`[^`\n]*`/g, '')
    .replace(/<(code|kbd|samp|pre)\b[^>]*>[\s\S]*?<\/\1>/gi, ' ')
    .replace(/\]\([^)]*\)/g, ']')
    .replace(/<[^>]+>/g, ' ');
  const front = fm ? fm[1] : '';
  const desc = front.match(/^description:\s*(.*)$/m)?.[1]?.replace(/^['"]|['"]$/g, '');
  if (!desc) errors.push('missing description');
  else if (desc.length > 160) errors.push(`description is ${desc.length} chars (max 160)`);
  if (/[—–]/.test(prose + front)) errors.push('em/en dash in prose');
  const lower = prose.match(/(^|[\s(*_"'])zever(?![\w./@:-])/gm);
  if (lower) errors.push(`lowercase "zever" in prose (${lower.length}x)`);
  const asides = (body.match(/<Aside\b/g) ?? []).length;
  if (asides > 2) errors.push(`${asides} Asides (max 2)`);
  for (const h of body.replace(/```[\s\S]*?```/g, '').matchAll(/^#{2,3} (.+)$/gm)) {
    const t = h[1].replace(/`[^`]*`/g, '');
    const caps = t.split(/\s+/).slice(1).filter((w) => /^[A-Z][a-z]+$/.test(w) && !KNOWN_CAPS.includes(w));
    if (caps.length >= 2) warns.push(`heading may not be sentence case: "${h[1]}"`);
  }
  const words = prose.split(/\s+/).filter(Boolean).length;
  return { errors, warns, words };
}

// selfTest plants the style mistakes this checker exists to catch and
// asserts it rejects them, and that the correct spellings pass. It proves
// the guard bites instead of trusting that the rules still match.
function selfTest() {
  const page = (body, front = 'title: Example\ndescription: A short description.') => `---\n${front}\n---\n\n${body}\n`;
  const cases = [
    {
      name: 'a clean page passes',
      raw: page('Run `zever serve` to start the server.'),
      expect: 0,
    },
    {
      name: 'an em dash in prose is rejected',
      raw: page('The server is fast — it starts in milliseconds.'),
      expect: 1,
    },
    {
      name: 'an en dash in prose is rejected',
      raw: page('The range is 45–55 allocs.'),
      expect: 1,
    },
    {
      name: 'a lowercase brand name in prose is rejected',
      raw: page('Install zever before you start.'),
      expect: 1,
    },
    {
      name: 'a lowercase brand name in a command is allowed',
      raw: page('Run `zever serve` to start the server.'),
      expect: 0,
    },
    {
      name: 'a lowercase brand name in a <code> element is allowed',
      raw: page('<summary>Does <code>zever doctor</code> work?</summary>'),
      expect: 0,
    },
    {
      name: 'three Asides are rejected',
      raw: page('<Aside>a</Aside>\n<Aside>b</Aside>\n<Aside>c</Aside>'),
      expect: 1,
    },
    {
      name: 'a missing description is rejected',
      raw: '---\ntitle: Example\n---\n\nBody.\n',
      expect: 1,
    },
    {
      name: 'a long description is rejected',
      raw: page('Body.', `title: Example\ndescription: ${'x'.repeat(161)}`),
      expect: 1,
    },
  ];

  let failed = 0;
  for (const c of cases) {
    const got = lintPage(c.raw).errors.length;
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

let errors = 0, warns = 0, words = 0;
const per = [];

for (const file of files) {
  const rel = path.relative(root, file);
  const raw = fs.readFileSync(file, 'utf8');
  const found = lintPage(raw);
  for (const m of found.errors) { errors++; console.log(`ERROR ${rel}: ${m}`); }
  for (const m of found.warns) { warns++; console.log(`warn  ${rel}: ${m}`); }
  words += found.words; per.push([found.words, rel]);
}
if (process.argv.includes('--words')) per.sort((a, b) => b[0] - a[0]).forEach(([w, r]) => console.log(String(w).padStart(6), r));
console.log(`${files.length} pages, ${words} prose words, ${errors} errors, ${warns} warnings`);
process.exit(errors ? 1 : 0);
