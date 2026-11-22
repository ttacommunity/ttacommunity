import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

type RGBA = [number, number, number, number];

const css = readFileSync(new URL('../styles/tokens.css', import.meta.url), 'utf8');
const root = css.slice(css.indexOf(':root {'), css.indexOf('\n}', css.indexOf(':root {')));
const tokens = new Map([...root.matchAll(/^\s*(--[a-z0-9-]+):\s*([^;]+);/gm)].map((m) => [m[1] as string, (m[2] as string).trim()]));

/** resolve replaces every var(--name) in a token's value with that token's own value. */
function resolve(value: string, depth = 0): string {
  if (depth > 20) throw new Error(`token loop at ${value}`);
  return value.replace(/var\((--[a-z0-9-]+)\)/g, (_, name: string) => {
    const v = tokens.get(name);
    if (v === undefined) throw new Error(`${name} is not defined`);
    return resolve(v, depth + 1);
  });
}

/** colour parses a resolved hex, rgb(a), transparent or color-mix(in srgb, …) value. */
function colour(v: string): RGBA {
  v = v.trim();
  if (v === 'transparent') return [0, 0, 0, 0];
  const hex = v.match(/^#([0-9a-f]{3,8})$/i)?.[1];
  if (hex) {
    const h = hex.length <= 4 ? [...hex].map((c) => c + c).join('') : hex;
    const n = (i: number) => parseInt(h.slice(i, i + 2), 16);
    return [n(0), n(2), n(4), h.length === 8 ? n(6) / 255 : 1];
  }
  const rgb = v.match(/^rgba?\(([^)]+)\)$/);
  if (rgb) {
    const p = (rgb[1] as string).split(/[\s,/]+/).filter(Boolean).map(Number);
    return [p[0] as number, p[1] as number, p[2] as number, p[3] ?? 1];
  }
  const mix = v.match(/^color-mix\(in srgb,\s*(.+?)\s+(\d+(?:\.\d+)?)%,\s*(.+)\)$/);
  if (mix) {
    const a = colour(mix[1] as string), b = colour(mix[3] as string), w = Number(mix[2]) / 100;
    const alpha = a[3] * w + b[3] * (1 - w);
    const ch = (i: 0 | 1 | 2) => (alpha === 0 ? 0 : (a[i] * a[3] * w + b[i] * b[3] * (1 - w)) / alpha);
    return [ch(0), ch(1), ch(2), alpha];
  }
  throw new Error(`cannot read the colour ${v}`);
}

const over = (top: RGBA, bottom: RGBA): RGBA => {
  const a = top[3] + bottom[3] * (1 - top[3]);
  const ch = (i: 0 | 1 | 2) => (top[i] * top[3] + bottom[i] * bottom[3] * (1 - top[3])) / a;
  return [ch(0), ch(1), ch(2), a];
};
const luminance = (c: RGBA) => {
  const f = (v: number) => ((v /= 255) <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
  return 0.2126 * f(c[0]) + 0.7152 * f(c[1]) + 0.0722 * f(c[2]);
};
const ratio = (a: RGBA, b: RGBA) => {
  const [x, y] = [luminance(a), luminance(b)];
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
};

/** contrast returns text's contrast on the surfaces given top first, laid over the page's own colour. */
function contrast(text: string, surfaces: string[]): number {
  const page = colour(resolve('var(--bg-dark)'));
  const behind = [...surfaces].reverse().reduce((c, s) => over(colour(resolve(`var(${s})`)), c), page);
  return ratio(over(colour(resolve(`var(${text})`)), behind), behind);
}

describe('contrast arithmetic', () => {
  it.each([
    ['white on black', '#fff', '#000', 21],
    ['grey 777 on white', '#777', '#fff', 4.48],
    ['same colour', '#e5a93c', '#e5a93c', 1],
  ])('%s', (_, a, b, want) => {
    expect(ratio(colour(a), colour(b))).toBeCloseTo(want, 2);
  });
  it('mixes in srgb with transparent', () => {
    expect(colour('color-mix(in srgb, #ff0000 25%, transparent)')).toEqual([255, 0, 0, 0.25]);
  });
});

const BODY = 4.5;
const LARGE = 3;
const PAGE = ['--surface-panel'];
const WELL = ['--surface-well'];
const CARD = ['--surface-card'];

// Each role that draws text, on each surface it is drawn on, and the WCAG minimum it needs.
const pairs: [string, string[], number][] = [
  ...['--text-main', '--text-soft', '--text-muted', '--text-bright', '--text-accent', '--text-accent-bright', '--player-name', '--player-name-hover'].flatMap(
    (t): [string, string[], number][] => [[t, PAGE, BODY], [t, WELL, BODY], [t, CARD, BODY], [t, ['--surface-header'], BODY], [t, ['--surface-footer'], BODY]],
  ),
  ...['--win', '--win-light', '--loss', '--loss-light', '--draw', '--medal-gold', '--medal-silver', '--medal-bronze-text', '--tag-special', '--formula-text'].flatMap(
    (t): [string, string[], number][] => [[t, PAGE, BODY], [t, WELL, BODY]],
  ),
  ...[1, 2, 3, 4, 5, 6, 7, 8].map((n): [string, string[], number] => [`--division-${n}`, ['--tag-surface', ...PAGE], BODY]),
  ['--tier-sgm', PAGE, BODY],
  ['--text-on-gold', ['--fill-accent'], BODY],
  ['--text-on-fill', ['--tier-gm'], BODY],
  ['--text-on-fill', ['--tier-m'], BODY],
  ['--text-on-fill', ['--tier-p'], BODY],
  ['--text-on-fill', ['--tier-g'], BODY],
  ['--text-on-fill', ['--tier-s'], BODY],
  ['--text-on-fill', ['--medal-silver'], BODY],
  ['--text-on-fill', ['--medal-bronze'], BODY],
  ['--text-bright', ['--tier-b'], BODY],
  ['--text-bright', ['--tier-w'], BODY],
  ['--medal-gold-text', ['--medal-gold-fill', ...PAGE], BODY],
  ['--medal-silver', ['--medal-silver-fill', ...PAGE], BODY],
  ['--medal-bronze-text', ['--medal-bronze-fill', ...PAGE], BODY],
  ['--text-main', ['--model-daneo-tint', ...PAGE], BODY],
  ['--text-main', ['--model-daneo-reset-tint', ...PAGE], BODY],
  ['--text-main', ['--model-std-tint', ...PAGE], BODY],
  ['--chip-out-text', ['--chip-out-surface'], BODY],
  ['--count-text', ['--count-surface'], BODY],
  ['--button-pressed-text', ['--button-pressed-surface'], BODY],
  ['--button-text-link', PAGE, BODY],
  ['--button-text-link', WELL, BODY],
  ['--button-text-link', CARD, BODY],
  ['--panel-title', PAGE, LARGE],
  ['--page-header-title', ['--surface-warm'], LARGE],
  ['--note-warning-title', WELL, BODY],
  ['--spoiler-blank', ['--spoiler-surface', ...PAGE], BODY],
];

describe('text roles meet WCAG contrast on their surfaces', () => {
  it.each(pairs.map(([t, s, min]) => [`${t} on ${s.join(' over ')}`, t, s, min] as const))('%s', (_, text, surfaces, min) => {
    expect(contrast(text, surfaces)).toBeGreaterThanOrEqual(min);
  });
});
