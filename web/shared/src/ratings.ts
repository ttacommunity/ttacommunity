/** A client for the public ratings API. Types come from the generated spec. */
import type { components } from '../api/schema';

export type Run = components['schemas']['Run'];
export type Model = components['schemas']['Model'];
export type Format = components['schemas']['Format'];
export type Leaderboard = components['schemas']['Leaderboard'];
export type LeaderboardRow = components['schemas']['LeaderboardRow'];
export type History = components['schemas']['History'];
export type TitleCode = NonNullable<components['schemas']['Title']>['code'];
export type Player = components['schemas']['Player'];
export type PlayerRating = components['schemas']['PlayerRating'];
export type PlayerHistory = components['schemas']['PlayerHistory'];
export type PlayerMatches = components['schemas']['PlayerMatches'];
export type PlayerMatch = components['schemas']['PlayerMatch'];
export type Rival = components['schemas']['Rival'];
export type PlayerRivals = components['schemas']['PlayerRivals'];
export type PlayerCareer = components['schemas']['PlayerCareer'];
export type CareerYear = components['schemas']['CareerYear'];
export type ModelPeak = components['schemas']['ModelPeak'];
export type PlayerVersus = components['schemas']['PlayerVersus'];
export type VersusMatch = components['schemas']['VersusMatch'];
export type PlayerMatrix = components['schemas']['PlayerMatrix'];
export type MatrixCell = components['schemas']['MatrixCell'];
export type Years = components['schemas']['Years'];
export type YearEntry = components['schemas']['YearEntry'];
export type YearLeaderboard = components['schemas']['YearLeaderboard'];
export type YearRow = components['schemas']['YearRow'];
export type Calibration = components['schemas']['Calibration'];
export type CalibrationRow = components['schemas']['CalibrationRow'];
export type Distribution = components['schemas']['Distribution'];
export type Agreement = components['schemas']['Agreement'];
export type ModelPair = components['schemas']['ModelPair'];
export type ComparedPlayers = components['schemas']['ComparedPlayers'];
export type ComparedPlayer = components['schemas']['ComparedPlayer'];
export type Activity = components['schemas']['Activity'];
export type CountryRatings = components['schemas']['CountryRatings'];
export type Series = components['schemas']['Series'];
export type TournamentSeries = components['schemas']['TournamentSeries'];
export type TrophyRow = components['schemas']['TrophyRow'];
export type SeasonRecord = components['schemas']['SeasonRecord'];
export type GameRecord = components['schemas']['GameRecord'];
export type HallOfFame = components['schemas']['HallOfFame'];
export type SeriesCount = components['schemas']['SeriesCount'];
export type Legend = components['schemas']['Legend'];
export type PlayerAchievements = components['schemas']['PlayerAchievements'];
export type AchievementSummary = NonNullable<components['schemas']['AchievementSummary']>;
export type TournamentRecord = components['schemas']['TournamentRecord'];
export type CommunityEdition = components['schemas']['CommunityEdition'];
export type CommunityLeaderboard = components['schemas']['CommunityLeaderboard'];
export type CommunityRow = components['schemas']['CommunityRow'];
export type Catalogue = components['schemas']['Catalogue'];
export type CatalogueEntry = components['schemas']['CatalogueEntry'];
export type CatalogueTournament = components['schemas']['CatalogueTournament'];
export type CatalogueSeason = components['schemas']['CatalogueSeason'];
export type CatalogueStage = components['schemas']['CatalogueStage'];
export type CatalogueGames = components['schemas']['CatalogueGames'];
export type LadderWin = components['schemas']['LadderWin'];
export type CatalogueGame = components['schemas']['CatalogueGame'];
export type GameSeat = components['schemas']['GameSeat'];
export type PodiumPlace = components['schemas']['PodiumPlace'];
export type CGETournament = components['schemas']['CGETournament'];

/** An error response from the API, carrying its status and message. */
export class RatingsApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = 'RatingsApiError';
  }
}

/** Returns the run the site shows now. */
export async function currentRun(base: string, fetchFn: typeof fetch = fetch): Promise<Run> {
  return get<Run>(fetchFn, base, '/v1/runs/current');
}

/** Returns every rated player in one published run, model and format, in rank order. */
export async function leaderboard(
  base: string,
  run: number,
  model: Model,
  format: Format,
  fetchFn: typeof fetch = fetch,
): Promise<Leaderboard> {
  const query = new URLSearchParams({ model, format: String(format) });
  return get<Leaderboard>(fetchFn, base, `/v1/runs/${run}/leaderboard?${query}`);
}

/** Returns each player's rating at a past date, or at the run's previous period when date is null. */
export async function history(
  base: string,
  run: number,
  model: Model,
  format: Format,
  date: string | null,
  fetchFn: typeof fetch = fetch,
): Promise<History> {
  const query = new URLSearchParams({ model, format: String(format) });
  if (date !== null) query.set('at', date);
  return get<History>(fetchFn, base, `/v1/runs/${run}/history?${query}`);
}

function playerPath(run: number, name: string, rest = ''): string {
  return `/v1/runs/${run}/players/${encodeURIComponent(name)}${rest}`;
}

/** Returns one player's details and ratings in every page model and format. */
export async function player(base: string, run: number, name: string, fetchFn: typeof fetch = fetch): Promise<Player> {
  return get<Player>(fetchFn, base, playerPath(run, name));
}

/** Returns one player's rating after every period, by model, in one format. */
export async function playerHistory(
  base: string,
  run: number,
  name: string,
  format: Format,
  fetchFn: typeof fetch = fetch,
): Promise<PlayerHistory> {
  return get<PlayerHistory>(fetchFn, base, playerPath(run, name, `/history?format=${format}`));
}

/** Returns one page of a player's games in a format, with each game's estimated rating change in model. */
export async function playerMatches(
  base: string,
  run: number,
  name: string,
  format: Format,
  model: Model,
  page: number,
  fetchFn: typeof fetch = fetch,
): Promise<PlayerMatches> {
  const query = new URLSearchParams({ format: String(format), model, page: String(page) });
  return get<PlayerMatches>(fetchFn, base, playerPath(run, name, `/matches?${query}`));
}

/** Returns a player's record against the opponents they met most, in one format. */
export async function playerRivals(
  base: string,
  run: number,
  name: string,
  format: Format,
  fetchFn: typeof fetch = fetch,
): Promise<PlayerRivals> {
  return get<PlayerRivals>(fetchFn, base, playerPath(run, name, `/rivals?format=${format}`));
}

/** Returns a player's record each year and each continuous model's peak, in one format. */
export async function playerCareer(
  base: string,
  run: number,
  name: string,
  format: Format,
  fetchFn: typeof fetch = fetch,
): Promise<PlayerCareer> {
  return get<PlayerCareer>(fetchFn, base, playerPath(run, name, `/career?format=${format}`));
}

/** Returns the latest games two players played together, with where each finished. */
export async function playerVersus(
  base: string,
  run: number,
  name: string,
  other: string,
  fetchFn: typeof fetch = fetch,
): Promise<PlayerVersus> {
  return get<PlayerVersus>(fetchFn, base, playerPath(run, name, `/versus/${encodeURIComponent(other)}`));
}

/** Returns a player's rating in each page model for a format: the run's, or at the end of year. */
export async function playerMatrix(
  base: string,
  run: number,
  name: string,
  format: Format,
  year: number | null,
  fetchFn: typeof fetch = fetch,
): Promise<PlayerMatrix> {
  const query = new URLSearchParams({ format: String(format) });
  if (year !== null) query.set('year', String(year));
  return get<PlayerMatrix>(fetchFn, base, playerPath(run, name, `/matrix?${query}`));
}

/** Returns the years a run's ratings cover, newest first, and which have a World Champion. */
export async function years(base: string, run: number, fetchFn: typeof fetch = fetch): Promise<Years> {
  return get<Years>(fetchFn, base, `/v1/runs/${run}/years`);
}

/** Returns a run's ratings at the end of a year for a model and format, as upstream's season archive. */
export async function yearLeaderboard(
  base: string,
  run: number,
  year: number,
  model: Model,
  format: Format,
  fetchFn: typeof fetch = fetch,
): Promise<YearLeaderboard> {
  const query = new URLSearchParams({ model, format: String(format) });
  return get<YearLeaderboard>(fetchFn, base, `/v1/runs/${run}/years/${year}/leaderboard?${query}`);
}

/** Returns every model's calibration in a run, for both evaluation modes and all formats. */
export async function calibration(base: string, run: number, fetchFn: typeof fetch = fetch): Promise<Calibration> {
  return get<Calibration>(fetchFn, base, `/v1/runs/${run}/analysis/calibration`);
}

/** Returns how a run's players spread over rating bins in each engine, for a format. */
export async function distribution(base: string, run: number, format: Format, fetchFn: typeof fetch = fetch): Promise<Distribution> {
  return get<Distribution>(fetchFn, base, `/v1/runs/${run}/analysis/distribution?format=${format}`);
}

/** Returns the model pairs whose player rankings agree most and least, for a format. */
export async function agreement(base: string, run: number, format: Format, fetchFn: typeof fetch = fetch): Promise<Agreement> {
  return get<Agreement>(fetchFn, base, `/v1/runs/${run}/analysis/agreement?format=${format}`);
}

/** Returns players matching a search (names or country codes, comma-separated), or upstream's defaults, across every model. */
export async function comparedPlayers(base: string, run: number, search: string, format: Format, fetchFn: typeof fetch = fetch): Promise<ComparedPlayers> {
  const query = new URLSearchParams({ format: String(format) });
  if (search !== '') query.set('search', search);
  return get<ComparedPlayers>(fetchFn, base, `/v1/runs/${run}/analysis/players?${query}`);
}

/** Returns a run's active players, games per format, titles and leading countries. */
export async function activity(base: string, run: number, fetchFn: typeof fetch = fetch): Promise<Activity> {
  return get<Activity>(fetchFn, base, `/v1/runs/${run}/analysis/activity`);
}

/** Returns what a tournament series' Hall of Fame page shows: counts, trophyboard, best seasons and games. */
export async function tournamentSeries(base: string, run: number, series: Series, fetchFn: typeof fetch = fetch): Promise<TournamentSeries> {
  return get<TournamentSeries>(fetchFn, base, `/v1/runs/${run}/tournaments/${series}`);
}

/** Returns the Hall of Fame overview: every series' counts, the all-time table, and the tops of the cups and ladders boards. */
export async function hallOfFame(base: string, run: number, fetchFn: typeof fetch = fetch): Promise<HallOfFame> {
  return get<HallOfFame>(fetchFn, base, `/v1/runs/${run}/tournaments`);
}

/** Returns every tournament with games: its seasons, games, players and first and last game. */
export async function catalogue(base: string, run: number, fetchFn: typeof fetch = fetch): Promise<Catalogue> {
  return get<Catalogue>(fetchFn, base, `/v1/runs/${run}/catalogue`);
}

/** Returns one tournament's seasons in order, each with its stages and recorded podium. */
export async function catalogueTournament(base: string, run: number, slug: string, fetchFn: typeof fetch = fetch): Promise<CatalogueTournament> {
  return get<CatalogueTournament>(fetchFn, base, `/v1/runs/${run}/catalogue/${encodeURIComponent(slug)}`);
}

/** Which of a tournament's games to ask for: empty filters do not narrow, and coded keeps games with a replay code. */
export interface GamesFilter {
  season?: string;
  stage?: string;
  group?: string;
  coded?: boolean;
  page: number;
}

/** Returns one page of a tournament's games, newest first, narrowed by the filter. */
export async function catalogueGames(base: string, run: number, slug: string, filter: GamesFilter, fetchFn: typeof fetch = fetch): Promise<CatalogueGames> {
  const query = new URLSearchParams({ page: String(filter.page) });
  for (const key of ['season', 'stage', 'group'] as const) {
    const value = filter[key];
    if (value !== undefined && value !== '') query.set(key, value);
  }
  if (filter.coded === true) query.set('coded', 'true');
  return get<CatalogueGames>(fetchFn, base, `/v1/runs/${run}/catalogue/${encodeURIComponent(slug)}/games?${query}`);
}

// Public routes take no credentials, so none are ever sent.
async function get<T>(fetchFn: typeof fetch, base: string, path: string): Promise<T> {
  const res = await fetchFn(base.replace(/\/+$/, '') + path, { credentials: 'omit' });
  if (!res.ok) {
    const body: unknown = await res.json().catch(() => null);
    const message =
      body !== null && typeof body === 'object' && 'error' in body && typeof body.error === 'string'
        ? body.error
        : `request failed (${res.status})`;
    throw new RatingsApiError(res.status, message);
  }
  return (await res.json()) as T;
}

/** Returns a player's tournament medal summary and every record their achievements page lists. */
export async function playerAchievements(
  base: string,
  run: number,
  name: string,
  fetchFn: typeof fetch = fetch,
): Promise<PlayerAchievements> {
  return get<PlayerAchievements>(fetchFn, base, playerPath(run, name, '/achievements'));
}

/** Returns an edition of the community leaderboard, best rank first. */
export async function communityLeaderboard(
  base: string,
  run: number,
  edition: CommunityEdition,
  fetchFn: typeof fetch = fetch,
): Promise<CommunityLeaderboard> {
  return get<CommunityLeaderboard>(fetchFn, base, `/v1/runs/${run}/community/${edition}`);
}
