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
// 1. Current prose (the published site, the repo-level contributor docs,
//    the module READMEs, the editor and example READMEs, the shell
//    scripts, the Go source): every mention of a legacy file name must sit
//    in a section that says the file is never read, or in a section about
//    migrating away from it. A mention anywhere else is an instruction to
//    configure through the file, and it fails.
// 2. Historical docs (docs/designs, docs/plans, docs/superpowers,
//    docs/adrs): a dated record may describe the old mechanism, but it
//    must carry the banner that says it predates v0.8.0, so a reader
//    cannot mistake the record for current behaviour.
//
// The migration and upgrade guides are exempt from rule 1: naming the
// old format is their job. The release notes (CHANGELOG.md and the
// unreleased changelog fragments) are records of what a release did, so
// they are not scanned: the v0.8.0 note is the record of the removal.
// This guard's own documentation (the AST guard's README) is exempt for
// the same reason: it names the retired formats to describe the rule.
//
// The wording of the rules is mechanical on purpose. A finding is never a
// judgement call, so this can gate a build.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

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

const SKIP_DIRS = new Set(['node_modules', 'dist', '.git', '.worktrees', 'vendor']);

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

// --- the scanned set --------------------------------------------------

const walk = (d) =>
  fs.readdirSync(d, { withFileTypes: true }).flatMap((e) =>
    e.isDirectory() ? (SKIP_DIRS.has(e.name) ? [] : walk(path.join(d, e.name))) : [path.join(d, e.name)],
  );
const md = (f) => /\.(?:md|mdx)$/.test(f);
const read = (f) => fs.readFileSync(f, 'utf8');

// A generated Go file is not the repo's prose: it carries a "DO NOT EDIT"
// marker, so a contributor cannot fix it there.
const isGenerated = (f) => {
  const head = read(f).slice(0, 400);
  return /DO NOT EDIT|Code generated by/.test(head);
};

// The release notes record what a release did. The v0.8.0 note is the
// record of the removal, so the notes are not scanned for the rules that
// police current prose.
const isReleaseNote = (rel) => rel === 'CHANGELOG.md' || rel.startsWith('.changelog/');

// The AST guard's own documentation names the retired formats to describe
// the rule that forbids them.
const isGuardDoc = (rel) => rel.startsWith('tools/tagcheck/');

// scannedFiles returns the files the guard reads, bucketed by scope:
//
//   prose      -- markdown and shell: rule 1
//   go         -- Go source: rule 1
//   historical -- the dated records: rule 2
//
// The paths are repo-relative and sorted, so the self-test can assert that
// the newly covered files are in the set.
export function scannedFiles() {
  const prose = [];
  const go = [];
  const historical = [];

  for (const dir of CURRENT_DIRS) {
    if (!fs.existsSync(dir)) continue;
    for (const file of walk(dir).filter(md)) prose.push(path.relative(repo, file));
  }
  for (const file of CURRENT_FILES) {
    if (!fs.existsSync(file)) continue;
    prose.push(path.relative(repo, file));
  }
  for (const dir of HISTORICAL_DIRS) {
    if (!fs.existsSync(dir)) continue;
    for (const file of walk(dir).filter(md)) historical.push(path.relative(repo, file));
  }

  for (const file of walk(repo)) {
    const rel = path.relative(repo, file);
    if (isReleaseNote(rel) || isGuardDoc(rel)) continue;
    if (md(file)) {
      // The dated records are rule 2's scope, not rule 1's.
      if (historical.includes(rel)) continue;
      prose.push(rel);
      continue;
    }
    if (file.endsWith('.sh')) {
      prose.push(rel);
      continue;
    }
    if (file.endsWith('.go') && !file.endsWith('_test.go') && !isGenerated(file)) go.push(rel);
  }

  return {
    prose: [...new Set(prose)].sort(),
    go: [...new Set(go)].sort(),
    historical: [...new Set(historical)].sort(),
  };
}

// --- the self-test ----------------------------------------------------

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
    {
      name: 'a module README that names the retired file is rejected',
      run: () => legacyConfigProblems('# db\n\nPut the DSN in `zever.yaml`.\n'),
      expect: 1,
    },
    {
      name: 'a shell script that names the retired file is rejected',
      run: () => legacyConfigProblems('#!/bin/sh\n# Read the DSN from zever.yaml\ncat zever.yaml\n'),
      expect: 1,
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

  // The scope cases: the files that used to be unscanned must be in the
  // scanned set, or a mention in them would pass CI again.
  const scanned = scannedFiles();
  const prose = new Set(scanned.prose);
  const go = new Set(scanned.go);
  const scopeCases = [
    { name: 'the root README is scanned', ok: prose.has('README.md') },
    { name: 'the contributor guide is scanned', ok: prose.has('AGENTS.md') },
    { name: 'the stability note is scanned', ok: prose.has('STABILITY.md') },
    { name: 'the config package README is scanned', ok: prose.has(path.join('config', 'README.md')) },
    { name: 'the container package README is scanned', ok: prose.has(path.join('container', 'README.md')) },
    { name: 'the CLI package README is scanned', ok: prose.has(path.join('cmd', 'zever', 'README.md')) },
    { name: 'a battery README is scanned', ok: prose.has(path.join('core', 'db', 'README.md')) },
    { name: 'an editor README is scanned', ok: prose.has(path.join('editors', 'vscode', 'README.md')) },
    { name: 'an example README is scanned', ok: prose.has(path.join('examples', 'todo', 'README.md')) },
    { name: 'a shell script is scanned', ok: prose.has(path.join('tools', 'changelog-lint.sh')) },
    { name: 'a battery doc.go is scanned', ok: go.has(path.join('core', 'db', 'doc.go')) },
    { name: 'a Go source file is scanned', ok: go.has(path.join('config', 'dotenv.go')) },
  ];
  for (const c of scopeCases) {
    if (!c.ok) {
      failed++;
      console.error(`self-test FAIL: ${c.name}`);
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

const isEntryPoint =
  process.argv[1] !== undefined && import.meta.url === pathToFileURL(process.argv[1]).href;

if (isEntryPoint) {
  const scanned = scannedFiles();
  const problems = [];
  let current = 0, historical = 0;

  for (const rel of scanned.prose) {
    current++;
    const file = path.join(repo, rel);
    const raw = read(file);
    // The migration and upgrade guides name the old format on purpose.
    const exempt = isExempt(file);
    for (const p of legacyConfigProblems(raw, { exempt })) problems.push(`${rel}: ${p}`);
  }
  for (const rel of scanned.go) {
    current++;
    const file = path.join(repo, rel);
    const raw = read(file);
    for (const p of legacyConfigProblems(raw)) problems.push(`${rel}: ${p}`);
  }
  for (const rel of scanned.historical) {
    historical++;
    const file = path.join(repo, rel);
    for (const p of legacyConfigProblems(read(file), { historical: true })) problems.push(`${rel}: ${p}`);
  }

  for (const p of problems) console.log(`ERROR ${p}`);
  console.log(
    `checked ${current} current files and ${historical} historical records, ${problems.length} problems`,
  );
  process.exit(problems.length ? 1 : 0);
}
