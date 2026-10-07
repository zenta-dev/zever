import { animate } from 'motion';
import { reducedMotion } from './motion';

const fine = () => window.matchMedia('(pointer: fine)').matches && !reducedMotion();

/** initHeroFx wires magnetic CTAs, pointer parallax and offscreen pausing for the gradient mesh. */
export function initHeroFx(): void {
  // Pause infinite mesh/scan animations while their section is offscreen.
  const meshes = document.querySelectorAll<HTMLElement>('[data-mesh]');
  if (meshes.length && 'IntersectionObserver' in window) {
    const io = new IntersectionObserver((es) => {
      es.forEach((e) => (e.target as HTMLElement).classList.toggle('is-paused', !e.isIntersecting));
    });
    meshes.forEach((m) => io.observe(m));
  }
  if (!fine()) return;

  const spring = { type: 'spring', stiffness: 220, damping: 16 } as const;

  // Magnetic: element drifts up to 8px toward the pointer while it is near.
  document.querySelectorAll<HTMLElement>('[data-magnetic]').forEach((el) => {
    const max = 8;
    const reach = 90;
    const move = (e: PointerEvent) => {
      const r = el.getBoundingClientRect();
      const dx = e.clientX - (r.left + r.width / 2);
      const dy = e.clientY - (r.top + r.height / 2);
      const d = Math.hypot(dx, dy);
      if (d > reach + r.width / 2) return rest();
      const k = Math.min(1, (reach + r.width / 2 - d) / reach);
      void animate(el, { x: Math.max(-max, Math.min(max, dx * 0.2 * k)), y: Math.max(-max, Math.min(max, dy * 0.3 * k)) }, spring);
    };
    const rest = () => void animate(el, { x: 0, y: 0 }, spring);
    window.addEventListener('pointermove', move, { passive: true });
    el.addEventListener('pointerleave', rest);
  });

  // Parallax: [data-parallax="px"] children of a root shift opposite the pointer inside that root.
  document.querySelectorAll<HTMLElement>('[data-parallax-root]').forEach((root) => {
    const items = [...root.querySelectorAll<HTMLElement>('[data-parallax]')];
    if (!items.length) return;
    let raf = 0;
    let px = 0;
    let py = 0;
    const apply = () => {
      raf = 0;
      items.forEach((it) => {
        const d = Number(it.dataset.parallax) || 10;
        void animate(it, { x: px * d, y: py * d }, { type: 'spring', stiffness: 90, damping: 18 });
      });
    };
    root.addEventListener('pointermove', (e) => {
      const r = root.getBoundingClientRect();
      px = ((e.clientX - r.left) / r.width - 0.5) * 2;
      py = ((e.clientY - r.top) / r.height - 0.5) * 2;
      if (!raf) raf = requestAnimationFrame(apply);
    }, { passive: true });
    root.addEventListener('pointerleave', () => { px = 0; py = 0; if (!raf) raf = requestAnimationFrame(apply); });
  });
}
