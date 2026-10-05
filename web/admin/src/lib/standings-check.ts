/** The standings check as admin shows it: the totals, each kind of difference, and the rows to list. */
import type { components } from '@ttac/shared/api';

export type StandingsCheck = components['schemas']['StandingsCheck'];
type Places = components['schemas']['Places'];
export type Kind = components['schemas']['StandingDifference']['kind'];

/** KINDS are the kinds of difference in the order admin lists them, each with what it means. */
export const KINDS: { kind: Kind; label: string }[] = [
  { kind: 'games', label: 'A different number of games' },
  { kind: 'places', label: 'The same games, other places' },
  { kind: 'not_held', label: 'CGE lists games we do not hold' },
  { kind: 'not_on_cge', label: "We hold games CGE's table lacks" },
  { kind: 'places_after_a_timeout', label: "Other places where a seat timed out (CGE's AI played it on)" },
];

/** DEFAULT_KINDS are listed until admin asks for more: timeouts explain the rest, and they are many. */
export const DEFAULT_KINDS: ReadonlySet<Kind> = new Set<Kind>(['games', 'places', 'not_held', 'not_on_cge']);

/** checkSummary adds up the stages and players checked, the share that agree, and each kind's count. */
export function checkSummary(c: StandingsCheck) {
  const players = c.stages.reduce((n, s) => n + s.players, 0);
  const agree = c.stages.reduce((n, s) => n + s.agree, 0);
  const counts = new Map<Kind, number>();
  for (const s of c.stages) for (const d of s.differences) counts.set(d.kind, (counts.get(d.kind) ?? 0) + 1);
  return {
    stages: c.stages.length,
    onlyOnCGE: c.stages_only_on_cge,
    players,
    agree,
    share: players === 0 ? '' : `${((100 * agree) / players).toFixed(1)}%`,
    byKind: KINDS.map((k) => ({ ...k, count: counts.get(k.kind) ?? 0 })),
  };
}

/** placesText writes a player's places by size of game ("2p: 1 0 · 3p: 0 1 0"), sizes not played left out. */
export function placesText(p: Places): string {
  const parts = (
    [
      ['2p', p.p2],
      ['3p', p.p3],
      ['4p', p.p4],
    ] as const
  ).flatMap(([size, counts]) => (counts === null ? [] : [`${size}: ${counts.join(' ')}`]));
  return parts.length === 0 ? 'no places' : parts.join(' · ');
}

/** differenceRows lists the differences of the kinds shown, by stage in CGE's order, players alphabetically within one. */
export function differenceRows(c: StandingsCheck, shown: ReadonlySet<Kind>) {
  const label = new Map(KINDS.map((k) => [k.kind, k.label]));
  return c.stages.flatMap((s) => {
    const stage = s.name === '' ? `CGE tournament ${s.cge_tournament}, stage ${s.cge_season}` : `${s.name} (${s.cge_tournament}), stage ${s.cge_season}`;
    return s.differences
      .filter((d) => shown.has(d.kind))
      .sort((a, b) => a.player.localeCompare(b.player, undefined, { sensitivity: 'base' }))
      .map((d) => ({
        key: `${s.cge_tournament}/${s.cge_season}/${d.cge_user}`,
        stage,
        player: d.player,
        kind: label.get(d.kind) ?? d.kind,
        cge: `${d.cge_games} games, ${d.cge_points} points · ${placesText(d.cge_places)}`,
        held:
          `${d.held_games} games` +
          (d.held_never_played > 0 ? ` (${d.held_never_played} never played)` : '') +
          ` · ${placesText(d.held_places)}`,
      }));
  });
}
