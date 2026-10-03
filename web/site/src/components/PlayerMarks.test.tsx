import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { ExtraBadges, Flag, TitleBadge } from './PlayerMarks';

describe('TitleBadge', () => {
  it('shows the code in its colour, with the reason on hover', () => {
    const html = renderToStaticMarkup(<TitleBadge title={{ code: 'GM', count: 3, reason: 'Played in Royal League' }} />);
    expect(html).toBe('<span class="badge badge-gm" title="Played in Royal League">GM</span>');
  });

  it('falls back to the title name when there is no reason', () => {
    const html = renderToStaticMarkup(<TitleBadge title={{ code: 'WC', count: 1, reason: '' }} />);
    expect(html).toBe('<span class="badge badge-wc" title="World Champion">WC</span>');
  });

  it('shows a Super GM as GM in its own colour, as upstream', () => {
    const html = renderToStaticMarkup(<TitleBadge title={{ code: 'SGM', count: 2, reason: 'Champion of Royal League Emperor (Season 8)' }} />);
    expect(html).toBe('<span class="badge badge-sgm" title="Champion of Royal League Emperor (Season 8)">GM</span>');
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

describe('ExtraBadges', () => {
  it('shows each badge admin gave, with its description on hover, or its label when it has none', () => {
    const html = renderToStaticMarkup(
      <ExtraBadges
        badges={[
          { label: 'Organiser', description: 'Runs a league' },
          { label: 'TD', description: '' },
        ]}
      />,
    );
    expect(html).toBe(
      '<span class="badge badge-extra" title="Runs a league">Organiser</span><span class="badge badge-extra" title="TD">TD</span>',
    );
  });

  it('uses the page tooltip on the profile, and shows nothing for no badges', () => {
    expect(renderToStaticMarkup(<ExtraBadges badges={[{ label: 'TD', description: 'Director' }]} tooltip />)).toBe(
      '<span class="badge badge-extra" data-tooltip="Director">TD</span>',
    );
    expect(renderToStaticMarkup(<ExtraBadges badges={undefined} />)).toBe('');
  });
});
