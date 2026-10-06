import { onVisible, reducedMotion } from './motion';

/** initCountUp animates [data-countup] elements from 0 to data-target when visible. The final value is already in the DOM text. */
export function initCountUp(root: ParentNode = document): void {
  root.querySelectorAll<HTMLElement>('[data-countup]').forEach((el) => {
    const target = parseFloat(el.dataset.target ?? '');
    if (Number.isNaN(target) || reducedMotion()) return;
    const decimals = parseInt(el.dataset.decimals ?? '0', 10);
    const suffix = el.dataset.suffix ?? '';
    const fmt = (v: number) => v.toFixed(decimals) + suffix;
    el.textContent = fmt(0);
    onVisible(el, () => {
      const dur = 1400;
      const t0 = performance.now();
      const tick = (now: number) => {
        const p = Math.min(1, (now - t0) / dur);
        const eased = 1 - Math.pow(1 - p, 4);
        el.textContent = fmt(target * eased);
        if (p < 1) requestAnimationFrame(tick);
        else el.textContent = fmt(target);
      };
      requestAnimationFrame(tick);
    }, 0.6);
  });
}

initCountUp();
