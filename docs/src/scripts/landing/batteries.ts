import { animate } from 'motion';
import { reducedMotion } from './motion';
import { edgeFade } from './scroll-fade';

const root = document.querySelector<HTMLElement>('[data-batteries]');
if (root) {
  const pills = Array.from(root.querySelectorAll<HTMLButtonElement>('[data-filters] [data-cat]'));
  const tiles = Array.from(root.querySelectorAll<HTMLElement>('.zl-bat-tile'));
  const status = root.querySelector<HTMLElement>('[data-status]')!;
  let token = 0;
  const fine = window.matchMedia('(pointer: fine)').matches;
  const filters = root.querySelector<HTMLElement>('[data-filters]');
  if (filters) edgeFade(filters);

  // Cursor-follow spotlight.
  if (fine) tiles.forEach((t) => {
    t.addEventListener('pointermove', (e) => {
      const r = t.getBoundingClientRect();
      t.style.setProperty('--mx', `${e.clientX - r.left}px`);
      t.style.setProperty('--my', `${e.clientY - r.top}px`);
    });
  });

  async function filter(cat: string) {
    const my = ++token;
    pills.forEach((p) => p.setAttribute('aria-pressed', String(p.dataset.cat === cat)));
    const show = tiles.filter((t) => cat === 'all' || t.dataset.cat === cat);
    const hide = tiles.filter((t) => !show.includes(t));
    status.textContent = `Showing ${show.length} ${cat === 'all' ? 'batteries' : cat + ' batteries'}.`;
    const motion = !reducedMotion();

    if (motion) {
      const out = hide.filter((t) => !t.hidden);
      if (out.length) {
        await Promise.all(out.map((t) => animate(t, { opacity: 0, scale: 0.94 }, { duration: 0.18 }).finished.catch(() => {})));
        if (my !== token) return;
      }
    }
    hide.forEach((t) => { t.hidden = true; });
    show.forEach((t, i) => {
      // Reveal observers may not have fired yet; force visible.
      t.classList.add('is-in');
      const wasHidden = t.hidden;
      t.hidden = false;
      if (motion) animate(t, { opacity: [wasHidden ? 0 : 1, 1], scale: [0.94, 1] }, { duration: 0.35, delay: Math.min(i, 12) * 0.03 });
      else { t.style.opacity = '1'; t.style.transform = 'none'; }
    });
  }

  pills.forEach((p) => p.addEventListener('click', () => filter(p.dataset.cat!)));
}
