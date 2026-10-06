import fs from 'node:fs';
import path from 'node:path';
import satori from 'satori';
import { Resvg } from '@resvg/resvg-js';

export interface OgInput {
  eyebrow: string;
  title: string;
  description?: string;
  /** Last title word(s) rendered in the accent color. */
  accent?: string;
}

const root = process.cwd();
const font = (pkg: string, file: string) => fs.readFileSync(path.join(root, 'node_modules/@fontsource', pkg, 'files', file));
let cache: { fonts: { name: string; data: Buffer; weight: 400 | 500 | 700; style: 'normal' }[]; logo: string } | undefined;

function assets() {
  cache ??= {
    fonts: [
      { name: 'Space Grotesk', data: font('space-grotesk', 'space-grotesk-latin-400-normal.woff'), weight: 400, style: 'normal' },
      { name: 'Space Grotesk', data: font('space-grotesk', 'space-grotesk-latin-700-normal.woff'), weight: 700, style: 'normal' },
      { name: 'JetBrains Mono', data: font('jetbrains-mono', 'jetbrains-mono-latin-500-normal.woff'), weight: 500, style: 'normal' },
    ],
    logo: 'data:image/svg+xml;base64,' + fs.readFileSync(path.join(root, 'src/assets/logo.svg')).toString('base64'),
  };
  return cache;
}

type Node = { type: string; props: Record<string, unknown> };
const h = (type: string, style: Record<string, unknown>, children?: unknown, extra: Record<string, unknown> = {}): Node => ({
  type,
  props: { style: { display: 'flex', ...style }, children, ...extra },
});

/** renderOg draws a 1200x630 branded social card and returns PNG bytes. */
export async function renderOg({ eyebrow, title, description, accent }: OgInput): Promise<Uint8Array> {
  const { fonts, logo } = assets();
  const size = title.length > 46 ? 60 : title.length > 28 ? 72 : 84;
  let head = title;
  let tail = '';
  if (accent && title.endsWith(accent)) {
    head = title.slice(0, title.length - accent.length);
    tail = accent;
  }
  const tree = h('div', {
    width: '1200px', height: '630px', flexDirection: 'column', justifyContent: 'space-between', padding: '64px 72px',
    backgroundColor: '#05070d', color: '#eef3fa', fontFamily: 'Space Grotesk',
    backgroundImage: 'radial-gradient(circle at 85% 8%, rgba(108,189,218,0.28) 0%, rgba(5,7,13,0) 55%), radial-gradient(circle at 5% 100%, rgba(81,152,200,0.16) 0%, rgba(5,7,13,0) 50%)',
  }, [
    h('div', { alignItems: 'center', justifyContent: 'space-between' }, [
      h('div', { alignItems: 'center', gap: '14px', fontFamily: 'JetBrains Mono', fontSize: '24px', letterSpacing: '0.12em', color: '#6cbdda', textTransform: 'uppercase' }, [
        h('div', { width: '40px', height: '2px', backgroundColor: '#6cbdda' }, undefined),
        h('div', {}, eyebrow),
      ]),
      h('img', { width: '150px', height: '150px', marginTop: '-30px' }, undefined, { src: logo, width: 150, height: 150 }),
    ]),
    h('div', { flexDirection: 'column', gap: '22px' }, [
      h('div', { flexWrap: 'wrap', fontSize: `${size}px`, fontWeight: 700, lineHeight: 1.04, letterSpacing: '-0.035em', maxWidth: '1000px' }, [
        h('span', { marginRight: tail ? '18px' : '0' }, head.trim()),
        ...(tail ? [h('span', { color: '#6cbdda' }, tail)] : []),
      ]),
      ...(description ? [h('div', { fontSize: '30px', lineHeight: 1.4, color: '#a4b0c3', maxWidth: '980px' }, description.length > 150 ? description.slice(0, 147).trimEnd() + '...' : description)] : []),
    ]),
    h('div', { alignItems: 'center', justifyContent: 'space-between', fontFamily: 'JetBrains Mono', fontSize: '24px', color: '#6b778b' }, [
      h('div', {}, 'zenta-dev.github.io/zever'),
      h('div', { padding: '8px 18px', border: '1px solid rgba(255,255,255,0.18)', borderRadius: '999px', color: '#a4b0c3' }, 'Schema-driven Go backend compiler'),
    ]),
  ]);
  const svg = await satori(tree as never, { width: 1200, height: 630, fonts });
  return new Resvg(svg, { fitTo: { mode: 'width', value: 1200 } }).render().asPng();
}

/** renderIcon draws the mascot on the brand background as a square PNG (app icons, touch icons). */
export function renderIcon(size: number): Uint8Array {
  const { logo } = assets();
  const pad = Math.round(size * 0.1);
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}"><rect width="${size}" height="${size}" rx="${Math.round(size * 0.22)}" fill="#05070d"/><image href="${logo}" x="${pad}" y="${pad}" width="${size - 2 * pad}" height="${size - 2 * pad}"/></svg>`;
  return new Resvg(svg, { fitTo: { mode: 'width', value: size } }).render().asPng();
}
