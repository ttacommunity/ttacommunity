/** A games table's rows, the same for any tournament's games, and how a list of them is paged and counted. */

/** GameSeat is one player in a row: the name shown, the player to link (null for none), a muted detail after the name, the score (null with no game) and how the seat ended. */
export type GameSeat = { label: string; link: string | null; detail: string; score: number | null; note: string };

/** GameRow is one game: its date ('' for none), what it belongs to (a season, a round), where it was played, a state to tag it with, whether it went unplayed, its seats and its replay code. */
export type GameRow = { key: string; date: string; context: string; where: string; tag: string; unplayed: boolean; seats: GameSeat[]; code: string | null; you?: string };

/** GAMES_PER_PAGE is how many games a page shows, as the API pages a tournament's games. */
export const GAMES_PER_PAGE = 50;

/** pageOf gives page number page of rows (kept between the first and the last) and how many pages there are, at least 1. */
export function pageOf<T>(rows: readonly T[], page: number, size = GAMES_PER_PAGE): { rows: T[]; page: number; pages: number } {
  const pages = Math.max(1, Math.ceil(rows.length / size));
  const at = Math.min(Math.max(page, 1), pages);
  return { rows: rows.slice((at - 1) * size, at * size), page: at, pages };
}

/** gamesTag counts a games table's games for its header: all of them and those with a replay code, or only the coded ones. */
export function gamesTag(total: number, coded: number, onlyCoded: boolean): string {
  const n = (x: number) => x.toLocaleString('en-US');
  return onlyCoded ? `${n(total)} games with a replay code` : `${n(total)} games · ${n(coded)} with a replay code`;
}
