/** The FAQ page's logic: upstream's tab aliases, and the figures it fills from the current run. */
import type { Agreement, CalibrationRow } from '@ttac/shared/ratings';

/** FAQ_ALIASES are the other names upstream's FAQ accepts in the address for a tab. */
export const FAQ_ALIASES: Record<string, string> = {
  models: 'philosophy',
  multiplayer: 'glicko-mp',
  leaderboard: 'crating',
  daneo: 'gold-standard',
  gold: 'gold-standard',
  'gold-standard': 'gold-standard',
  'calibration-bins': 'resets',
  recalibration: 'recalibration-15',
  'recalibration-15': 'recalibration-15',
  '15-game': 'recalibration-15',
  '15game': 'recalibration-15',
};

/** resolveTab returns the tab an address names, through the aliases, or null for none. */
export function resolveTab(name: string, tabs: readonly string[]): string | null {
  const tab = FAQ_ALIASES[name] ?? name;
  return tabs.includes(tab) ? tab : null;
}

const FIGURE_MODELS = { mp: 'glicko2_mp', std: 'glicko2_std' } as const;

/**
 * faqFigures formats upstream's live FAQ figures from a run's calibration and all-format agreement: walk-forward
 * Brier (4 places) and ECE (percent, 2 places) for MP and Standard in all formats and 4-player, MP's games
 * evaluated, and the players with 15 or more opponents. A figure without data is left out.
 */
export function faqFigures(rows: readonly CalibrationRow[], agreement: Agreement | null): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [short, model] of Object.entries(FIGURE_MODELS)) {
    for (const format of [0, 4] as const) {
      const r = rows.find((x) => x.mode === 'walk_forward' && x.model === model && x.format === format);
      if (r === undefined) continue;
      out[`${short}.${format}.brier`] = r.brier.toFixed(4);
      out[`${short}.${format}.ece`] = (r.ece * 100).toFixed(2);
      if (short === 'mp' && format === 0) out['mp.0.matches'] = r.matches.toLocaleString('en-US');
    }
  }
  if (agreement !== null) out.players = String(agreement.sample);
  return out;
}
