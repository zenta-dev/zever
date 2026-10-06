import type { APIRoute, GetStaticPaths } from 'astro';
import { getCollection } from 'astro:content';
import { renderOg } from '../../lib/og';

export const getStaticPaths: GetStaticPaths = async () => {
  const docs = await getCollection('docs');
  return docs.map((entry) => ({ params: { slug: entry.id }, props: { entry } }));
};

export const GET: APIRoute = async ({ props }) => {
  const { entry } = props as { entry: { id: string; data: { title: string; description?: string } } };
  const group = entry.id.split('/')[0].replace(/-/g, ' ');
  const png = await renderOg({
    eyebrow: `Zever docs / ${group}`,
    title: entry.data.title,
    description: entry.data.description,
  });
  return new Response(png, { headers: { 'Content-Type': 'image/png' } });
};
