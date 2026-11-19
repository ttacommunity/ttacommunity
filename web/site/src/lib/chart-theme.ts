/** The site's chart look for Chart.js, read from the theme's tokens: ticks, grid, axis titles, legend and tooltip. */

/** A token's value by its name, such as '--accent-gold'; '' when the theme does not define it. */
export type ReadToken = (name: string) => string;

/** documentTokens reads a token from the page's root element. */
export const documentTokens: ReadToken = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

/** chartTheme returns the chart options every chart on the site shares, from read; fonts fall back to generic families. */
export function chartTheme(read: ReadToken) {
  const gold = read('--accent-gold');
  const stats = read('--font-stats') || 'sans-serif';
  const heading = read('--font-heading') || 'serif';
  return {
    gold,
    ticks: { color: read('--text-soft'), font: { family: stats } },
    grid: { color: read('--border-faint') },
    axisTitle: (text: string) => ({ display: true as const, text, color: gold, font: { family: heading } }),
    legend: { color: read('--text-main'), font: { family: stats, size: 12 }, padding: 12 },
    tooltip: {
      backgroundColor: read('--surface-well'),
      titleColor: gold,
      bodyColor: read('--text-main'),
      titleFont: { family: heading, size: 13, weight: 'bold' as const },
      bodyFont: { family: stats, size: 11 },
      borderColor: read('--gold-wash-strong'),
      borderWidth: 1,
      padding: 10,
    },
  };
}
