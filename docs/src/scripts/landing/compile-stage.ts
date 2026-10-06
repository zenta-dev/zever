import { animate, stagger } from 'motion';
import { reducedMotion } from './motion';

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

/** initStageFx adds the cursor-follow glow and 3D card tilt (fine pointers, no reduced motion). */
function initStageFx(root: HTMLElement): void {
  if (!window.matchMedia('(pointer: fine)').matches) return;
  root.addEventListener('pointermove', (e) => {
    const r = root.getBoundingClientRect();
    root.style.setProperty('--gx', `${e.clientX - r.left}px`);
    root.style.setProperty('--gy', `${e.clientY - r.top}px`);
    root.classList.add('is-hot');
  }, { passive: true });
  root.addEventListener('pointerleave', () => root.classList.remove('is-hot'));
  const max = 6;
  root.querySelectorAll<HTMLElement>('.zl-cs-tilt').forEach((t) => {
    t.addEventListener('pointermove', (e) => {
      const r = t.getBoundingClientRect();
      const x = (e.clientX - r.left) / r.width - 0.5;
      const y = (e.clientY - r.top) / r.height - 0.5;
      t.style.setProperty('--ry', `${(x * 2 * max).toFixed(2)}deg`);
      t.style.setProperty('--rx', `${(-y * 2 * max).toFixed(2)}deg`);
      t.classList.add('is-tilting');
    }, { passive: true });
    t.addEventListener('pointerleave', () => {
      t.style.setProperty('--rx', '0deg');
      t.style.setProperty('--ry', '0deg');
      t.classList.remove('is-tilting');
    });
  });
}

/** initCompileStage plays schema typing -> compile pulse -> beams -> output cards -> success tick, looping while on screen. */
export function initCompileStage(root: HTMLElement): void {
  if (reducedMotion()) return; // final state is already in the DOM
  initStageFx(root);
  const lines = [...root.querySelectorAll<HTMLElement>('.zl-cs-code .line')];
  const cards = [...root.querySelectorAll<HTMLElement>('.zl-cs-card')];
  const beams = [...root.querySelectorAll<SVGPathElement>('.zl-cs-beam')];
  const dots = [...root.querySelectorAll<SVGPathElement>('.zl-cs-dot')];
  const chip = root.querySelector<HTMLElement>('.zl-cs-chip');
  const ring = root.querySelector<HTMLElement>('.zl-cs-ring');
  const ok = root.querySelector<HTMLElement>('.zl-cs-ok');
  const tick = ok?.querySelector<SVGPathElement>('path');
  if (!lines.length) return;

  let visible = false;
  let waiters: Array<() => void> = [];
  new IntersectionObserver(([e]) => {
    visible = e.isIntersecting;
    if (visible) { waiters.forEach((w) => w()); waiters = []; }
  }, { threshold: 0.25 }).observe(root);
  const whenVisible = () => (visible ? Promise.resolve() : new Promise<void>((r) => waiters.push(r)));

  const setFlow = (on: boolean) => {
    beams.forEach((b) => { b.style.animationPlayState = on ? 'running' : 'paused'; });
    dots.forEach((d) => { d.style.display = on ? '' : 'none'; });
  };

  const reset = () => {
    lines.forEach((l) => { l.style.clipPath = 'inset(0 100% 0 0)'; l.style.opacity = '0'; });
    cards.forEach((c) => { c.style.opacity = '0'; c.style.transform = 'translateY(14px) scale(0.94)'; });
    beams.forEach((b) => { b.style.opacity = '0'; });
    setFlow(false);
    if (ok) { ok.style.opacity = '0'; ok.style.transform = 'scale(0)'; }
    if (chip) chip.style.opacity = '0.45';
  };

  const run = async () => {
    root.classList.add('is-playing');
    for (;;) {
      await whenVisible();
      reset();
      await Promise.all(lines.map((l, i) =>
        animate(l, { clipPath: ['inset(0 100% 0 0)', 'inset(0 0% 0 0)'], opacity: [1, 1] },
          { delay: i * 0.11, duration: 0.45, ease: 'linear' }).finished));
      await sleep(150);
      if (chip) animate(chip, { opacity: 1, scale: [0.92, 1] }, { duration: 0.3 });
      if (ring) animate(ring, { opacity: [0.7, 0], scale: [0.6, 2.4] }, { duration: 0.9, ease: 'easeOut' });
      await sleep(250);
      // Beams draw in once (solid), then become dashed flow with traveling dots.
      beams.forEach((b, i) => {
        b.style.animation = 'none';
        const len = b.getTotalLength();
        b.style.strokeDasharray = `${len}`;
        animate(b, { opacity: [1, 1], strokeDashoffset: [len, 0] }, { delay: i * 0.07, duration: 0.6, ease: 'easeOut' });
      });
      await sleep(450);
      await animate(cards, { opacity: 1, transform: ['translateY(14px) scale(0.94)', 'translateY(0) scale(1)'] },
        { delay: stagger(0.09), duration: 0.5, ease: [0.22, 1, 0.36, 1] }).finished;
      beams.forEach((b) => { b.style.strokeDasharray = ''; b.style.strokeDashoffset = ''; b.style.animation = ''; });
      setFlow(true);
      if (ok) {
        animate(ok, { opacity: 1, scale: [0, 1.25, 1] }, { duration: 0.45, ease: 'easeOut' });
        if (tick) animate(tick, { strokeDashoffset: [1, 0] }, { duration: 0.4, delay: 0.1 });
      }
      await sleep(5200);
      setFlow(false);
      await animate([...lines, ...cards, ...(ok ? [ok] : [])], { opacity: 0 }, { duration: 0.5 }).finished;
    }
  };
  void run();
}
