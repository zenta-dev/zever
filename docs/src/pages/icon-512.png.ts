import type { APIRoute } from 'astro';
import { renderIcon } from '../lib/og';

export const GET: APIRoute = () => new Response(renderIcon(512), { headers: { 'Content-Type': 'image/png' } });
