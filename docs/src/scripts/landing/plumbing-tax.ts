import { scroll } from 'motion';
import { reducedMotion } from './motion';
import { wireCopy } from './walkthrough';

const PINNED = '(min-width: 900px) and (pointer: fine) and (prefers-reduced-motion: no-preference)';

/**
 * initPlumbingTax scroll-links the line counter, progress bar and dim/glow state (--p).
 * The final state (--p: 1) is the CSS default, so no JS / reduced motion shows it statically.
 */
export function initPlumbingTax(): void {
  const root = document.querySelector<HTMLElement>('[data-plumbing]');
  if (!root) return;
  wireCopy(root);
  const num = root.querySelector<HTMLElement>('[data-count]');
  const pct = root.querySelector<HTMLElement>('[data-pct]');
  if (!num || reducedMotion()) return;
  const from = Number(root.dataset.from);
  const to = Number(root.dataset.to);

  // Run the border animation only while the section is on screen.
  if ('IntersectionObserver' in window) {
    new IntersectionObserver((es) => es.forEach((e) => root.classList.toggle('is-live', e.isIntersecting))).observe(root);
  }

  const apply = (progress: number) => {
    const p = Math.min(1, Math.max(0, progress));
    root.style.setProperty('--p', p.toFixed(3));
    num.textContent = String(Math.round(from + (to - from) * p));
    if (pct) pct.textContent = String(Math.round(((from - to) / from) * 100 * p));
  };

  const pinned = window.matchMedia(PINNED);
  let stop: (() => void) | undefined;
  const bind = () => {
    stop?.();
    stop = pinned.matches
      ? // Pinned stage: progress runs over the first 70% of the tall track, then holds the final state.
        scroll((v: number) => apply(v / 0.7), { target: root, offset: ['start 15%', 'end end'] })
      : scroll(apply, { target: root, offset: ['start 85%', 'center 45%'] });
  };
  bind();
  pinned.addEventListener('change', bind);
}
