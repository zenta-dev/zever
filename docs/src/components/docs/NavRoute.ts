import { href, GITHUB } from '../../lib/url';

export type NavTab = 'guides' | 'reference';

/** tabFor maps a pathname to the top-level docs tab it belongs to. */
export function tabFor(pathname: string): NavTab {
  const ref = href('reference');
  return pathname === ref || pathname.startsWith(`${ref}/`) ? 'reference' : 'guides';
}

export const NAV_TABS = [
  { id: 'guides', label: 'Guides', href: href('getting-started/introduction') },
  { id: 'reference', label: 'Reference', href: href('reference/') },
  { id: 'examples', label: 'Examples', href: `${GITHUB}/tree/main/examples`, external: true },
] as const;
