import { reducedMotion } from './motion';

/** wireCopy attaches copy-to-clipboard to every [data-copy] button under root, announcing via the sibling [data-copy-status]. */
export function wireCopy(root: ParentNode, textFor?: (btn: HTMLElement) => string): void {
  root.querySelectorAll<HTMLButtonElement>('[data-copy]').forEach((btn) => {
    const win = btn.closest('.zl-win');
    const status = win?.querySelector<HTMLElement>('[data-copy-status]');
    let timer = 0;
    btn.addEventListener('click', async () => {
      const text = textFor ? textFor(btn) : win?.querySelector('.zl-win-body pre')?.textContent ?? '';
      let ok = false;
      try {
        await navigator.clipboard.writeText(text.replace(/\n$/, ''));
        ok = true;
      } catch {
        ok = false;
      }
      if (status) status.textContent = ok ? 'Copied' : 'Copy failed';
      if (!ok) return;
      btn.classList.add('is-copied');
      window.clearTimeout(timer);
      timer = window.setTimeout(() => {
        btn.classList.remove('is-copied');
        if (status) status.textContent = '';
      }, 1800);
    });
  });
}

/** initWalkthrough wires step cards, scroll-driven activation, the tab pill and the ARIA tablist. */
export function initWalkthrough(): void {
  const root = document.querySelector<HTMLElement>('[data-walkthrough]');
  if (!root) return;
  const tablist = root.querySelector<HTMLElement>('[role=tablist]')!;
  const tabs = [...root.querySelectorAll<HTMLButtonElement>('[role=tab]')];
  const panels = [...root.querySelectorAll<HTMLElement>('[role=tabpanel]')];
  const steps = [...root.querySelectorAll<HTMLElement>('.sw-step')];
  const pill = root.querySelector<HTMLElement>('.sw-pill');
  const status = root.querySelector<HTMLElement>('[data-sw-status]');
  const mdesc = root.querySelector<HTMLElement>('[data-sw-desc]');
  const stepData = steps.map((s) => ({
    t: s.querySelector('h3')?.textContent ?? '',
    d: s.querySelector('p')?.textContent ?? '',
  }));
  let step = -1;
  let announce = false;

  const setStep = (n: number) => {
    if (n === step) return;
    step = n;
    steps.forEach((s, i) => {
      if (i === n) s.setAttribute('aria-current', 'step');
      else s.removeAttribute('aria-current');
      s.classList.toggle('is-past', i < n);
    });
    root.style.setProperty('--sw-prog', String(steps.length > 1 ? n / (steps.length - 1) : 1));
    root.dataset.step = String(n);
    root.querySelectorAll('.sw-dot').forEach((d, i) => d.classList.toggle('is-on', i <= n));
    if (mdesc) mdesc.textContent = stepData[n]?.d ?? '';
  };

  const movePill = (tab: HTMLElement, animate: boolean) => {
    if (!pill) return;
    pill.style.transition = animate && !reducedMotion() ? '' : 'none';
    pill.style.transform = `translate(${tab.offsetLeft}px, ${tab.offsetTop}px) scale(${tab.offsetWidth / 100}, ${tab.offsetHeight / 100})`;
    if (!animate) void pill.offsetWidth;
  };

  const updateFades = () => {
    const max = tablist.scrollWidth - tablist.clientWidth;
    tablist.classList.toggle('fade-start', tablist.scrollLeft > 4);
    tablist.classList.toggle('fade-end', tablist.scrollLeft < max - 4);
  };

  const setTab = (id: string, focus = false, animate = true) => {
    tabs.forEach((t) => {
      const on = t.dataset.tab === id;
      t.setAttribute('aria-selected', String(on));
      t.tabIndex = on ? 0 : -1;
      if (!on) return;
      if (focus) t.focus({ preventScroll: true });
      setStep(Number(t.dataset.step));
      movePill(t, animate);
      const left = t.offsetLeft - (tablist.clientWidth - t.offsetWidth) / 2;
      tablist.scrollTo({ left, behavior: animate && !reducedMotion() ? 'smooth' : 'auto' });
      if (announce && status) status.textContent = `${t.textContent?.trim()} selected. Step ${step + 1} of ${steps.length}: ${stepData[step]?.t}.`;
    });
    panels.forEach((p) => p.classList.toggle('is-active', p.dataset.panel === id));
  };
  const stepFirstTab = (n: number) => tabs.find((t) => Number(t.dataset.step) === n)?.dataset.tab ?? 'zen';

  setTab('zen', false, false);
  announce = true;
  updateFades();
  tablist.addEventListener('scroll', updateFades, { passive: true });
  if ('ResizeObserver' in window) {
    new ResizeObserver(() => {
      const on = tabs.find((t) => t.getAttribute('aria-selected') === 'true');
      if (on) movePill(on, false);
      updateFades();
    }).observe(tablist);
  }
  root.classList.add('is-ready');

  tabs.forEach((t, i) => {
    t.addEventListener('click', () => setTab(t.dataset.tab!));
    t.addEventListener('keydown', (e) => {
      let j = -1;
      if (e.key === 'ArrowRight') j = (i + 1) % tabs.length;
      else if (e.key === 'ArrowLeft') j = (i - 1 + tabs.length) % tabs.length;
      else if (e.key === 'Home') j = 0;
      else if (e.key === 'End') j = tabs.length - 1;
      if (j < 0) return;
      e.preventDefault();
      setTab(tabs[j].dataset.tab!, true);
    });
  });

  const activeStep = () => Number(tabs.find((t) => t.getAttribute('aria-selected') === 'true')?.dataset.step);
  root.querySelectorAll<HTMLButtonElement>('[data-step-btn]').forEach((b) =>
    b.addEventListener('click', () => {
      const n = Number(b.dataset.stepBtn);
      if (n !== activeStep()) setTab(stepFirstTab(n));
      if (!window.matchMedia('(min-width: 900px)').matches) {
        // Keep the active chip centred in its scroller.
        const rail = b.closest<HTMLElement>('.sw-rail');
        const li = b.closest<HTMLElement>('.sw-step');
        if (rail && li) rail.scrollTo({ left: li.offsetLeft - (rail.clientWidth - li.offsetWidth) / 2, behavior: reducedMotion() ? 'auto' : 'smooth' });
      }
    }),
  );
  root.querySelectorAll<HTMLElement>('.sw-step-btn').forEach((b) => b.addEventListener('keydown', (e) => {
    const k = (e as KeyboardEvent).key;
    if (k !== 'ArrowRight' && k !== 'ArrowLeft') return;
    const btns = [...root.querySelectorAll<HTMLElement>('.sw-step-btn')];
    const j = (btns.indexOf(b) + (k === 'ArrowRight' ? 1 : btns.length - 1)) % btns.length;
    btns[j].focus({ preventScroll: true });
    e.preventDefault();
  }));

  wireCopy(root, () => root.querySelector('.sw-panel.is-active pre')?.textContent ?? '');

  // Desktop: the step cards stack and stick; derive the active step from scroll position.
  const desktop = window.matchMedia('(min-width: 900px)');
  const rail = root.querySelector<HTMLElement>('.sw-rail')!;
  let lastScrollStep = -1;
  let raf = 0;
  const onScroll = () => {
    raf = 0;
    if (!desktop.matches || steps.length < 2) return;
    const top = rail.getBoundingClientRect().top;
    const last = steps[steps.length - 1];
    const stride = (rail.offsetHeight - last.offsetHeight) / (steps.length - 1);
    const probe = window.innerHeight * 0.3; // viewport line at which a card counts as "reached"
    const raw = (probe - top) / stride;
    const n = Math.max(0, Math.min(steps.length - 1, Math.floor(raw + 0.15)));
    if (n === lastScrollStep) return;
    lastScrollStep = n;
    if (n !== activeStep()) setTab(stepFirstTab(n), false, true);
  };
  window.addEventListener('scroll', () => { if (!raf) raf = requestAnimationFrame(onScroll); }, { passive: true });
  window.addEventListener('resize', () => { lastScrollStep = -1; });
  onScroll();
}
