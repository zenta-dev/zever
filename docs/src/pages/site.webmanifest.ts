import type { APIRoute } from 'astro';
import { SITE, NAME, TAGLINE } from '../lib/seo';

export const GET: APIRoute = () =>
  new Response(
    JSON.stringify({
      name: `${NAME}: ${TAGLINE}`,
      short_name: NAME,
      description: 'Schema-driven scaffolding compiler for Go.',
      start_url: `${new URL(SITE).pathname}/`,
      scope: `${new URL(SITE).pathname}/`,
      display: 'standalone',
      background_color: '#05070d',
      theme_color: '#05070d',
      icons: [
        { src: 'icon-192.png', sizes: '192x192', type: 'image/png', purpose: 'any' },
        { src: 'icon-512.png', sizes: '512x512', type: 'image/png', purpose: 'any maskable' },
      ],
    }),
    { headers: { 'Content-Type': 'application/manifest+json' } },
  );
