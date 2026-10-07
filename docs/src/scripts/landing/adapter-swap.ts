import { animate } from 'motion';
import { reducedMotion } from './motion';
import { edgeFade, keepVisible } from './scroll-fade';

interface Adapter { id: string; config: string }
interface Service { id: string; label: string; adapters: Adapter[]; call: string }

const root = document.querySelector<HTMLElement>('[data-swap]');
const dataEl = document.getElementById('zl-swap-data');

if (root && dataEl) {
  const swap: Service[] = JSON.parse(dataEl.textContent || '[]');
  const tabsEl = root.querySelector<HTMLElement>('[data-tabs]')!;
  const tabs = Array.from(root.querySelectorAll<HTMLButtonElement>('[role=tab]'));
  const chipsEl = root.querySelector<HTMLElement>('[data-chips]')!;
  const yamlEl = root.querySelector<HTMLElement>('[data-yaml]')!;
  const callEl = root.querySelector<HTMLElement>('[data-call]')!;
  const live = root.querySelector<HTMLElement>('[data-live]')!;
  const changedTag = root.querySelector<HTMLElement>('[data-changed]')!;
  const pinned = root.querySelector<HTMLElement>('[data-pinned]')!;
  const containerBox = root.querySelector<SVGGElement>('.zl-d-box--c');
  const svcs = Array.from(root.querySelectorAll<SVGGElement>('[data-svc]'));

  let svc = swap[0];
  let adapter = svc.adapters[0];
  let prevLines: string[] = adapter.config.split('\n');

  const EASE: [number, number, number, number] = [0.22, 1, 0.36, 1];
  interface Rect { x: number; y: number; w: number; h: number }
  function mkInd(host: HTMLElement) {
    const el = document.createElement('span');
    el.className = 'zl-swap-ind';
    el.setAttribute('aria-hidden', 'true');
    el.style.opacity = '0';
    return { el, host, prev: null as Rect | null };
  }
  const tabInd = mkInd(tabsEl);
  const chipInd = mkInd(chipsEl);
  tabsEl.prepend(tabInd.el);

  /** FLIP: size is set to the target, the old box is applied as an inverted transform, then released. */
  function moveInd(ind: ReturnType<typeof mkInd>, btn: HTMLElement | null, animateIt: boolean) {
    if (!btn) return;
    const r: Rect = { x: btn.offsetLeft, y: btn.offsetTop, w: btn.offsetWidth, h: btn.offsetHeight };
    const el = ind.el;
    el.style.width = `${r.w}px`;
    el.style.height = `${r.h}px`;
    const to = `translate(${r.x}px, ${r.y}px) scale(1, 1)`;
    const from = ind.prev;
    ind.prev = r;
    el.style.opacity = '1';
    if (animateIt && from && !reducedMotion()) {
      animate(el, { transform: [`translate(${from.x}px, ${from.y}px) scale(${from.w / r.w}, ${from.h / r.h})`, to] }, { duration: 0.45, ease: EASE });
    } else {
      el.style.transform = to;
    }
  }
  const place = (animateIt: boolean) => {
    moveInd(tabInd, tabs.find((t) => t.getAttribute('aria-selected') === 'true') ?? null, animateIt);
    moveInd(chipInd, chipsEl.querySelector<HTMLElement>('[aria-checked=true]'), animateIt);
  };

  function renderChips(focus = false, animateIt = false, reset = false) {
    const btns = svc.adapters.map((a) => {
      const b = document.createElement('button');
      b.type = 'button';
      b.setAttribute('role', 'radio');
      b.dataset.adapter = a.id;
      b.textContent = a.id;
      const on = a.id === adapter.id;
      b.setAttribute('aria-checked', String(on));
      b.tabIndex = on ? 0 : -1;
      return b;
    });
    chipsEl.replaceChildren(chipInd.el, ...btns);
    if (reset) { chipInd.prev = null; chipsEl.scrollLeft = 0; }
    const on = btns.find((b) => b.getAttribute('aria-checked') === 'true');
    if (on && focus) on.focus({ preventScroll: true });
    moveInd(chipInd, on ?? null, animateIt);
    if (on) keepVisible(chipsEl, on, animateIt);
  }

  function renderYaml(flash: boolean) {
    const lines = adapter.config.split('\n');
    const old = prevLines;
    yamlEl.replaceChildren();
    let changes = 0;
    const ghosts: HTMLElement[] = [];
    const adds: HTMLElement[] = [];
    lines.forEach((l, i) => {
      const changed = flash && old[i] !== l;
      if (changed && old[i] !== undefined) {
        const g = document.createElement('span');
        g.className = 'zl-ln is-del';
        g.dataset.g = '-';
        g.textContent = old[i] || ' ';
        yamlEl.appendChild(g);
        ghosts.push(g);
      }
      const s = document.createElement('span');
      s.className = 'zl-ln';
      s.dataset.g = changed ? '+' : ' ';
      s.textContent = l || ' ';
      yamlEl.appendChild(s);
      if (changed) { changes++; adds.push(s); s.classList.add('is-add'); }
    });
    prevLines = lines;
    changedTag.hidden = !(flash && changes);
    if (!flash) return;

    const motion = !reducedMotion();
    if (motion) {
      adds.forEach((s, i) => animate(s, { opacity: [0, 1], transform: ['translateX(-8px)', 'translateX(0)'] }, { duration: 0.45, delay: i * 0.05 }));
      ghosts.forEach((g, i) => animate(g, { opacity: [0, 1] }, { duration: 0.25, delay: i * 0.05 }));
    }
    window.setTimeout(() => {
      const fade = () => { ghosts.forEach((g) => g.remove()); adds.forEach((s) => { s.classList.remove('is-add'); s.dataset.g = ' '; }); };
      if (motion && ghosts.length) {
        Promise.all(ghosts.map((g) => animate(g, { opacity: 0 }, { duration: 0.3 }).finished.catch(() => {}))).then(fade);
      } else fade();
    }, motion ? 1800 : 2600);
  }

  function pulse() {
    pinned.classList.remove('is-pulse');
    void pinned.offsetWidth;
    pinned.classList.add('is-pulse');
  }

  let swapToken = 0;
  function renderDiagram() {
    const my = ++swapToken;
    const motion = !reducedMotion();
    svcs.forEach((g) => {
      const on = g.dataset.svc === svc.id;
      g.classList.toggle('is-on', on);
      const t = g.querySelector<SVGTextElement>('.zl-d-ad');
      if (!t) return;
      if (!on) { t.textContent = ''; t.style.opacity = ''; t.style.transform = ''; return; }
      if (t.textContent === adapter.id) return;
      const set = () => {
        if (my !== swapToken) return;
        t.textContent = adapter.id;
        if (motion) animate(t, { opacity: [0, 1], transform: ['translateY(7px)', 'translateY(0)'] }, { duration: 0.28, ease: EASE });
        else t.style.opacity = '1';
      };
      if (motion && t.textContent) {
        animate(t, { opacity: [1, 0], transform: ['translateY(0)', 'translateY(-7px)'] }, { duration: 0.16 }).finished.then(set, set);
      } else set();
    });
    if (motion && containerBox) animate(containerBox, { transform: ['scale(1)', 'scale(1.05)', 'scale(1)'] }, { duration: 0.5, ease: EASE });
  }

  function selectService(id: string, focus = false) {
    const next = swap.find((s) => s.id === id);
    if (!next) return;
    svc = next;
    adapter = svc.adapters[0];
    tabs.forEach((t) => {
      const on = t.dataset.service === id;
      t.setAttribute('aria-selected', String(on));
      t.tabIndex = on ? 0 : -1;
      if (on && focus) t.focus({ preventScroll: true });
    });
    chipsEl.setAttribute('aria-label', `${svc.label} adapter`);
    callEl.textContent = svc.call;
    renderChips(false, false, true);
    renderYaml(true);
    renderDiagram();
    moveInd(tabInd, tabs.find((t) => t.dataset.service === id) ?? null, true);
    const selTab = tabs.find((t) => t.dataset.service === id);
    if (selTab) keepVisible(tabsEl, selTab);
    pulse();
    announce();
  }

  function selectAdapter(id: string, focus = false) {
    const a = svc.adapters.find((x) => x.id === id);
    if (!a || a === adapter) return;
    adapter = a;
    renderChips(focus, true);
    renderYaml(true);
    renderDiagram();
    pulse();
    announce();
  }

  function announce() {
    live.textContent = `${svc.label} adapter set to ${adapter.id}. handler.go is unchanged.`;
  }

  function arrow(e: KeyboardEvent, count: number, cur: number): number | null {
    switch (e.key) {
      case 'ArrowRight': case 'ArrowDown': return (cur + 1) % count;
      case 'ArrowLeft': case 'ArrowUp': return (cur - 1 + count) % count;
      case 'Home': return 0;
      case 'End': return count - 1;
      default: return null;
    }
  }

  tabs.forEach((t, i) => {
    t.addEventListener('click', () => selectService(t.dataset.service!));
    t.addEventListener('keydown', (e) => {
      const n = arrow(e, tabs.length, i);
      if (n === null) return;
      e.preventDefault();
      selectService(tabs[n].dataset.service!, true);
    });
  });

  chipsEl.addEventListener('click', (e) => {
    const b = (e.target as HTMLElement).closest<HTMLElement>('[data-adapter]');
    if (b) selectAdapter(b.dataset.adapter!);
  });
  chipsEl.addEventListener('keydown', (e) => {
    const list = svc.adapters;
    const cur = list.findIndex((a) => a.id === adapter.id);
    const n = arrow(e, list.length, cur);
    if (n === null) return;
    e.preventDefault();
    selectAdapter(list[n].id, true);
  });

  renderChips();
  edgeFade(tabsEl);
  edgeFade(chipsEl);
  place(false);
  window.addEventListener('resize', () => place(false), { passive: true });
  document.fonts?.ready.then(() => place(false));
}
