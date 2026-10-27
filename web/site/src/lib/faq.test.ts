import { describe, expect, it } from 'vitest';
import { count } from '@ttac/shared/numbers';
import type { Activity, CalibrationRow } from '@ttac/shared/ratings';
import { FAQ_ALIASES, faqFigures, resolveTab } from './faq';

const row = (model: CalibrationRow['model'], format: CalibrationRow['format'], brier: number, ece: number, matches = 1000): CalibrationRow => ({
  mode: 'walk_forward',
  model,
  format,
  matches,
  brier,
  ece,
  log_loss: 0.6,
  accuracy: 65,
  bins: { labels: [], ideal: [], preds: [], actuals: [], counts: [] },
});

describe('faqFigures', () => {
  it("formats the walk-forward figures as upstream's FAQ: Brier to 4 places, ECE as a percentage to 2", () => {
    const rows = [
      row('glicko2_std', 0, 0.2143, 0.0147),
      row('glicko2_std', 4, 0.224, 0.0324),
      row('glicko2_daneo_softer', 0, 0.2070, 0.006),
      row('glicko2_daneo', 0, 0.2050, 0.005),
      { ...row('glicko2_std', 0, 0.1, 0.1), mode: 'standard' as const },
    ];
    const activity = { run_id: 2, active: 1579, inactive: 1910, formats: [], titles: [], countries: [] } satisfies Activity;
    expect(faqFigures(rows, activity)).toEqual({
      'std.0.brier': '0.2143',
      'std.0.ece': '1.47',
      'std.4.brier': '0.2240',
      'std.4.ece': '3.24',
      'daneo_softer.0.brier': '0.2070',
      'daneo_softer.0.ece': '0.60',
      players: '1579',
    });
  });

  it('leaves out a figure whose row is missing, so the page keeps its fallback (the retired models always do)', () => {
    expect(faqFigures([row('glicko2_std', 0, 0.2, 0.01)], null)).toEqual({ 'std.0.brier': '0.2000', 'std.0.ece': '1.00' });
  });
});

describe('resolveTab', () => {
  const tabs = ['philosophy', 'gold-standard', 'recalibration-15', 'resets'];

  it.each([
    ['an alias', '15game', 'recalibration-15'],
    ['another alias', 'gold', 'gold-standard'],
    ['a tab by its own name', 'resets', 'resets'],
    ['an unknown tab, refused', 'nope', null],
    ['no tab, refused', '', null],
  ])("resolves %s, as upstream's tabs do", (_name, asked, want) => {
    expect(resolveTab(asked, tabs)).toBe(want);
  });

  it("keeps upstream's 11 aliases", () => {
    expect(Object.keys(FAQ_ALIASES)).toHaveLength(11);
  });
});

describe('faqFigures from the run', () => {
  const run = {
    run_id: 6, published_at: '2026-10-01T02:00:00Z', engine_version: 'a', finished_at: null,
    games: 135393, players: 3526, first_game: '2017-09-25', last_game: '2026-08-31',
  };

  it("adds the run's games, players and years", () => {
    expect(faqFigures([], null, run)).toEqual({ 'run.games': count(135393), 'run.players': count(3526), 'run.years': '2017–2026' });
  });

  it('leaves out what the run did not record', () => {
    expect(faqFigures([], null, { ...run, games: null, players: null, first_game: null })).toEqual({});
  });
});
