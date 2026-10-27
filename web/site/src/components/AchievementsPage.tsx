import { useEffect, useMemo, useState } from 'react';
import {
  currentRun,
  player as fetchPlayer,
  playerAchievements,
  type Player,
  type PlayerAchievements,
  type TournamentRecord,
} from '@ttac/shared/ratings';
import { errorText, type Load } from '@ttac/shared/load';
import { Flag } from './PlayerMarks';
import * as show from '../lib/leaderboard-display';
import {
  bestResultsPerTournament,
  canonicalTournamentName,
  careerCells,
  careerResult,
  cgeTournamentUrl,
  formatTournamentDisplay,
  honorRoll,
  isMedal,
  isPromotion,
  isQnD,
  majorBestResults,
  matchesFilter,
  matchesTournamentFilter,
  recordCategory,
  resultCell,
  seasonCells,
  seasonDisplay,
  sortCareer,
  sortSeasons,
  sortTournaments,
  titleBadge,
  tournamentCategory,
  tournamentOverviews,
  CATEGORY_ICON,
  type Direction,
  type TournamentCategoryFilter,
} from '../lib/achievements';
import './PlayerPage.css';
import './AchievementsPage.css';
import BackLink from '@ttac/shared/BackLink';
import { playerHref } from '../lib/player-view';
import FilterInput from './FilterInput';
import ScrollTable from './ScrollTable';
import Waiting from './Waiting';

type Sort = { col: number; dir: Direction } | null;

const SEASON_HEADS = [
  ['Date of Finish', 'Click to sort by Date of Finish'],
  ['Tournament', 'Click to sort by Tournament'],
  ['Season', 'Click to sort by Season'],
  ['Division / Tier', 'Click to sort by Division'],
  ['Result', 'Click to sort by Result'],
];
const TOURNAMENT_HEADS = [
  ['Tournament', 'Click to sort by Tournament'],
  ['Seasons Played', 'Click to sort by Seasons Played'],
  ['Highest Division Result', 'Click to sort by Peak Result'],
  ['Career Record (W - L - D)', 'Click to sort by Record'],
];
const DRILLDOWN_HEADS = [
  ['Date of Finish', 'Click to sort by Date of Finish'],
  ['Season', 'Click to sort by Season'],
  ['Division / Tier', 'Click to sort by Division'],
  ['Result', 'Click to sort by Result'],
];
const CAREER_HEADS = [
  ['Tournament', 'Click to sort by Tournament'],
  ['Career Scope', 'Click to sort by Career Scope'],
  ['Division', 'Click to sort by Division'],
  ['Result', 'Click to sort by Result'],
];
const BEST_HEADS = ['Tournament', 'Best Season', 'Division / Tier', 'Best Result', 'Career Details'];

const CATEGORY_OPTIONS: { id: TournamentCategoryFilter; label: string }[] = [
  { id: 'all', label: 'All Tournaments' },
  { id: 'major', label: 'Major Circuits' },
  { id: 'slam', label: 'Grand Slams & Cups' },
  { id: 'ladder', label: 'Ladders & Leagues' },
  { id: 'community', label: 'Community Events' },
];

function next(dirs: Record<number, Direction>, col: number): Direction {
  return (dirs[col] ?? 'asc') === 'asc' ? 'desc' : 'asc';
}

function ResultCell({ r }: { r: TournamentRecord }) {
  if (r.medal === null) return <span className="ach-result">{r.placement || '-'}</span>;
  const medal = r.medal.charAt(0).toUpperCase() + r.medal.slice(1);
  return (
    <span
      className={`medal-badge-inline medal-${recordCategory(r.tournament)}-${r.medal}`}
      title={`${formatTournamentDisplay(r.tournament)} ${medal} (${r.placement})`}
    >
      {resultCell(r)}
    </span>
  );
}

function TournamentName({ r }: { r: TournamentRecord }) {
  const displayName = formatTournamentDisplay(r.tournament);
  const url = cgeTournamentUrl(r.tournament, r.season);
  if (url === null) return <>{displayName}</>;
  return (
    <a href={url} target="_blank" rel="noopener noreferrer" className="tournament-cge-link" title="Open official CGE Tournament page">
      {displayName} <span className="external-link-icon">↗</span>
    </a>
  );
}

function Header({ player }: { player: Player }) {
  const badge = titleBadge(player.title, player.peak);
  return (
    <div className="achievements-header">
      <div className="header-back">
        <BackLink href={playerHref(player.name)}>Player Profile</BackLink>
      </div>
      <div className="player-identity">
        <div className="ach-identity-name">
          {badge && (
            <span
              className={`badge badge-${badge.code}${badge.code === 'wc' ? ' ach-wc' : ''}${badge.reigning ? ' ach-wc-reigning' : ''}`}
              title={badge.tooltip}
            >
              {badge.text}
            </span>
          )}
          <h1 className="player-profile-name ach-name">{player.name}</h1>
        </div>
        <div className="player-meta-pill">
          <span title="Nationality / Flag">
            <Flag country={player.country} />
          </span>
          {player.peak !== null && (
            <>
              <span className="meta-separator">•</span>
              <span className="peak-meta-wrap">
                Peak Rank:{' '}
                <span className={`badge badge-${player.peak.code.toLowerCase()} ach-peak-badge`}>
                  {show.titleName(player.peak.code)} ({player.peak.year ?? 'Career'})
                </span>
              </span>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

type HighlightMode = 'medals' | 'promotions' | 'best' | 'hof';

function Records({ records }: { records: TournamentRecord[] }) {
  const [tab, setTab] = useState<'seasons' | 'highlights'>('seasons');
  const [filter, setFilter] = useState('');
  const [category, setCategory] = useState<TournamentCategoryFilter>('all');
  const [selectedTournament, setSelectedTournament] = useState<string | null>(null);

  const allTournaments = useMemo(() => tournamentOverviews(records), [records]);
  const seasonRecords = useMemo(() => records.filter((r) => !r.career), [records]);
  const medalRecords = useMemo(() => seasonRecords.filter(isMedal), [seasonRecords]);
  const promotionRecords = useMemo(() => seasonRecords.filter(isPromotion), [seasonRecords]);
  const bestRecords = useMemo(() => bestResultsPerTournament(seasonRecords), [seasonRecords]);
  const hofRecords = useMemo(() => records.filter((r) => r.career && !isQnD(r.tournament)), [records]);

  // Smart default highlight mode based on player's achievements
  const initialHighlight: HighlightMode =
    medalRecords.length > 0 ? 'medals' : promotionRecords.length > 0 ? 'promotions' : 'best';
  const [highlightMode, setHighlightMode] = useState<HighlightMode>(initialHighlight);

  const [tournamentSort, setTournamentSort] = useState<Sort>(null);
  const [tournamentDirs, setTournamentDirs] = useState<Record<number, Direction>>({});
  const [shownTournaments, setShownTournaments] = useState(() => allTournaments);

  const [drilldownSort, setDrilldownSort] = useState<Sort>(null);
  const [drilldownDirs, setDrilldownDirs] = useState<Record<number, Direction>>({ 0: 'desc' });

  const [careerSort, setCareerSort] = useState<Sort>(null);
  const [careerDirs, setCareerDirs] = useState<Record<number, Direction>>({});
  const [shownHof, setShownHof] = useState(() => hofRecords);

  useEffect(() => {
    setShownTournaments(allTournaments);
    setShownHof(hofRecords);
  }, [allTournaments, hofRecords]);

  const selectedOverview = allTournaments.find((t) => t.tournament === selectedTournament) ?? null;
  const selectedTournamentSeasons = useMemo(() => {
    if (!selectedTournament) return [];
    return records.filter((r) => !r.career && canonicalTournamentName(r.tournament) === selectedTournament);
  }, [records, selectedTournament]);

  const [shownDrilldownSeasons, setShownDrilldownSeasons] = useState<TournamentRecord[]>([]);

  useEffect(() => {
    setShownDrilldownSeasons(selectedTournamentSeasons);
    setDrilldownSort(null);
    setDrilldownDirs({ 0: 'desc' });
  }, [selectedTournament, selectedTournamentSeasons]);

  const sortTournamentsBy = (col: number) => {
    const dir = next(tournamentDirs, col);
    setTournamentDirs({ ...tournamentDirs, [col]: dir });
    setTournamentSort({ col, dir });
    setShownTournaments(sortTournaments(shownTournaments, col, dir));
  };

  const sortDrilldownBy = (col: number) => {
    const dir = next(drilldownDirs, col);
    setDrilldownDirs({ ...drilldownDirs, [col]: dir });
    setDrilldownSort({ col, dir });
    const seasonColMap: Record<number, number> = { 0: 0, 1: 2, 2: 3, 3: 4 };
    const targetCol = seasonColMap[col] ?? 0;
    setShownDrilldownSeasons(sortSeasons(shownDrilldownSeasons, targetCol, dir));
  };

  const sortCareerBy = (col: number) => {
    const dir = next(careerDirs, col);
    setCareerDirs({ ...careerDirs, [col]: dir });
    setCareerSort({ col, dir });
    setShownHof(sortCareer(shownHof, col, dir));
  };

  const tournamentArrow = (col: number) =>
    tournamentSort ? (tournamentSort.col === col ? (tournamentSort.dir === 'asc' ? '▲' : '▼') : '') : '';
  const drilldownArrow = (col: number) =>
    drilldownSort ? (drilldownSort.col === col ? (drilldownSort.dir === 'asc' ? '▲' : '▼') : '') : col === 0 ? '▼' : '';
  const careerArrow = (col: number) =>
    careerSort ? (careerSort.col === col ? (careerSort.dir === 'asc' ? '▲' : '▼') : '') : col === 0 ? '▲' : '';

  // Filter records by category
  const activeTournaments =
    category === 'all'
      ? shownTournaments
      : shownTournaments.filter((t) => t.category === category);
  const activeMedals =
    category === 'all' ? medalRecords : medalRecords.filter((r) => tournamentCategory(r.tournament) === category);
  const activePromotions =
    category === 'all'
      ? promotionRecords
      : promotionRecords.filter((r) => tournamentCategory(r.tournament) === category);
  const activeBest =
    category === 'all' ? bestRecords : bestRecords.filter((r) => tournamentCategory(r.tournament) === category);
  const activeHof =
    category === 'all' ? shownHof : shownHof.filter((r) => tournamentCategory(r.tournament) === category);

  const activeHighlights =
    highlightMode === 'medals'
      ? activeMedals
      : highlightMode === 'promotions'
        ? activePromotions
        : highlightMode === 'best'
          ? activeBest
          : activeHof;

  const handleCategoryChange = (cat: TournamentCategoryFilter) => {
    setCategory(cat);
    setSelectedTournament(null);
  };

  const handleTabChange = (newTab: 'seasons' | 'highlights') => {
    setTab(newTab);
    setSelectedTournament(null);
  };

  const selectedCgeUrl = selectedOverview
    ? cgeTournamentUrl(selectedOverview.tournament, selectedOverview.bestRecord.season)
    : null;

  return (
    <div className="rating-card ach-records">
      <div className="ach-records-top">
        <div>
          <h2 className="ach-records-title">Historical Tournament Records</h2>
          <div className="ach-tabs" role="tablist">
            <button
              type="button"
              className={`records-tab-btn${tab === 'seasons' ? ' active' : ''}`}
              onClick={() => handleTabChange('seasons')}
              role="tab"
              aria-selected={tab === 'seasons'}
            >
              🏆 Tournaments ({activeTournaments.length})
            </button>
            <button
              type="button"
              className={`records-tab-btn${tab === 'highlights' ? ' active' : ''}`}
              onClick={() => handleTabChange('highlights')}
              role="tab"
              aria-selected={tab === 'highlights'}
            >
              🎖️ Career Highlights ({activeHighlights.length})
            </button>
          </div>
        </div>
        <FilterInput
          label="Filter tournaments, seasons"
          placeholder="🔍 Filter tournaments, seasons..."
          className="ach-filter"
          value={filter}
          onChange={setFilter}
        />
      </div>

      <div className="ach-category-filters" role="group" aria-label="Tournament Category Filters">
        {CATEGORY_OPTIONS.map((opt) => (
          <button
            key={opt.id}
            type="button"
            className={`ach-category-btn${category === opt.id ? ' active' : ''}`}
            onClick={() => handleCategoryChange(opt.id)}
          >
            {opt.label}
          </button>
        ))}
      </div>

      {tab === 'seasons' ? (
        selectedTournament === null ? (
          <div>
            <ScrollTable hint="click any row or button to view seasons">
              <table className="records-table">
                <thead>
                  <tr>
                    {TOURNAMENT_HEADS.map(([head, hint], col) => (
                      <th key={head} className="ach-sortable" title={hint} onClick={() => sortTournamentsBy(col)}>
                        {head} <span>{tournamentArrow(col)}</span>
                      </th>
                    ))}
                    <th className="ach-action-head">Action</th>
                  </tr>
                </thead>
                <tbody>
                  {activeTournaments.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="ach-empty">
                        No tournaments found for this category.
                      </td>
                    </tr>
                  ) : (
                    activeTournaments.map((t) => {
                      const cgeUrl = cgeTournamentUrl(t.tournament, t.bestRecord.season);
                      return (
                        <tr
                          key={t.tournament}
                          className="ach-row-clickable"
                          onClick={() => setSelectedTournament(t.tournament)}
                          hidden={!matchesTournamentFilter(t, filter)}
                        >
                          <td className="ach-tournament">
                            <div className="ach-tournament-name-wrap">
                              <span className="ach-category-icon" title={t.category}>
                                {CATEGORY_ICON[recordCategory(t.tournament)]}
                              </span>
                              <span className="ach-tournament-title">
                                {formatTournamentDisplay(t.tournament)}
                              </span>
                              {cgeUrl && (
                                <a
                                  href={cgeUrl}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  className="tournament-cge-link"
                                  title="Open official CGE Tournament page"
                                  onClick={(e) => e.stopPropagation()}
                                >
                                  <span className="external-link-icon">↗</span>
                                </a>
                              )}
                            </div>
                          </td>
                          <td className="ach-seasons-cell">
                            <span className="seasons-count-badge">
                              {t.seasonsCount} {t.seasonsCount === 1 ? 'season' : 'seasons'}
                            </span>
                          </td>
                          <td>
                            <div className="ach-best-tier">
                              <span className="ach-division-label">{t.bestRecord.division}</span>
                              <ResultCell r={t.bestRecord} />
                            </div>
                          </td>
                          <td>
                            <div className="ach-record-summary">
                              <span className="record-stat">
                                {t.wins}W - {t.losses}L{t.draws > 0 ? ` - ${t.draws}D` : ''}
                              </span>
                              {t.totalMatches > 0 && (
                                <span className="winrate-pill">{t.winRate.toFixed(1)}%</span>
                              )}
                            </div>
                          </td>
                          <td className="ach-action-cell">
                            <button
                              type="button"
                              className="ach-view-btn"
                              title="View all seasons of this tournament"
                              onClick={(e) => {
                                e.stopPropagation();
                                setSelectedTournament(t.tournament);
                              }}
                            >
                              Explore ➔
                            </button>
                          </td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </ScrollTable>
          </div>
        ) : (
          <div className="ach-drilldown-view">
            <div className="ach-drilldown-header">
              <button
                type="button"
                className="ach-back-btn"
                onClick={() => setSelectedTournament(null)}
              >
                ← Back to all tournaments
              </button>
              {selectedOverview && (
                <div className="ach-drilldown-info">
                  <div className="ach-drilldown-title">
                    <span className="ach-category-icon" title={selectedOverview.category}>
                      {CATEGORY_ICON[recordCategory(selectedOverview.tournament)]}
                    </span>
                    <h3 className="ach-drilldown-name">
                      {formatTournamentDisplay(selectedOverview.tournament)}
                    </h3>
                    {selectedCgeUrl && (
                      <a
                        href={selectedCgeUrl}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="tournament-cge-link"
                        title="Open official CGE Tournament page"
                      >
                        <span className="external-link-icon">↗</span>
                      </a>
                    )}
                  </div>
                  <div className="ach-drilldown-summary">
                    <span className="seasons-count-badge">
                      {selectedOverview.seasonsCount} {selectedOverview.seasonsCount === 1 ? 'edition' : 'editions'}
                    </span>
                    <span className="record-stat">
                      {selectedOverview.totalMatches} matches: {selectedOverview.wins}W - {selectedOverview.losses}L
                      {selectedOverview.draws > 0 ? ` - ${selectedOverview.draws}D` : ''} ({selectedOverview.winRate.toFixed(1)}% win rate)
                    </span>
                  </div>
                </div>
              )}
            </div>

            <ScrollTable hint="full season details">
              <table className="records-table">
                <thead>
                  <tr>
                    {DRILLDOWN_HEADS.map(([head, hint], col) => (
                      <th key={head} className="ach-sortable" title={hint} onClick={() => sortDrilldownBy(col)}>
                        {head} <span>{drilldownArrow(col)}</span>
                      </th>
                    ))}
                    <th>Details</th>
                  </tr>
                </thead>
                <tbody>
                  {shownDrilldownSeasons.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="ach-empty">
                        No seasons found.
                      </td>
                    </tr>
                  ) : (
                    shownDrilldownSeasons.map((r, i) => {
                      const cells = seasonCells(r);
                      return (
                        <tr key={i} hidden={!matchesFilter(cells, filter)}>
                          <td className="ach-date">{cells[0]}</td>
                          <td className="ach-strong">{seasonDisplay(r)}</td>
                          <td>{r.division}</td>
                          <td>
                            <ResultCell r={r} />
                          </td>
                          <td className="ach-details">{r.details}</td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </ScrollTable>
          </div>
        )
      ) : (
        <div>
          <div className="ach-sub-tabs" role="tablist" aria-label="Career Highlights View Mode">
            <button
              type="button"
              className={`ach-sub-btn${highlightMode === 'medals' ? ' active' : ''}`}
              onClick={() => setHighlightMode('medals')}
              role="tab"
              aria-selected={highlightMode === 'medals'}
            >
              🥇 Major Medals ({activeMedals.length})
            </button>
            <button
              type="button"
              className={`ach-sub-btn${highlightMode === 'promotions' ? ' active' : ''}`}
              onClick={() => setHighlightMode('promotions')}
              role="tab"
              aria-selected={highlightMode === 'promotions'}
            >
              🚀 Promotions &amp; Group Wins ({activePromotions.length})
            </button>
            <button
              type="button"
              className={`ach-sub-btn${highlightMode === 'best' ? ' active' : ''}`}
              onClick={() => setHighlightMode('best')}
              role="tab"
              aria-selected={highlightMode === 'best'}
            >
              ⭐ Best per Tournament ({activeBest.length})
            </button>
            <button
              type="button"
              className={`ach-sub-btn${highlightMode === 'hof' ? ' active' : ''}`}
              onClick={() => setHighlightMode('hof')}
              role="tab"
              aria-selected={highlightMode === 'hof'}
            >
              📜 All-Time Career Totals ({activeHof.length})
            </button>
          </div>

          <ScrollTable hint="full highlight details">
            {highlightMode === 'hof' ? (
              <table className="records-table">
                <thead>
                  <tr>
                    {CAREER_HEADS.map(([head, hint], col) => (
                      <th key={head} className="ach-sortable" title={hint} onClick={() => sortCareerBy(col)}>
                        {head} <span>{careerArrow(col)}</span>
                      </th>
                    ))}
                    <th>Career Details</th>
                  </tr>
                </thead>
                <tbody>
                  {activeHof.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="ach-empty">
                        No career totals found for this category or filter.
                      </td>
                    </tr>
                  ) : (
                    activeHof.map((r, i) => (
                      <tr key={i} hidden={!matchesFilter(careerCells(r), filter)}>
                        <td className="ach-career-tournament">
                          <TournamentName r={r} />
                        </td>
                        <td className="ach-career-scope">{r.season}</td>
                        <td>{r.division}</td>
                        <td className="ach-strong">{careerResult(r)}</td>
                        <td className="ach-details">{r.details}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            ) : highlightMode === 'promotions' ? (
              <table className="records-table">
                <thead>
                  <tr>
                    <th>Date of Finish</th>
                    <th>Tournament</th>
                    <th>Season</th>
                    <th>Division / Tier</th>
                    <th>Status</th>
                    <th>Details</th>
                  </tr>
                </thead>
                <tbody>
                  {activePromotions.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="ach-empty">
                        No division promotions or lower-tier group wins registered for this category.
                      </td>
                    </tr>
                  ) : (
                    activePromotions.map((r, i) => {
                      const cells = seasonCells(r);
                      return (
                        <tr key={i} hidden={!matchesFilter(cells, filter)}>
                          <td className="ach-date">{cells[0]}</td>
                          <td className="ach-tournament">
                            <TournamentName r={r} />
                          </td>
                          <td className="ach-strong">{r.season}</td>
                          <td>{r.division}</td>
                          <td>
                            <span className="promotion-badge">🚀 Promoted ({r.placement})</span>
                          </td>
                          <td className="ach-details">{r.details}</td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            ) : highlightMode === 'best' ? (
              <table className="records-table">
                <thead>
                  <tr>
                    {BEST_HEADS.map((head) => (
                      <th key={head}>{head}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {activeBest.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="ach-empty">
                        No tournament records found for this category.
                      </td>
                    </tr>
                  ) : (
                    activeBest.map((r, i) => {
                      const cells = seasonCells(r);
                      return (
                        <tr key={i} hidden={!matchesFilter(cells, filter)}>
                          <td className="ach-tournament">
                            <TournamentName r={r} />
                          </td>
                          <td className="ach-strong">{r.season}</td>
                          <td>{r.division}</td>
                          <td>
                            <ResultCell r={r} />
                          </td>
                          <td className="ach-details">{r.details}</td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            ) : (
              <table className="records-table">
                <thead>
                  <tr>
                    {SEASON_HEADS.map(([head]) => (
                      <th key={head}>{head}</th>
                    ))}
                    <th>Details</th>
                  </tr>
                </thead>
                <tbody>
                  {activeMedals.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="ach-empty">
                        No major medal finishes recorded for this category. Check out Promotions &amp; Group Wins or Best per Tournament!
                      </td>
                    </tr>
                  ) : (
                    activeMedals.map((r, i) => {
                      const cells = seasonCells(r);
                      return (
                        <tr key={i} hidden={!matchesFilter(cells, filter)}>
                          <td className="ach-date">{cells[0]}</td>
                          <td className="ach-tournament">
                            <TournamentName r={r} />
                          </td>
                          <td className="ach-strong">{r.season}</td>
                          <td>{r.division}</td>
                          <td>
                            <ResultCell r={r} />
                          </td>
                          <td className="ach-details">{r.details}</td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            )}
          </ScrollTable>
        </div>
      )}
    </div>
  );
}

/** AchievementsPage renders the achievements of the player named in the address, from the current run. */
export default function AchievementsPage({ apiUrl }: { apiUrl: string }) {
  const [state, setState] = useState<Load<{ player: Player; ach: PlayerAchievements }>>({ kind: 'loading' });

  useEffect(() => {
    const name = new URLSearchParams(window.location.search).get('name') ?? '';
    if (name === '') {
      setState({ kind: 'error', message: 'No player named in the address.' });
      return;
    }
    let stale = false;
    currentRun(apiUrl)
      .then((run) => Promise.all([fetchPlayer(apiUrl, run.run_id, name), playerAchievements(apiUrl, run.run_id, name)]))
      .then(([player, ach]) => {
        if (stale) return;
        document.title = `${player.name} - Tournament Achievements & Records | Through the Ages`;
        setState({ kind: 'ready', data: { player, ach } });
      })
      .catch((e: unknown) => !stale && setState({ kind: 'error', message: errorText(e) }));
    return () => {
      stale = true;
    };
  }, [apiUrl]);

  if (state.kind !== 'ready') return <Waiting load={state} what="this player" />;
  const { player, ach } = state.data;
  const s = ach.summary;
  const totalMedals = (s?.gold ?? 0) + (s?.silver ?? 0) + (s?.bronze ?? 0);
  const best = majorBestResults(ach.records);

  return (
    <div className="achievements-page">
      <Header player={player} />
      {totalMedals > 0 ? (
        <div className="medals-grid">
          {s?.world_titles && s.world_titles > 0 ? (
            <div className="medal-card" title="World Championship titles in Through the Ages">
              <div className="medal-icon" aria-hidden="true">
                👑
              </div>
              <div className="medal-count trophy-accent">{s.world_titles}</div>
              <div className="medal-label">World Championships</div>
            </div>
          ) : null}
          <div className="medal-card" title="1st place gold medals in major circuits (World Championship, International Championship, Intermezzo, Royal League Emperor)">
            <div className="medal-icon" aria-hidden="true">
              🥇
            </div>
            <div className="medal-count gold-accent">{s?.gold ?? 0}</div>
            <div className="medal-label">Major Gold Medals</div>
          </div>
          <div className="medal-card" title="2nd place silver medals in major circuits (World Championship, International Championship, Intermezzo, Royal League Emperor)">
            <div className="medal-icon" aria-hidden="true">
              🥈
            </div>
            <div className="medal-count silver-accent">{s?.silver ?? 0}</div>
            <div className="medal-label">Major Silver Medals</div>
          </div>
          <div className="medal-card" title="3rd place bronze medals in major circuits (World Championship, International Championship, Intermezzo, Royal League Emperor)">
            <div className="medal-icon" aria-hidden="true">
              🥉
            </div>
            <div className="medal-count bronze-accent">{s?.bronze ?? 0}</div>
            <div className="medal-label">Major Bronze Medals</div>
          </div>
        </div>
      ) : (
        <div className="best-results-grid">
          <div className="best-result-card" title="Highest division reached in Royal League">
            <div className="best-result-icon" aria-hidden="true">
              🏰
            </div>
            <div className="best-result-value">{best.rl ? best.rl.division : '—'}</div>
            <div className="best-result-sub">{best.rl ? `Best: ${best.rl.placement} (${best.rl.season})` : 'Not entered yet'}</div>
            <div className="best-result-label">Royal League Peak</div>
          </div>
          <div className="best-result-card" title="Highest tier reached in International Championship (4P)">
            <div className="best-result-icon" aria-hidden="true">
              🌍
            </div>
            <div className="best-result-value">{best.ic ? best.ic.division : '—'}</div>
            <div className="best-result-sub">{best.ic ? `Best: ${best.ic.placement} (${best.ic.season})` : 'Not entered yet'}</div>
            <div className="best-result-label">International Peak</div>
          </div>
          <div className="best-result-card" title="Highest tier reached in Intermezzo (3P)">
            <div className="best-result-icon" aria-hidden="true">
              ⚔️
            </div>
            <div className="best-result-value">{best.im ? best.im.division : '—'}</div>
            <div className="best-result-sub">{best.im ? `Best: ${best.im.placement} (${best.im.season})` : 'Not entered yet'}</div>
            <div className="best-result-label">Intermezzo Peak</div>
          </div>
          <div className="best-result-card" title="Career competitive tournament matches and victory rate">
            <div className="best-result-icon" aria-hidden="true">
              📊
            </div>
            <div className="best-result-value">{best.totalMatches > 0 ? `${best.totalMatches} Matches` : '—'}</div>
            <div className="best-result-sub">{best.totalMatches > 0 ? `${best.winRate.toFixed(1)}% Victory Rate` : 'No matches on record'}</div>
            <div className="best-result-label">Tournament Record</div>
          </div>
        </div>
      )}

      {s && (
        <div className="rating-card ach-card">
          <h3 className="ach-card-title">
            <span>🏅 Career Tournament Honor Roll</span>
          </h3>
          <p className="ach-honor-roll">
            {totalMedals > 0
              ? honorRoll(ach.name, s)
              : `${ach.name} has competed in ${ach.records.filter((r) => !r.career).length} tournament seasons with ${best.totalMatches} competitive matches played (${best.winRate.toFixed(1)}% win rate).`}
          </p>
        </div>
      )}
      {s && s.accolades.length > 0 && (
        <div className="rating-card ach-card">
          <h3 className="ach-card-title ach-accolades-title">Featured Career Accolades</h3>
          <div className="career-honors-grid">
            {s.accolades.map((a) => (
              <div className="career-honor-badge" key={a}>
                <span className="honor-star">★</span>
                <span className="honor-text">{a}</span>
              </div>
            ))}
          </div>
        </div>
      )}
      <Records records={ach.records} />
    </div>
  );
}
