import type { APIRoute } from 'astro';
import { renderOg } from '../lib/og';

export const GET: APIRoute = async () => {
  const png = await renderOg({
    eyebrow: 'Go backend compiler',
    title: 'One schema. Every backend concern. Zero rewiring.',
    accent: 'Zero rewiring.',
    description: 'Describe services in .zen. Zever compiles typed ORM, OpenAPI and migrations, and wires every backend through one container.',
  });
  return new Response(png, { headers: { 'Content-Type': 'image/png' } });
};
