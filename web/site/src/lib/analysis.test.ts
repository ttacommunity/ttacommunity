import { describe, expect, it } from 'vitest';
import type { CalibrationRow } from '@ttac/shared/ratings';
import {
  MODELS,
  activeModels,
  agreementName,
  compositeScore,
  eceClass,
  parseAnalysisParams,
  rankModels,
  shownInTable,
  type AnalysisFilters,
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
  it("lists upstream's 16 models in its order", () => {
    expect(MODELS).toHaveLength(16);
    expect(MODELS.slice(0, 3).map((m) => m.key)).toEqual(['glicko2_daneo', 'glicko2_daneo_softer', 'glicko2_adapt']);
    expect(MODELS.at(-1)?.key).toBe('whr_softer');
  });
});

describe('compositeScore', () => {
  it("weighs Brier and ECE 35% each, log loss and accuracy 15%, as upstream", () => {
    // Brier 0.20 → 0.5, ECE 0.02 → 0.75, log loss 0.60 → 0.4, accuracy 65 → 0.5: 57.2499… in floating point, so 57.2
    expect(compositeScore(0.2, 0.02, 0.6, 65)).toBe(57.2);
    expect(compositeScore(0.3, 0.1, 0.8, 40)).toBe(0);
    expect(compositeScore(0.1, 0, 0.4, 90)).toBe(100);
  });
});

describe('rankModels', () => {
  it('ranks the chosen mode and format by composite score, with the favourite rates from the bins', () => {
    const rows = [row('whr', 0.2, 0.02), row('glicko2_std', 0.19, 0.02), row('glicko2_std', 0.1, 0, { mode: 'walk_forward' })];
    const ranked = rankModels(rows, 'standard', 0);
    expect(ranked).toHaveLength(16);
    expect(ranked[0]).toMatchObject({ key: 'glicko2_std', rank: 1, brier: 0.19, ecePct: 2 });
    expect(ranked[1]).toMatchObject({ key: 'whr', rank: 2 });
    // Predicted 0.5·1 + 0.9·3 = 3.2 over 4 = 80.0%; actual 0.6 + 2.4 = 3.0 over 4 = 75.0%
    expect(ranked[0]).toMatchObject({ pSollPct: 80, pIstPct: 75, pDeltaPct: -5 });
  });

  it("falls back to upstream's placeholder figures for a model without a row", () => {
    const missing = rankModels([], 'standard', 0).find((m) => m.key === 'glicko2_daneo');
    expect(missing).toMatchObject({ brier: 0.214, ece: 0.015, logLoss: 0.62, accuracy: 64.5, pIstPct: 65, pSollPct: 65 });
  });
});

describe('filters', () => {
  const defaults: AnalysisFilters = {
    engines: new Set(['glicko2_daneo', 'glicko2_mp', 'glicko2_adapt', 'glicko2_std', 'whr']),
    resets: new Set(['continuous']),
    prior: 'standard',
  };

  it("draws upstream's default curves: continuous, standard prior, GlickoD and WHR whatever the prior", () => {
    expect(activeModels(defaults)).toEqual(['glicko2_daneo', 'glicko2_adapt', 'glicko2_mp', 'glicko2_std', 'whr']);
    expect(activeModels({ ...defaults, prior: 'retro' })).toEqual([
      'glicko2_daneo',
      'glicko2_adapt_retro',
      'glicko2_mp_retro',
      'glicko2_std_retro',
      'whr',
    ]);
  });

  it('lists in the table every model the prior allows, active or not', () => {
    const shown = MODELS.filter((m) => shownInTable(m, defaults)).map((m) => m.key);
    expect(shown).toContain('glicko2_std_softer');
    expect(shown).toContain('glicko2_daneo');
    expect(shown).not.toContain('glicko2_std_retro');
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
  it("names models as upstream's agreement cards do", () => {
    expect(agreementName('glicko2_daneo')).toBe('GlickoD');
    expect(agreementName('glicko2_daneo_softer')).toBe('GlickoD (Season Reset)');
    expect(agreementName('glicko2_adapt')).toBe('G2 Adapt (Career)');
    expect(agreementName('whr_softer')).toBe('WHR (Season Reset)');
  });
});
