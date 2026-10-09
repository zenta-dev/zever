import type { APIRoute } from 'astro';
import { groupedDocs, mdUrl } from '../scripts/docs/md';
import { abs } from '../lib/seo';
import { INSTALL, INSTALL_WIN } from '../data/landing';

export const GET: APIRoute = async ({ site }) => {
  const groups = await groupedDocs();
  const lines = [
    '# Zever',
    '',
    '> Zever is a schema-driven scaffolding compiler for Go. Describe services in a .zen schema and it compiles database access, caching, queues, routing and other backend plumbing, never your business logic.',
    '',
    'Key facts:',
    '',
    '- Language and version: Go 1.27+. License: Apache-2.0. Repository: https://github.com/zenta-dev/zever',
    `- Install: \`${INSTALL}\` (Linux, macOS); Windows PowerShell: \`${INSTALL_WIN}\``,
    '- Input: `.zen` schema files. Output: typed ORM, OpenAPI, migration DDL, routing glue. Business logic is never generated.',
    '- Every backend (db, cache, queue, auth, storage, mail, search and more) is a small interface with swappable adapters, resolved lazily through one container. Switching adapters is a config change.',
    '- Zero-infrastructure defaults (sqlite, memory, local, log) so a clean machine builds and tests with no external services.',
    '- Each page below is also available as raw Markdown (append `.md`); `llms-full.txt` contains every page in one file.',
    '',
  ];
  for (const g of groups) {
    lines.push(`## ${g.label}`, '');
    for (const e of g.entries) {
      const d = e.data.description ? `: ${e.data.description}` : '';
      lines.push(`- [${e.data.title}](${mdUrl(site, e.id)})${d}`);
    }
    lines.push('');
  }
  lines.push(
    '## Optional',
    '',
    `- [Full documentation in one file](${abs('llms-full.txt')}): every page concatenated`,
    '- [Source code](https://github.com/zenta-dev/zever): Go module, examples and changelog',
    '- [Changelog](https://github.com/zenta-dev/zever/blob/main/CHANGELOG.md): release notes',
    '',
  );
  return new Response(lines.join('\n'), {
    headers: { 'Content-Type': 'text/plain; charset=utf-8' },
  });
};
