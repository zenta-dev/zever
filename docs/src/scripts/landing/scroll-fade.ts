/** edgeFade toggles --fl/--fr mask sizes on a horizontally scrollable row so edges fade only when more content exists. */
export function edgeFade(el: HTMLElement): void {
  const update = () => {
    const max = el.scrollWidth - el.clientWidth;
    el.style.setProperty('--fl', el.scrollLeft > 2 ? '28px' : '0px');
    el.style.setProperty('--fr', max - el.scrollLeft > 2 ? '28px' : '0px');
  };
  el.addEventListener('scroll', update, { passive: true });
  if ('ResizeObserver' in window) new ResizeObserver(update).observe(el);
  window.addEventListener('resize', update, { passive: true });
  update();
}

/** keepVisible scrolls a child into the horizontal view of its scroller without moving the page. */
export function keepVisible(scroller: HTMLElement, child: HTMLElement, smooth = true): void {
  const left = child.offsetLeft - (scroller.clientWidth - child.offsetWidth) / 2;
  scroller.scrollTo({ left, behavior: smooth ? 'smooth' : 'auto' });
}
