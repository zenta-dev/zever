import { getCollection, type CollectionEntry } from 'astro:content';
import config from 'virtual:starlight/user-config';

type Entry = CollectionEntry<'docs'>;
type SidebarItem = { label?: string; link?: string; items?: SidebarItem[] };

const ORIGIN = 'https://zenta-dev.github.io';

/** Resolves a docs link against the page's HTML URL so it works wherever the Markdown is read. */
function absolutize(target: string, id: string): string {
  if (/^(#|[a-z][a-z0-9+.-]*:|\/\/)/i.test(target)) return target;
  const base = import.meta.env.BASE_URL.replace(/\/$/, '');
  try {
    return new URL(target, `${ORIGIN}${base}/${id}/`).href;
  } catch {
    return target;
  }
}

/** Strips MDX imports/exports and JSX tags outside fenced code; makes links absolute. */
export function cleanBody(body: string, id = ''): string {
  const out: string[] = [];
  let fence = '';
  for (const line of body.split('\n')) {
    const m = line.match(/^\s*(`{3,}|~{3,})/);
    if (m) {
      if (!fence) fence = m[1][0];
      else if (m[1][0] === fence) fence = '';
      out.push(line);
      continue;
    }
    if (fence) {
      out.push(line);
      continue;
    }
    if (/^\s*import\s.+from\s+['"].+['"];?\s*$/.test(line)) continue;
    // <LinkCard title="X" description="Y" href="Z" /> becomes a Markdown list item.
    const card = line.match(/^\s*<LinkCard\b(.*)\/>\s*$/);
    if (card) {
      const attr = (n: string) => card[1].match(new RegExp(`${n}="([^"]*)"`))?.[1];
      const t = attr('title');
      const h = attr('href');
      if (t && h) out.push(`- [${t}](${absolutize(h, id)})${attr('description') ? `: ${attr('description')}` : ''}`);
      continue;
    }
    if (/^\s*<\/?[A-Z][\w.]*(\s[^>]*)?\/?>\s*$/.test(line)) continue;
    out.push(
      line
        .replace(/<\/?[A-Z][\w.]*(\s[^<>]*)?\/?>/g, '')
        .replace(/\]\(([^)\s]+)\)/g, (_m, t: string) => `](${absolutize(t, id)})`),
    );
  }
  return out.join('\n').replace(/\n{3,}/g, '\n\n').trim();
}

/** Full Markdown document for one page. */
export function toMarkdown(entry: Entry): string {
  const { title, description } = entry.data;
  return `# ${title}\n\n${description ? description + '\n\n' : ''}${cleanBody(entry.body ?? '', entry.id)}\n`;
}

/** Absolute URL of a page's Markdown source. */
export function mdUrl(site: URL | undefined, id: string): string {
  const base = import.meta.env.BASE_URL.replace(/\/$/, '');
  return new URL(`${base}/${id}.md`, site ?? 'https://zenta-dev.github.io').href;
}

/** Docs grouped and ordered like the sidebar. */
export async function groupedDocs(): Promise<{ label: string; entries: Entry[] }[]> {
  const all = await getCollection('docs');
  const byId = new Map(all.map((e) => [e.id, e]));
  const seen = new Set<string>();
  const groups: { label: string; entries: Entry[] }[] = [];
  const walk = (items: SidebarItem[], acc: Entry[]) => {
    for (const it of items) {
      if (it.items) walk(it.items, acc);
      else if (it.link) {
        const id = it.link.replace(/^\/+|\/+$/g, '');
        const e = byId.get(id);
        if (e && !seen.has(id)) {
          seen.add(id);
          acc.push(e);
        }
      }
    }
  };
  const sidebar = ((config as { sidebar?: SidebarItem[] }).sidebar ?? []) as SidebarItem[];
  let loose: Entry[] = [];
  for (const g of sidebar) {
    if (g.items) {
      const acc: Entry[] = [];
      walk(g.items, acc);
      if (acc.length) groups.push({ label: g.label ?? 'Docs', entries: acc });
    } else walk([g], loose);
  }
  const rest = all.filter((e) => !seen.has(e.id)).sort((a, b) => a.id.localeCompare(b.id));
  loose = loose.concat(rest);
  if (loose.length) groups.push({ label: 'More', entries: loose });
  return groups;
}
