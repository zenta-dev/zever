function init() {
  document.querySelectorAll<HTMLElement>('[data-helpful]').forEach((root) => {
    if (root.dataset.ready) return;
    root.dataset.ready = '1';
    const key = 'zever-helpful:' + location.pathname;
    const ask = root.querySelector<HTMLElement>('[data-ask]')!;
    const yes = root.querySelector<HTMLElement>('[data-yes]')!;
    const no = root.querySelector<HTMLElement>('[data-no]')!;
    const link = root.querySelector<HTMLAnchorElement>('[data-issue-link]')!;
    link.href = (root.dataset.issue ?? '') + encodeURIComponent(location.href);
    const show = (v: string | null) => {
      ask.hidden = v === 'yes' || v === 'no';
      yes.hidden = v !== 'yes';
      no.hidden = v !== 'no';
    };
    let saved: string | null = null;
    try { saved = sessionStorage.getItem(key); } catch { /* storage unavailable */ }
    show(saved);
    root.querySelectorAll<HTMLButtonElement>('[data-vote]').forEach((b) =>
      b.addEventListener('click', () => {
        const v = b.dataset.vote!;
        try { sessionStorage.setItem(key, v); } catch { /* ignore */ }
        show(v);
      }),
    );
  });
}
document.addEventListener('astro:page-load', init);
