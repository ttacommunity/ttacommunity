import { describe, expect, it } from 'vitest';
import type { CalibrationRow } from '@ttac/shared/ratings';
import {
  MODELS,
  activeModels,
  agreementName,
  compositeScore,
  eceClass,
  formatCards,
  announceFormat,
  onFormat,
  parseAnalysisParams,
  sortCountries,
  titleLabel,
  type CountryRatings,
  rankModels,
  type AnalysisFilters,
  seatDelta,
  seatDeltaText,
  seatSpread,
  SEAT_TIERS,
} from './analysis';

const row = (model: CalibrationRow['model'], brier: number, ece: number, extra: Partial<CalibrationRow> = {}): CalibrationRow => ({
  mode: 'standard',
  model,
  format: 0,
  matches: 100,
  brier,
  ece,
  log_loss: 0.6,
  accuracy: 65,
  bins: { labels: ['50-55%', '95-100%'], ideal: [0.525, 0.975], preds: [0.5, 0.9], actuals: [0.6, 0.8], counts: [1, 3] },
  ...extra,
});

describe('MODELS', () => {
  it('lists the three models a run computes, GlickoD first', () => {
    expect(MODELS.map((m) => m.key)).toEqual(['glicko2_daneo', 'glicko2_daneo_softer', 'glicko2_std']);
  });
});

describe('compositeScore', () => {
  // Brier 0.20 → 0.5, ECE 0.02 → 0.75, log loss 0.60 → 0.4, accuracy 65 → 0.5: 57.2499… in floating point, so 57.2
  it.each([
    ['in between', 0.2, 0.02, 0.6, 65, 57.2],
    ['the worst', 0.3, 0.1, 0.8, 40, 0],
    ['the best', 0.1, 0, 0.4, 90, 100],
  ])('weighs Brier and ECE 35%% each, log loss and accuracy 15%%, as upstream: %s', (_name, brier, ece, logLoss, accuracy, want) => {
    expect(compositeScore(brier, ece, logLoss, accuracy)).toBe(want);
  });
});

describe('rankModels', () => {
  it('ranks the chosen mode and format by composite score, with the favourite rates from the bins', () => {
    const rows = [row('glicko2_daneo_softer', 0.2, 0.02), row('glicko2_std', 0.19, 0.02), row('glicko2_std', 0.1, 0, { mode: 'walk_forward' })];
    const ranked = rankModels(rows, 'standard', 0);
    expect(ranked).toHaveLength(3);
    expect(ranked[0]).toMatchObject({ key: 'glicko2_std', rank: 1, brier: 0.19, ecePct: 2 });
    expect(ranked[1]).toMatchObject({ key: 'glicko2_daneo_softer', rank: 2 });
    // Predicted 0.5·1 + 0.9·3 = 3.2 over 4 = 80.0%; actual 0.6 + 2.4 = 3.0 over 4 = 75.0%
    expect(ranked[0]).toMatchObject({ pSollPct: 80, pIstPct: 75, pDeltaPct: -5 });
  });

  it("falls back to upstream's placeholder figures for a model without a row", () => {
    const missing = rankModels([], 'standard', 0).find((m) => m.key === 'glicko2_daneo');
    expect(missing).toMatchObject({ brier: 0.214, ece: 0.015, logLoss: 0.62, accuracy: 64.5, pIstPct: 65, pSollPct: 65 });
  });
});

describe('filters', () => {
  const defaults: AnalysisFilters = { engines: new Set(['glicko2_daneo', 'glicko2_std']), resets: new Set(['continuous']) };

  it("draws upstream's default curves, the continuous models, and the season reset when asked", () => {
    expect(activeModels(defaults)).toEqual(['glicko2_daneo', 'glicko2_std']);
    expect(activeModels({ ...defaults, resets: new Set(['continuous', 'softer']) })).toEqual(['glicko2_daneo', 'glicko2_daneo_softer', 'glicko2_std']);
    expect(activeModels({ ...defaults, engines: new Set(['glicko2_std']), resets: new Set(['softer']) })).toEqual([]);
  });

  it("colours ECE as upstream: under 1.5% good, under 2.5% fair", () => {
    expect([eceClass(1.49), eceClass(1.5), eceClass(2.49), eceClass(2.5)]).toEqual(['good', 'fair', 'fair', 'poor']);
  });
});

describe('parseAnalysisParams', () => {
  it('reads the evaluation mode and format from the address', () => {
    expect(parseAnalysisParams(new URLSearchParams('eval_mode=walk_forward&format=3'), {})).toEqual({ mode: 'walk_forward', format: 3 });
  });

  it('falls back to the remembered format, then standard and all formats', () => {
    expect(parseAnalysisParams(new URLSearchParams(''), { format: 4 })).toEqual({ mode: 'standard', format: 4 });
    expect(parseAnalysisParams(new URLSearchParams('eval_mode=x&format=5'), {})).toEqual({ mode: 'standard', format: 0 });
  });
});

describe('agreementName', () => {
  it.each([
    ['glicko2_daneo', 'GlickoD'],
    ['glicko2_daneo_softer', 'GlickoD (Season Reset)'],
    ['glicko2_std', 'G2 Std (Career)'],
  ] as const)("names %s %s, as upstream's agreement cards do", (model, want) => {
    expect(agreementName(model)).toBe(want);
  });
});

describe('titleLabel', () => {
  it('names every title by its own name, not "Platinum" for all but GM and M', () => {
    expect(['GM', 'M', 'P', 'G', 'S', 'B', 'W', 'WC', 'SGM'].map((t) => titleLabel(t as never))).toEqual([
      'Grandmaster', 'Master', 'Platinum', 'Gold', 'Silver', 'Bronze', 'Wood', 'World Champion', 'Super GM',
    ]);
  });
});

describe('sortCountries', () => {
  const c = (country: string, players: number, gm: number | null): CountryRatings => ({ country, players, gm, m_plus: null, p_plus: null, all: 1500 });
  const rows = [c('US', 131, 1885.6), c('CZ', 33, null), c('FR', 27, 1953.6)].map((r, i) => ({ ...r, rank: i + 1 }));

  it.each([
    ['a number, descending', 'gm', true, ['CZ', 'US', 'FR']],
    ['a number, ascending', 'gm', false, ['FR', 'US', 'CZ']],
    ['codes, as text', 'country', true, ['CZ', 'FR', 'US']],
  ] as const)('sorts by %s, a missing average as 0', (_name, column, descending, want) => {
    expect(sortCountries(rows, column, descending).map((r) => r.country)).toEqual(want);
  });

  it('sorts the rank column as numbers, not as text (1, 2, 3, not 1, 10, 11)', () => {
    const many = Array.from({ length: 11 }, (_, i) => ({ ...c(`C${i}`, 1, null), rank: i + 1 }));
    expect(sortCountries(many, 'rank', true).map((r) => r.rank).slice(0, 4)).toEqual([1, 2, 3, 4]);
    expect(sortCountries(many, 'rank', false).map((r) => r.rank).slice(0, 3)).toEqual([11, 10, 9]);
  });
});

describe('formatCards', () => {
  const formats = [
    { format: 2 as const, games: 47157, pairs: 47157 },
    { format: 3 as const, games: 62714, pairs: 188142 },
    { format: 4 as const, games: 25522, pairs: 153132 },
  ];

  it('totals the games of every format', () => {
    expect(formatCards(formats).total).toBe(135393);
  });

  it('notes each format with its own pair count, in upstream order', () => {
    expect(formatCards(formats).cards).toEqual([
      { format: 3, name: '3-Player Games', note: 'Most popular tournament format (188,142 pairwise duels)', games: 62714 },
      { format: 2, name: '2-Player (Duels)', note: 'Pure 1v1 heads-up duels (47,157 pairwise games)', games: 47157 },
      { format: 4, name: '4-Player Games', note: 'Full table political strategy (153,132 pairwise duels)', games: 25522 },
    ]);
  });

  it('counts a format with no games as zero', () => {
    const { total, cards } = formatCards([{ format: 2, games: 5, pairs: 5 }]);
    expect(total).toBe(5);
    expect(cards[0]).toMatchObject({ format: 3, games: 0, note: 'Most popular tournament format (0 pairwise duels)' });
  });
});

describe('announceFormat and onFormat', () => {
  it('tells every listener the format chosen', () => {
    const target = new EventTarget();
    const heard: number[] = [];
    onFormat(target, (f) => heard.push(f));
    onFormat(target, (f) => heard.push(f * 10));

    announceFormat(target, 3);

    expect(heard).toEqual([3, 30]);
  });

  it('stops telling a listener once it unsubscribes', () => {
    const target = new EventTarget();
    const heard: number[] = [];
    const stop = onFormat(target, (f) => heard.push(f));

    announceFormat(target, 2);
    stop();
    announceFormat(target, 4);

    expect(heard).toEqual([2]);
  });
});

describe('seat helpers', () => {
  it('calculates seat advantage delta relative to parity', () => {
    expect(seatDelta(50.26, 2)).toBe(0.26);
    expect(seatDelta(27.47, 4)).toBe(2.47);
    expect(seatDelta(22.61, 4)).toBe(-2.39);
    expect(seatDelta(33.33, 3)).toBe(0);
    expect(seatDelta(0, 0)).toBe(0);
  });

  it('formats seat advantage delta with explicit sign and two decimals', () => {
    expect(seatDeltaText(2.47)).toBe('+2.47%');
    expect(seatDeltaText(-2.39)).toBe('-2.39%');
    expect(seatDeltaText(0)).toBe('0.00%');
    expect(seatDeltaText(0.004)).toBe('0.00%');
    expect(seatDeltaText(-0.004)).toBe('0.00%');
  });

  it.each([
    ['first seat ahead', 4, [30, 26, 23, 21], 9],
    ['last seat ahead', 3, [31, 33, 36], -5],
    ['two players', 2, [52.5, 47.5], 5],
    ['too few seats', 4, [30, 26, 23], null],
    ['no group', null, [], null],
  ])('seatSpread gives %s', (_name, players, rates, want) => {
    const group = players === null ? undefined : { player_count: players, total_games: 100, seats: rates.map((win_rate, i) => ({ seat: i + 1, games: 100, wins: 0, draws: 0, win_rate, places: [] })) };
    expect(seatSpread(group)).toBe(want);
  });

  it('provides the 6 expected skill tiers', () => {
    expect(SEAT_TIERS).toHaveLength(6);
    expect(SEAT_TIERS.map((t) => t.value)).toEqual([
      'all',
      'grandmaster',
      'master',
      'advanced',
      'intermediate',
      'beginner',
    ]);
  });
});

