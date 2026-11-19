import { describe, expect, it } from 'vitest';
import { chartTheme } from './chart-theme';

const tokens: Record<string, string> = {
  '--accent-gold': 'gold',
  '--border-faint': 'faint',
  '--text-soft': 'soft',
  '--text-main': 'main',
  '--font-stats': 'Stats',
  '--font-heading': 'Heading',
  '--surface-well': 'well',
  '--gold-wash-strong': 'wash',
};
const read = (name: string) => tokens[name] ?? '';

describe('chartTheme', () => {
  it.each([
    ['tick numbers', () => chartTheme(read).ticks, { color: 'soft', font: { family: 'Stats' } }],
    ['grid lines', () => chartTheme(read).grid, { color: 'faint' }],
    ['axis titles', () => chartTheme(read).axisTitle('Players'), { display: true, text: 'Players', color: 'gold', font: { family: 'Heading' } }],
    ['legend labels', () => chartTheme(read).legend, { color: 'main', font: { family: 'Stats', size: 12 }, padding: 12 }],
  ])('draws %s in the site tokens', (_, part, want) => {
    expect(part()).toEqual(want);
  });

  it('frames tooltips in the site tokens', () => {
    expect(chartTheme(read).tooltip).toMatchObject({ backgroundColor: 'well', titleColor: 'gold', bodyColor: 'main', borderColor: 'wash', titleFont: { family: 'Heading' }, bodyFont: { family: 'Stats' } });
  });

  it('falls back to generic fonts when the fonts are not defined', () => {
    const theme = chartTheme(() => '');
    expect([theme.ticks.font.family, theme.axisTitle('x').font.family]).toEqual(['sans-serif', 'serif']);
  });
});
