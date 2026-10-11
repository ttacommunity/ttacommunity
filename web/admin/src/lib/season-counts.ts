/** A season's stages as admin lists them, with their finished games and players, and the season's summary line. */
type Stage = { label: string; number: number; finished: number };

/** stageRows returns the stages with finished games, the highest number first (by name where numbers tie). */
export function stageRows<T extends Stage>(stages: readonly T[]): T[] {
  return stages
    .filter((st) => st.finished > 0)
    .sort((a, b) => b.number - a.number || b.label.localeCompare(a.label, undefined, { numeric: true }));
}

/** seasonSummary returns a season's stages with finished games, its finished games (a season without stages has them
 * in an unnamed stage, which stages leaves out), and its players. */
export function seasonSummary(season: { players: number; finished: number; stages: readonly Stage[] }): { stages: number; games: number; players: number } {
  return { stages: stageRows(season.stages).length, games: season.finished, players: season.players };
}
