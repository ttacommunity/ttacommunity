/** The FAQ page's logic: upstream's tab aliases, and the figures it fills from the current run. */
import { count } from '@ttac/shared/numbers';
import type { Activity, CalibrationRow, Run } from '@ttac/shared/ratings';
import { runYears } from './player-view';

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

// The retired models' figures (mp, std_softer) are no longer computed: the FAQ keeps its fallback text for them.
const FIGURE_MODELS = { std: 'glicko2_std', daneo_softer: 'glicko2_daneo_softer' } as const;

/**
 * faqFigures formats upstream's live FAQ figures from a run's calibration and activity: walk-forward
 * Brier (4 places) and ECE (percent, 2 places) for Standard in all formats and 4-player and GlickoD's season
 * reset, the active players, and the run's games, players and years. A figure without data is left out.
 */
export function faqFigures(rows: readonly CalibrationRow[], activity: Activity | null, run?: Run): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [short, model] of Object.entries(FIGURE_MODELS)) {
    for (const format of [0, 4] as const) {
      const r = rows.find((x) => x.mode === 'walk_forward' && x.model === model && x.format === format);
      if (r === undefined) continue;
      out[`${short}.${format}.brier`] = r.brier.toFixed(4);
      out[`${short}.${format}.ece`] = (r.ece * 100).toFixed(2);
    }
  }
  if (activity?.active != null) out.players = String(activity.active);
  if (run?.games != null) out['run.games'] = count(run.games);
  if (run?.players != null) out['run.players'] = count(run.players);
  const years = run ? runYears(run.first_game, run.last_game) : null;
  if (years) out['run.years'] = `${years[0]}–${years[1]}`;
  return out;
}
