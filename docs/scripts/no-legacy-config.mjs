// Docs guard: the docs must never send a reader to a legacy config file.
// Usage: node docs/scripts/no-legacy-config.mjs [--self-test]
//
// Config is env-only since v0.8.0. The framework never reads a legacy
// config file, never probes for one and never warns about one: a leftover
// file is inert, so the values in it are dropped in silence. A page that
// tells a reader to put configuration into such a file sends them to a
// dead end, and that is the drift this check stops.
//
// Two rules:
//
// 1. Current docs (the published site under docs/src, the repo-level
//    contributor docs, the editor setup docs): every mention of a legacy
//    file name must sit in a section that says the file is never read, or
//    in a section about migrating away from it. A mention anywhere else is
//    an instruction to configure through the file, and it fails.
// 2. Historical docs (docs/designs, docs/plans, docs/superpowers,
//    docs/adrs): a dated record may describe the old mechanism, but it
//    must carry the banner that says it predates v0.8.0, so a reader
//    cannot mistake the record for current behaviour.
//
// The migration and upgrade guides are exempt from rule 1: naming the old
// format is their job.
//
// The wording of the rules is mechanical on purpose. A finding is never a
// judgement call, so this can gate a build.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// The script resolves its own paths, so it runs from the repo root
// (node docs/scripts/no-legacy-config.mjs) and from docs/ alike.
const here = path.dirname(fileURLToPath(import.meta.url));
const repo = path.resolve(here, '..', '..');
const docs = path.join(here, '..');

// The legacy config file names. The pattern matches the full name only, so
// a group like "`zever.yaml`/`.yml`/`.json`" is caught by its first name.
const LEGACY_FILE = /\bzever\.(?:ya?ml|json|toml)\b/i;
const legacyNames = (text) => [...text.matchAll(/\bzever\.(?:ya?ml|json|toml)\b/gi)].map((m) => m[0]);

// The wordings a section may use to say the file is not read. A section
// carrying one of them may name the file.
const NOT_READ = [
  'never read',
  'never looked for',
  'never parsed',
  'never opened',
  'not read',
  'no longer read',
  'nothing reads it',
  'does not look for',
  'is inert',
];

// A section about moving off the old format may name it.
const MIGRATION_HEADING = /\b(?:migrat|upgrad|v0\.7)\b/i;

// The body of a legacy config file: one block per service, each with an
// "adapter" key. A yaml/json/toml block shaped like that is an
// instruction to write a config file, even when it never names the file.
const CONFIG_BLOCK = /^[ \t]*["']?adapter["']?[ \t]*:/m;
const CONFIG_LANGS = /^(?:ya?ml|json|toml)$/;
const configBlocks = (text) =>
  [...text.matchAll(/```([^\n`]*)\n([\s\S]*?)```/g)]
    .filter((m) => CONFIG_LANGS.test(m[1].trim().split(/\s+/)[0] ?? ''))
    .filter((m) => CONFIG_BLOCK.test(m[2]))
    .map((m) => m[1].trim().split(/\s+/)[0]);

// The banner a historical record must carry.
const BANNER = 'predates v0.8.0';

// Rule 1 scope: the published site, the repo-level contributor docs, and
// the editor setup docs.
const CURRENT_DIRS = [path.join(docs, 'src'), path.join(docs, 'security')];
const CURRENT_FILES = [
  ...fs.readdirSync(docs).filter((f) => f.endsWith('.md')).map((f) => path.join(docs, f)),
  path.join(repo, 'editors', 'nvim', 'README.md'),
];

// Rule 2 scope: the dated records. They may describe the old mechanism,
// but they have to say they predate v0.8.0.
const HISTORICAL_DIRS = [
  path.join(docs, 'designs'),
  path.join(docs, 'plans'),
  path.join(docs, 'superpowers'),
  path.join(docs, 'adrs'),
];

// Pages that name the old format on purpose, relative to the content root.
const CONTENT = path.join(docs, 'src', 'content', 'docs');
const EXEMPT = ['guides/operate/migration.mdx', 'guides/operate/upgrade.mdx'];
const isExempt = (file) => EXEMPT.includes(path.relative(CONTENT, file));

const SKIP_DIRS = new Set(['node_modules', 'dist', '.git']);

// sections splits a page into { heading, text } blocks. A heading opens a
// section, and the text before the first heading is the lead section. Code
// fences are tracked so a "#" inside a shell snippet is not read as a
// heading.
export function sections(raw) {
  const out = [];
  let cur = { heading: '', lines: [] };
  let fence = false;
  for (const line of raw.split('\n')) {
    if (/^\s*(?:```|~~~)/.test(line)) fence = !fence;
    const m = fence ? null : line.match(/^(#{1,6})\s+(.*?)\s*$/);
    if (m) {
      out.push(cur);
      cur = { heading: m[2], lines: [] };
      continue;
    }
    cur.lines.push(line);
  }
  out.push(cur);
  return out.filter((s) => s.heading || s.lines.join('\n').trim());
}

// legacyConfigProblems returns the findings for one page. `historical`
// selects rule 2, `exempt` waives rule 1 for a page whose job is to name
// the old format. It is exported so the self-test can plant the mistakes
// this checker exists to catch and assert that it rejects them.
export function legacyConfigProblems(raw, opts = {}) {
  const { historical = false, exempt = false } = opts;
  const problems = [];

  if (historical) {
    if (LEGACY_FILE.test(raw) && !raw.includes(BANNER))
      problems.push(`names a legacy config file without the "${BANNER}" banner that marks it as a record of the past`);

    return problems;
  }

  if (exempt) return problems;

  for (const sec of sections(raw)) {
    const text = sec.lines.join('\n');
    const names = legacyNames(text);
    const blocks = configBlocks(text);
    if (!names.length && !blocks.length) continue;
    if (MIGRATION_HEADING.test(sec.heading)) continue;
    if (NOT_READ.some((p) => text.toLowerCase().includes(p))) continue;
    const where = `section "${sec.heading || '(lead)'}"`;
    if (names.length)
      problems.push(`${where} names ${[...new Set(names)].join(', ')} without saying the file is never read`);
    if (blocks.length)
      problems.push(`${where} shows a ${[...new Set(blocks)].join(', ')} block shaped like a config file, without saying the file is never read`);
  }

  return problems;
}

// selfTest plants the mistakes this checker exists to catch and asserts it
// rejects them, and that the correct wordings pass. It proves the guard
// bites instead of trusting that the rules still match.
function selfTest() {
  const page = (body) => `---\ntitle: Example\ndescription: A short description.\n---\n\n${body}\n`;
  const cases = [
    {
      name: 'a page that never mentions the file passes',
      run: () => legacyConfigProblems(page('Run `zever serve` to start the server.')),
      expect: 0,
    },
    {
      name: 'a page that says the file is never read passes',
      run: () => legacyConfigProblems(page('There are no config files: `zever.yaml` is never read.')),
      expect: 0,
    },
    {
      name: 'a page that says the file is never looked for passes',
      run: () => legacyConfigProblems(page('`zever.yml` is never looked for, so a stale one contributes nothing.')),
      expect: 0,
    },
    {
      name: 'a page that tells the reader to configure through the file is rejected',
      run: () => legacyConfigProblems(page('Put your database DSN in `zever.yaml` before you start.')),
      expect: 1,
    },
    {
      name: 'a yaml block that shows the file content is rejected',
      run: () => legacyConfigProblems(page('Create the file:\n\n```yaml\ndb:\n  adapter: postgres\n```')),
      expect: 1,
    },
    {
      name: 'a yaml block that is not a config file passes',
      run: () => legacyConfigProblems(page('Apply it:\n\n```yaml\napiVersion: v1\nkind: ConfigMap\n```')),
      expect: 0,
    },
    {
      name: 'a migration section may name the old format',
      run: () => legacyConfigProblems(page('## Migration from v0.7\n\nMove every `zever.yaml` stanza to `.env`.')),
      expect: 0,
    },
    {
      name: 'the migration guide may name the old format',
      run: () => legacyConfigProblems(page('Move every `zever.yaml` stanza to `.env`.'), { exempt: true }),
      expect: 0,
    },
    {
      name: 'a historical record without the banner is rejected',
      run: () => legacyConfigProblems(page('Existing `zever.yaml` files keep working unchanged.'), { historical: true }),
      expect: 1,
    },
    {
      name: 'a historical record with the banner passes',
      run: () =>
        legacyConfigProblems(
          page(`> **Historical (${BANNER}).** A legacy config file is never read.\n\nExisting \`zever.yaml\` files keep working unchanged.`),
          { historical: true },
        ),
      expect: 0,
    },
  ];

  let failed = 0;
  for (const c of cases) {
    const got = c.run().length;
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

const walk = (d) =>
  fs.readdirSync(d, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? (SKIP_DIRS.has(e.name) ? [] : walk(path.join(d, e.name))) : [path.join(d, e.name)],
  );
const md = (f) => /\.(?:md|mdx)$/.test(f);
const read = (f) => fs.readFileSync(f, 'utf8');

const problems = [];
let current = 0, historical = 0;

for (const dir of CURRENT_DIRS) {
  if (!fs.existsSync(dir)) continue;
  for (const file of walk(dir).filter(md)) {
    current++;
    const found = legacyConfigProblems(read(file), { exempt: isExempt(file) });
    for (const p of found) problems.push(`${path.relative(repo, file)}: ${p}`);
  }
}
for (const file of CURRENT_FILES) {
  if (!fs.existsSync(file)) continue;
  current++;
  const found = legacyConfigProblems(read(file), { exempt: isExempt(file) });
  for (const p of found) problems.push(`${path.relative(repo, file)}: ${p}`);
}
for (const dir of HISTORICAL_DIRS) {
  if (!fs.existsSync(dir)) continue;
  for (const file of walk(dir).filter(md)) {
    historical++;
    const found = legacyConfigProblems(read(file), { historical: true });
    for (const p of found) problems.push(`${path.relative(repo, file)}: ${p}`);
  }
}

for (const p of problems) console.log(`ERROR ${p}`);
console.log(`checked ${current} current pages and ${historical} historical records, ${problems.length} problems`);
process.exit(problems.length ? 1 : 0);
