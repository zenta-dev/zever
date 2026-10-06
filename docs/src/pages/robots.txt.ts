import type { APIRoute } from 'astro';
import { SITE } from '../lib/seo';

// Everything is public, open-source documentation: allow search engines AND AI crawlers.
const bots = ['GPTBot', 'OAI-SearchBot', 'ChatGPT-User', 'ClaudeBot', 'Claude-User', 'Claude-SearchBot', 'anthropic-ai', 'PerplexityBot', 'Perplexity-User', 'Google-Extended', 'Applebot-Extended', 'Bingbot', 'DuckDuckBot', 'CCBot', 'Meta-ExternalAgent', 'cohere-ai', 'YouBot', 'Amazonbot'];

export const GET: APIRoute = () => {
  const body = [
    '# Zever docs: open source (Apache-2.0). Search and AI crawlers are welcome.',
    '# Machine-readable index: /zever/llms.txt  Full text: /zever/llms-full.txt  Per page: append .md',
    'Content-Signal: search=yes, ai-input=yes, ai-train=yes',
    '',
    'User-agent: *',
    'Allow: /',
    '',
    ...bots.flatMap((b) => [`User-agent: ${b}`, 'Allow: /', '']),
    `Sitemap: ${SITE}/sitemap-index.xml`,
    '',
  ].join('\n');
  return new Response(body, { headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
};
