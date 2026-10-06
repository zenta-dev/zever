import { onVisible, reducedMotion } from './motion';

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

/** copyText writes to the clipboard, falling back to a hidden textarea. */
async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.cssText = 'position:fixed;opacity:0';
    document.body.appendChild(ta);
    ta.select();
    let ok = false;
    try { ok = document.execCommand('copy'); } catch { /* ignore */ }
    ta.remove();
    return ok;
  }
}

/** initTerminal types the [data-term] transcript step by step; without JS the full transcript is already in the DOM. */
function initTerminal(root: HTMLElement): void {
  const live = root.querySelector<HTMLElement>('[data-live]');
  root.querySelectorAll<HTMLButtonElement>('[data-copy]').forEach((b) => {
    b.addEventListener('click', async () => {
      if (!(await copyText(b.dataset.copy ?? ''))) return;
      b.classList.add('is-copied');
      if (live) live.textContent = 'Copied command';
      setTimeout(() => { b.classList.remove('is-copied'); if (live) live.textContent = ''; }, 1600);
    });
  });

  if (reducedMotion()) return;
  const body = root.querySelector<HTMLElement>('[data-term-body]');
  const steps = Array.from(root.querySelectorAll<HTMLElement>('[data-step]'));
  const dots = Array.from(root.querySelectorAll<HTMLElement>('[data-dots] li'));
  const replay = root.querySelector<HTMLButtonElement>('[data-replay]');
  if (!body || !steps.length) return;
  // Freeze height so typing never shifts layout. Re-measure on resize while idle.
  const freeze = () => { body.style.minHeight = ''; body.style.minHeight = `${body.offsetHeight}px`; };
  freeze();
  let playing = false;
  addEventListener('resize', () => { if (!playing) freeze(); });
  const parts = steps.map((s) => ({
    s,
    cmd: s.querySelector<HTMLElement>('[data-cmd]')!,
    out: s.querySelector<HTMLElement>('[data-out]')!,
    text: s.querySelector<HTMLElement>('[data-cmd]')!.textContent ?? '',
  }));
  let run = 0;

  const reset = () => {
    root.classList.add('is-typing');
    root.classList.remove('is-idle');
    parts.forEach((p) => {
      p.s.classList.remove('is-shown', 'is-active');
      p.out.classList.remove('is-in');
      p.cmd.textContent = '';
    });
    dots.forEach((d) => d.classList.remove('is-active', 'is-done'));
  };

  async function play(): Promise<void> {
    const id = ++run;
    playing = true;
    reset();
    if (replay) replay.disabled = true;
    for (let k = 0; k < parts.length; k++) {
      const p = parts[k];
      p.s.classList.add('is-shown', 'is-active');
      dots[k]?.classList.add('is-active');
      for (let i = 1; i <= p.text.length; i++) {
        if (id !== run) return;
        p.cmd.textContent = p.text.slice(0, i);
        await sleep(14 + Math.random() * 18);
      }
      await sleep(260);
      if (id !== run) return;
      p.out.classList.add('is-in');
      p.s.classList.remove('is-active');
      dots[k]?.classList.replace('is-active', 'is-done');
      await sleep(520);
    }
    if (id !== run) return;
    root.classList.remove('is-typing');
    root.classList.add('is-idle');
    playing = false;
    if (replay) replay.disabled = false;
  }

  replay?.addEventListener('click', () => void play());
  onVisible(root, () => void play(), 0.4);
}

document.querySelectorAll<HTMLElement>('[data-term]').forEach(initTerminal);
