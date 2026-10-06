function init() {
  document.querySelectorAll<HTMLElement>('.zp-actions').forEach((root) => {
    if (root.dataset.ready) return;
    root.dataset.ready = '1';
    const trigger = root.querySelector<HTMLButtonElement>('[data-trigger]')!;
    const menu = root.querySelector<HTMLElement>('[data-menu]')!;
    const copy = root.querySelector<HTMLButtonElement>('[data-copy]')!;
    const label = root.querySelector<HTMLElement>('[data-copy-label]')!;
    const live = root.querySelector<HTMLElement>('[data-live]')!;
    const items = () => Array.from(menu.querySelectorAll<HTMLElement>('[role=menuitem]'));

    const close = (refocus = false) => {
      menu.hidden = true;
      trigger.setAttribute('aria-expanded', 'false');
      if (refocus) trigger.focus();
    };
    const open = (focusIdx: number | null) => {
      menu.hidden = false;
      trigger.setAttribute('aria-expanded', 'true');
      if (focusIdx !== null) {
        const list = items();
        list[(focusIdx + list.length) % list.length]?.focus();
      }
    };

    trigger.addEventListener('click', () => (menu.hidden ? open(null) : close()));
    trigger.addEventListener('keydown', (e) => {
      if (e.key === 'ArrowDown') { e.preventDefault(); open(0); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); open(-1); }
    });
    menu.addEventListener('keydown', (e) => {
      const list = items();
      const i = list.indexOf(document.activeElement as HTMLElement);
      if (e.key === 'ArrowDown') { e.preventDefault(); list[(i + 1) % list.length].focus(); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); list[(i - 1 + list.length) % list.length].focus(); }
      else if (e.key === 'Home') { e.preventDefault(); list[0].focus(); }
      else if (e.key === 'End') { e.preventDefault(); list[list.length - 1].focus(); }
      else if (e.key === 'Escape') { e.preventDefault(); close(true); }
      else if (e.key === 'Tab') close();
    });
    menu.addEventListener('click', () => close());
    root.addEventListener('keydown', (e) => {
      if (e.key === 'Escape' && !menu.hidden) { close(true); }
    });
    document.addEventListener('click', (e) => {
      if (!root.isConnected) return;
      if (!menu.hidden && !root.contains(e.target as Node)) close();
    });

    let timer: number | undefined;
    copy.addEventListener('click', async () => {
      try {
        const res = await fetch(root.dataset.md!);
        if (!res.ok) throw new Error(String(res.status));
        await navigator.clipboard.writeText(await res.text());
        copy.classList.add('is-done');
        label.textContent = 'Copied';
        live.textContent = 'Copied';
      } catch {
        label.textContent = 'Copy failed';
        live.textContent = 'Copy failed';
      }
      clearTimeout(timer);
      timer = window.setTimeout(() => {
        copy.classList.remove('is-done');
        label.textContent = 'Copy page';
        live.textContent = '';
      }, 2000);
    });
  });
}
document.addEventListener('astro:page-load', init);
