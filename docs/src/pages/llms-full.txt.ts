import type { APIRoute } from 'astro';
import { groupedDocs, toMarkdown } from '../scripts/docs/md';

export const GET: APIRoute = async () => {
  const groups = await groupedDocs();
  const parts = ['# Zever documentation\n'];
  for (const g of groups) for (const e of g.entries) parts.push(toMarkdown(e));
  return new Response(parts.join('\n---\n\n'), {
    headers: { 'Content-Type': 'text/plain; charset=utf-8' },
  });
};
