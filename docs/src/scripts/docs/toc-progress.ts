let bound = false;
function update() {
  const doc = document.documentElement;
  const max = doc.scrollHeight - doc.clientHeight;
  const p = max > 0 ? Math.min(1, Math.max(0, window.scrollY / max)) : 0;
  document.querySelectorAll<HTMLElement>('[data-zp-toc]').forEach((el) => el.style.setProperty('--zp-progress', String(p)));
}
function init() {
  update();
  if (bound) return;
  bound = true;
  window.addEventListener('scroll', () => requestAnimationFrame(update), { passive: true });
  window.addEventListener('resize', update);
}
document.addEventListener('astro:page-load', init);
