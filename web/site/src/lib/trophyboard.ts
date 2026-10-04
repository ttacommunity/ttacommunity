/** A Trophyboard for any tournament, in the rows DANeo's board takes: medals from its season podiums, or a ladder's top-tier wins. */

/** BoardEntry is one row of a Trophyboard: a player's medals, or a ladder's games, wins and win rate. */
export type BoardEntry = {
  name: string;
  gold: number | null;
  silver: number | null;
  bronze: number | null;
  games: number | null;
  wins: number | null;
  win_rate: number | null;
};

/** podiumBoard counts each player's 1st, 2nd and 3rd places over the seasons: most golds first, then silvers, bronzes and name. */
export function podiumBoard(seasons: readonly { podium: readonly { place: number; name: string }[] }[]): BoardEntry[] {
  const medals = new Map<string, [number, number, number]>();
  for (const s of seasons) {
    for (const p of s.podium) {
      if (p.place < 1 || p.place > 3) continue;
      const m = medals.get(p.name) ?? [0, 0, 0];
      m[p.place - 1] += 1;
      medals.set(p.name, m);
    }
  }
  return [...medals]
    .sort(([a, x], [b, y]) => y[0] - x[0] || y[1] - x[1] || y[2] - x[2] || a.localeCompare(b))
    .map(([name, [gold, silver, bronze]]) => ({ name, gold, silver, bronze, games: null, wins: null, win_rate: null }));
}

/** ladderBoard shows a ladder's top-tier wins, most first, as its organisers count them. */
export function ladderBoard(wins: readonly { name: string; wins: number; source?: string }[]): BoardEntry[] {
  return [...wins]
    .sort((a, b) => b.wins - a.wins || a.name.localeCompare(b.name))
    .map((w) => ({ name: w.name, gold: null, silver: null, bronze: null, games: null, wins: w.wins, win_rate: null }));
}
