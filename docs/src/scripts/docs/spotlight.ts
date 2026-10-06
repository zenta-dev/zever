// Cursor spotlight for docs cards: sets --mx/--my on any `.zd-spot` element. Idempotent.
const w = window as unknown as { __zdSpot?: boolean };
if (!w.__zdSpot) {
  w.__zdSpot = true;
  document.addEventListener(
    'pointermove',
    (e) => {
      const t = e.target;
      if (!(t instanceof Element)) return;
      const el = t.closest<HTMLElement>('.zd-spot');
      if (!el) return;
      const r = el.getBoundingClientRect();
      el.style.setProperty('--mx', `${e.clientX - r.left}px`);
      el.style.setProperty('--my', `${e.clientY - r.top}px`);
    },
    { passive: true },
  );
}
