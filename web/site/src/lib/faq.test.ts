import { describe, expect, it } from 'vitest';
import type { Agreement, CalibrationRow } from '@ttac/shared/ratings';
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
      row('glicko2_mp', 0, 0.21406, 0.011549, 308863),
      row('glicko2_std', 0, 0.2143, 0.0147),
      row('glicko2_mp', 4, 0.22235, 0.0196),
      row('glicko2_std', 4, 0.224, 0.0324),
      { ...row('glicko2_mp', 0, 0.1, 0.1), mode: 'standard' as const },
    ];
    const agreement = { run_id: 2, format: 0, sample: 3151, top: [], weakest: [] } satisfies Agreement;
    expect(faqFigures(rows, agreement)).toEqual({
      'mp.0.brier': '0.2141',
      'mp.0.ece': '1.15',
      'mp.0.matches': '308,863',
      'std.0.brier': '0.2143',
      'std.0.ece': '1.47',
      'mp.4.brier': '0.2223',
      'mp.4.ece': '1.96',
      'std.4.brier': '0.2240',
      'std.4.ece': '3.24',
      players: '3151',
    });
  });

  it('leaves out a figure whose row is missing, so the page keeps its fallback', () => {
    expect(faqFigures([row('glicko2_mp', 0, 0.2, 0.01)], null)).toEqual({ 'mp.0.brier': '0.2000', 'mp.0.ece': '1.00', 'mp.0.matches': '1,000' });
  });
});

describe('resolveTab', () => {
  it("resolves upstream's tab aliases and refuses unknown tabs", () => {
    const tabs = ['philosophy', 'gold-standard', 'recalibration-15', 'resets'];
    expect(resolveTab('15game', tabs)).toBe('recalibration-15');
    expect(resolveTab('gold', tabs)).toBe('gold-standard');
    expect(resolveTab('resets', tabs)).toBe('resets');
    expect(resolveTab('nope', tabs)).toBeNull();
    expect(resolveTab('', tabs)).toBeNull();
    expect(Object.keys(FAQ_ALIASES)).toHaveLength(11);
  });
});

describe('faqFigures from the run', () => {
  const run = {
    run_id: 6, published_at: '2026-10-01T02:00:00Z', engine_version: 'a', finished_at: null,
    games: 135393, players: 3526, first_game: '2017-09-25', last_game: '2026-08-31',
  };

  it("adds the run's games, players and years", () => {
    expect(faqFigures([], null, run)).toEqual({ 'run.games': '135,393', 'run.players': '3,526', 'run.years': '2017–2026' });
  });

  it('leaves out what the run did not record', () => {
    expect(faqFigures([], null, { ...run, games: null, players: null, first_game: null })).toEqual({});
  });
});
