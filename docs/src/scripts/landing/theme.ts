/** initTheme wires every [data-theme-toggle] button; shares Starlight's storage key. */
export function initTheme(): void {
  const root = document.documentElement;
  // Tokens are scoped to .zl-page; mirror the attribute onto body.
  const apply = () => {
    document.body.dataset.theme = root.dataset.theme;
  };
  apply();
  document.querySelectorAll<HTMLButtonElement>('[data-theme-toggle]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const next = root.dataset.theme === 'light' ? 'dark' : 'light';
      root.dataset.theme = next;
      try { localStorage.setItem('starlight-theme', next); } catch { /* storage blocked */ }
      apply();
    });
  });
}
