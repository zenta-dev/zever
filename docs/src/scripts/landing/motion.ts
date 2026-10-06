import { inView } from 'motion';

export const reducedMotion = (): boolean =>
  window.matchMedia('(prefers-reduced-motion: reduce)').matches;

/** onVisible runs fn once when el enters the viewport (immediately if reduced motion). */
export function onVisible(el: Element, fn: () => void, amount = 0.3): void {
  if (reducedMotion()) return fn();
  inView(el, () => {
    fn();
  }, { amount });
}

/** revealAll marks every .zl-reveal element visible as it scrolls in. */
export function revealAll(root: ParentNode = document): void {
  root.querySelectorAll<HTMLElement>('.zl-reveal').forEach((el) => {
    onVisible(el, () => el.classList.add('is-in'), 0.15);
  });
}
