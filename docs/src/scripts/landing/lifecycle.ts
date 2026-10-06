import { animate } from 'motion';
import { onVisible, reducedMotion } from './motion';

const root = document.querySelector<HTMLElement>('[data-life]');
if (root) {
  const nodes = Array.from(root.querySelectorAll<SVGGElement>('[data-node]'));
  const edges = Array.from(root.querySelectorAll<SVGPathElement>('[data-edge]'));
  const line = root.querySelector<HTMLElement>('[data-line]')!;
  const phaseBtns = Array.from(root.querySelectorAll<HTMLButtonElement>('[data-phase]'));
  const play = root.querySelector<HTMLButtonElement>('[data-play]')!;
  const range = root.querySelector<HTMLInputElement>('[data-range]')!;
  const prev = root.querySelector<HTMLButtonElement>('[data-prev]')!;
  const next = root.querySelector<HTMLButtonElement>('[data-next]')!;
  const STEP = 1000;
  const MAX = 12; // 0..6 resolve, 6..12 close(ctx)

  // r / c come from the markup (container/README.md close order).
  const info = new Map<string, { r: number; c: number }>();
  nodes.forEach((n) => info.set(n.dataset.node!, { r: Number(n.dataset.r), c: Number(n.dataset.c) }));

  const CAPTIONS = [
    'Nothing is open yet. Services resolve on first accessor call.',
    'c.DB()        opens the shared pool',
    'c.Cache()     resolves on first call',
    'c.Queue()     leaf dependency, opens once',
    'c.Job()       builds a dispatcher over the resolved Queue',
    'c.Scheduler() shares Queue via Job(), nothing new opened',
    'other accessors (snapshots, GRPC) resolve lazily on first use',
    'close 1: scheduler, job (dependents holding a queue ref)',
    'close 2: snapshots (everything without explicit ordering)',
    'close 3: cache, queue (leaf dependencies)',
    'close 4: db and registry pools, after every borrower',
    'close 5: grpc server, GracefulStop bounded by ctx',
    'All services closed in dependency order.',
  ];
  const DONE_RESOLVE = 'All services resolved, each exactly once.';

  type NodeState = 'on' | 'off' | 'closing' | 'closed';
  const STATE_SUB: Record<NodeState, string | null> = { on: null, off: 'idle', closing: 'closing…', closed: 'closed ✓' };

  let v = MAX;
  let lastPhase: 'resolve' | 'close' = 'close';
  let timers: number[] = [];
  let anims: { stop: () => void }[] = [];
  let playing = false;

  const stateAt = (n: number) => (n <= 6 ? { phase: 'resolve' as const, s: n } : { phase: 'close' as const, s: n - 6 });

  function clearFx() {
    anims.forEach((a) => { try { a.stop(); } catch { /* already finished */ } });
    anims = [];
    root!.querySelectorAll<SVGCircleElement>('.zl-dot').forEach((d) => { d.style.opacity = '0'; });
    edges.forEach((e) => e.classList.remove('is-hot'));
  }

  function pulse(n: SVGGElement, warm: boolean) {
    const ring = n.querySelector<SVGRectElement>('.zl-n-ring');
    if (ring) animate(ring, { opacity: [0.85, 0], transform: ['scale(1)', warm ? 'scale(1.1, 1.3)' : 'scale(1.16, 1.4)'] }, { duration: 0.8, ease: 'easeOut' });
    animate(n, { transform: ['scale(0.94)', 'scale(1)'], opacity: [0.55, 1] }, { duration: 0.45, ease: [0.22, 1, 0.36, 1] });
  }

  function travel(path: SVGPathElement, reverse: boolean) {
    const svg = path.ownerSVGElement;
    const dot = svg?.querySelector<SVGCircleElement>(`[data-dot="${path.dataset.edge}"]`);
    if (!dot || !path.getClientRects().length) return;
    const len = path.getTotalLength();
    path.classList.add('is-hot');
    dot.style.opacity = '1';
    const a = animate(0, len, {
      duration: 0.75,
      ease: 'easeInOut',
      onUpdate: (l: number) => {
        const p = path.getPointAtLength(reverse ? len - l : l);
        dot.setAttribute('transform', `translate(${p.x} ${p.y})`);
      },
      onComplete: () => { dot.style.opacity = '0'; path.classList.remove('is-hot'); },
    });
    anims.push(a);
  }

  function apply(n: number, fx: boolean) {
    clearFx();
    v = Math.max(0, Math.min(MAX, n));
    const { phase, s } = stateAt(v);
    if (v !== 6) lastPhase = phase;
    root!.dataset.phase = lastPhase;
    if (v >= 6) root!.dataset.badges = '1'; else delete root!.dataset.badges;
    const motion = fx && !reducedMotion();

    nodes.forEach((nd) => {
      const { r, c } = info.get(nd.dataset.node!)!;
      const st: NodeState = phase === 'resolve' ? (r <= s ? 'on' : 'off') : c > s ? 'on' : c === s ? 'closing' : 'closed';
      const was = (['on', 'off', 'closing', 'closed'] as NodeState[]).find((x) => nd.classList.contains(`is-${x}`));
      nd.classList.remove('is-on', 'is-off', 'is-closing', 'is-closed');
      nd.classList.add(`is-${st}`);
      const sub = nd.querySelector<SVGTextElement>('.zl-n-s');
      if (sub) sub.textContent = STATE_SUB[st] ?? sub.dataset.sub ?? '';
      if (motion && was !== st && (st === 'on' || st === 'closing')) pulse(nd, st === 'closing');
    });

    edges.forEach((e) => {
      const from = info.get(e.dataset.from!)!;
      let st: 'live' | 'off' | 'hot';
      if (phase === 'resolve') st = from.r < s ? 'live' : from.r === s ? 'hot' : 'off';
      else st = from.c > s ? 'live' : from.c === s ? 'hot' : 'off';
      e.classList.remove('is-live', 'is-off', 'is-hot');
      if (st === 'hot') {
        // The dot carries the "hot" look while it travels; afterwards the edge settles.
        e.classList.add(phase === 'resolve' ? 'is-live' : 'is-off');
        if (motion) travel(e, phase === 'resolve');
      } else e.classList.add(`is-${st}`);
    });

    const text = v === 6 ? DONE_RESOLVE : CAPTIONS[v];
    line.textContent = text;
    range.value = String(v);
    range.setAttribute('aria-valuetext', text);
    prev.disabled = v === 0;
    next.disabled = v === MAX;
    phaseBtns.forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.phase === lastPhase)));
  }

  function stop() {
    timers.forEach(clearTimeout);
    timers = [];
    playing = false;
    line.setAttribute('aria-live', 'polite');
  }

  /** run steps from..to; announcements are muted while auto-playing and restored at the end. */
  function run(from: number, to: number) {
    stop();
    if (reducedMotion()) { apply(to, false); return; }
    playing = true;
    line.setAttribute('aria-live', 'off');
    apply(from, false);
    for (let k = from + 1; k <= to; k++) {
      timers.push(window.setTimeout(() => {
        apply(k, true);
        if (k === to) { playing = false; line.setAttribute('aria-live', 'polite'); }
      }, (k - from) * STEP));
    }
  }

  phaseBtns.forEach((b) => b.addEventListener('click', () => (b.dataset.phase === 'resolve' ? run(0, 6) : run(6, MAX))));
  play.addEventListener('click', () => (lastPhase === 'resolve' ? run(0, 6) : run(6, MAX)));
  prev.addEventListener('click', () => { stop(); apply(v - 1, true); });
  next.addEventListener('click', () => { stop(); apply(v + 1, true); });
  range.addEventListener('input', () => { stop(); apply(Number(range.value), false); });

  apply(6, false);
  if (reducedMotion()) apply(MAX, false);
  let ran = false;
  onVisible(root, () => {
    if (ran || reducedMotion()) return;
    ran = true;
    if (!playing && v === 6) run(0, MAX);
  }, 0.4);
  void playing;
}
