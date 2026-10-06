import { href, GITHUB } from '../../lib/url';

export type NavTab = 'start' | 'guides' | 'concepts' | 'reference';

const SEGMENT_TAB: Record<string, NavTab> = {
  start: 'start',
  tutorials: 'start',
  contribute: 'start',
  guides: 'guides',
  concepts: 'concepts',
  reference: 'reference',
};

/** tabFor maps a pathname to the top-level docs tab it belongs to. */
export function tabFor(pathname: string): NavTab {
  const rest = pathname.replace(href('') , '').replace(/^\//, '');
  return SEGMENT_TAB[rest.split('/')[0]] ?? 'start';
}

/** Sidebar group (or top-level link) label to the tab that owns it. */
export const GROUP_TAB: Record<string, NavTab> = {
  'Guides overview': 'guides',
  Start: 'start',
  Tutorials: 'start',
  Contribute: 'start',
  'HTTP APIs': 'guides',
  'Data and storage': 'guides',
  'Background work': 'guides',
  Security: 'guides',
  Operate: 'guides',
  Integrations: 'guides',
  Extend: 'guides',
  Concepts: 'concepts',
  Reference: 'reference',
};

export const NAV_TABS = [
  { id: 'start', label: 'Start', href: href('start/introduction') },
  { id: 'guides', label: 'Guides', href: href('guides/') },
  { id: 'concepts', label: 'Concepts', href: href('concepts/how-zever-works') },
  { id: 'reference', label: 'Reference', href: href('reference/') },
  { id: 'examples', label: 'Examples', href: `${GITHUB}/tree/main/examples`, external: true },
] as const;
