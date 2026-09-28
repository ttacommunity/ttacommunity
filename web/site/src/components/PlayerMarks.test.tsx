import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { Flag, TitleBadge } from './PlayerMarks';

describe('TitleBadge', () => {
  it('shows the code in its colour, with the reason on hover', () => {
    const html = renderToStaticMarkup(<TitleBadge title={{ code: 'GM', count: 3, reason: 'Played in Royal League' }} />);
    expect(html).toBe('<span class="badge badge-gm" title="Played in Royal League">GM</span>');
  });

  it('falls back to the title name when there is no reason', () => {
    const html = renderToStaticMarkup(<TitleBadge title={{ code: 'WC', count: 1, reason: '' }} />);
    expect(html).toBe('<span class="badge badge-wc" title="World Champion">WC</span>');
  });

  it('shows a dash for a player without a title', () => {
    expect(renderToStaticMarkup(<TitleBadge title={null} />)).toBe('<span class="badge-none">-</span>');
  });
});

describe('Flag', () => {
  it('shows the country flag, named for screen readers', () => {
    expect(renderToStaticMarkup(<Flag country="PL" />)).toBe(
      '<span class="flag-icon" title="Country: PL"><img class="flag-icon-img" src="/flags/pl.svg" alt="PL" width="22" height="15" loading="lazy"/></span>',
    );
  });

  it('shows a faint globe when the country is not known', () => {
    expect(renderToStaticMarkup(<Flag country={null} />)).toBe(
      '<span class="flag-icon flag-none" title="Country: Unknown" role="img" aria-label="Country unknown">🌐</span>',
    );
  });
});
