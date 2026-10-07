/**
 * Preserves scroll state across navigation and reloads:
 * - docs sidebar scroll position and which groups are open (survives ClientRouter swaps),
 * - content scroll per URL (restores on reload and back/forward; a fresh link click starts at top).
 * Everything is best-effort: storage can be blocked, so every access is guarded.
 */
const SIDEBAR = 'zv:sidebar-scroll';
const GROUPS = 'zv:sidebar-groups';
const PAGES = 'zv:page-scroll';

const read = <T>(key: string, fallback: T): T => {
  try {
    const raw = sessionStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : fallback;
  } catch {
    return fallback;
  }
};
const write = (key: string, value: unknown) => {
  try {
    sessionStorage.setItem(key, JSON.stringify(value));
  } catch {
    /* storage blocked */
  }
};

const sidebar = () => document.getElementById('starlight__sidebar');
const groupKey = (d: HTMLDetailsElement) => (d.querySelector('summary')?.textContent ?? '').replace(/\s+/g, ' ').trim();
const pathKey = () => location.pathname;

function restoreGroups() {
  const saved = read<Record<string, boolean>>(GROUPS, {});
  sidebar()?.querySelectorAll<HTMLDetailsElement>('details').forEach((d) => {
    const k = groupKey(d);
    // The group holding the current page always stays open.
    if (d.querySelector('a[aria-current="page"]')) d.open = true;
    else if (k in saved) d.open = saved[k];
  });
}

function restoreSidebar() {
  const el = sidebar();
  if (!el) return;
  const y = read<number>(SIDEBAR, 0);
  if (y > 0) el.scrollTop = y;
  // Guarantee the current page is visible (deep link, search result, stale position).
  const cur = el.querySelector<HTMLElement>('a[aria-current="page"]');
  if (cur) {
    const r = cur.getBoundingClientRect();
    const b = el.getBoundingClientRect();
    if (r.top < b.top + 8 || r.bottom > b.bottom - 8) cur.scrollIntoView({ block: 'center', behavior: 'instant' as ScrollBehavior });
  }
}

function restoreContent(kind: 'load' | 'swap') {
  if (location.hash) return;
  const y = read<Record<string, number>>(PAGES, {})[pathKey()] ?? 0;
  if (y <= 0 || window.scrollY > 0) return;
  const nav = performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined;
  // Full loads: only reload/back_forward restore. Soft swaps: only history traversal.
  const ok = kind === 'load' ? nav?.type === 'reload' || nav?.type === 'back_forward' : lastNav === 'traverse';
  if (ok) window.scrollTo({ top: y, behavior: 'instant' as ScrollBehavior });
}

let lastNav: string = 'push';
let tick = 0;
const saveSoon = () => {
  if (tick) return;
  tick = window.setTimeout(() => {
    tick = 0;
    const el = sidebar();
    if (el) write(SIDEBAR, el.scrollTop);
    const pages = read<Record<string, number>>(PAGES, {});
    pages[pathKey()] = window.scrollY;
    write(PAGES, pages);
  }, 120);
};

// scroll does not bubble: capture on document to see the window and the sidebar pane, across swaps.
document.addEventListener('scroll', saveSoon, { capture: true, passive: true });
document.addEventListener('toggle', (e) => {
  const d = e.target;
  if (!(d instanceof HTMLDetailsElement) || !sidebar()?.contains(d)) return;
  const saved = read<Record<string, boolean>>(GROUPS, {});
  saved[groupKey(d)] = d.open;
  write(GROUPS, saved);
}, { capture: true });
window.addEventListener('pagehide', () => {
  clearTimeout(tick);
  tick = 0;
  saveSoon();
});

document.addEventListener('astro:before-preparation', ((e: Event) => {
  lastNav = (e as unknown as { navigationType?: string }).navigationType ?? 'push';
  // Persist the departing page's position before the swap.
  const el = sidebar();
  if (el) write(SIDEBAR, el.scrollTop);
  const pages = read<Record<string, number>>(PAGES, {});
  pages[pathKey()] = window.scrollY;
  write(PAGES, pages);
}) as EventListener);

document.addEventListener('astro:after-swap', () => {
  restoreGroups();
  restoreSidebar();
});
document.addEventListener('astro:page-load', () => restoreContent('swap'));

// First load of this document.
restoreGroups();
restoreSidebar();
restoreContent('load');
