/** A player's tournament achievements page, as upstream's: medal counts, honour roll, accolades and every record. */
import { useEffect, useState } from 'react';
import {
  currentRun,
  player as fetchPlayer,
  playerAchievements,
  type Player,
  type PlayerAchievements,
  type TournamentRecord,
} from '@ttac/shared/ratings';
import { Flag } from './PlayerMarks';
import * as show from '../lib/leaderboard-display';
import {
  careerCells,
  cgeTournamentUrl,
  honorRoll,
  matchesFilter,
  recordCategory,
  resultCell,
  seasonCells,
  sortCareer,
  sortSeasons,
  titleBadge,
  totalTitles,
  type Direction,
} from '../lib/achievements';
import { playerHref } from '../lib/player-view';
import './PlayerPage.css';
import './AchievementsPage.css';
import FilterInput from './FilterInput';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; player: Player; ach: PlayerAchievements };
type Sort = { col: number; dir: Direction } | null;

const SEASON_HEADS = [
  ['Date of Finish', 'Click to sort by Date of Finish'],
  ['Tournament', 'Click to sort by Tournament'],
  ['Season', 'Click to sort by Season'],
  ['Division / Tier', 'Click to sort by Division'],
  ['Result', 'Click to sort by Result'],
];
const CAREER_HEADS = ['Tournament', 'Career Scope', 'Division / Honor', 'Placement / Podium', 'Points / Wins', 'Podium Medal'];

function errorText(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

/** next is upstream's toggle: a column's first click sorts it descending, the date column (already descending) ascending. */
function next(dirs: Record<number, Direction>, col: number): Direction {
  return (dirs[col] ?? 'asc') === 'asc' ? 'desc' : 'asc';
}

function MedalCell({ r }: { r: TournamentRecord }) {
  if (r.medal === null) return <span className="ach-muted">-</span>;
  const text = { gold: '🥇 Gold', silver: '🥈 Silver', bronze: '🥉 Bronze' }[r.medal];
  return <span className={`medal-badge-inline medal-${recordCategory(r.tournament)}-${r.medal}`}>{text}</span>;
}

/** ResultCell shows upstream's result: a medal badge styled by the tournament's category, or the placement. */
function ResultCell({ r }: { r: TournamentRecord }) {
  if (r.medal === null) return <span className="ach-result">{r.placement || '-'}</span>;
  const medal = r.medal.charAt(0).toUpperCase() + r.medal.slice(1);
  return (
    <span
      className={`medal-badge-inline medal-${recordCategory(r.tournament)}-${r.medal}`}
      title={`${r.tournament} ${medal} (${r.placement})`}
    >
      {resultCell(r)}
    </span>
  );
}

/** TournamentName links a record's tournament to its CGE page, where it has one. */
function TournamentName({ r }: { r: TournamentRecord }) {
  const url = cgeTournamentUrl(r.tournament, r.season);
  if (url === null) return <>{r.tournament}</>;
  return (
    <a href={url} target="_blank" rel="noopener noreferrer" className="tournament-cge-link" title="Open official CGE Tournament page">
      {r.tournament} <span className="external-link-icon">↗</span>
    </a>
  );
}

function Header({ player }: { player: Player }) {
  const badge = titleBadge(player.title, player.peak);
  return (
    <div className="achievements-header">
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
      <div className="ach-links">
        <a href={playerHref(player.name)} className="btn-back-profile">
          « Back to Player Profile
        </a>
        <a href="/ratings/" className="home-circle-btn" title="Return to Competitive Leaderboard">
          <img src="/img/home-button-circle.webp" alt="Leaderboard" className="home-btn-img" width={58} height={58} />
        </a>
      </div>
    </div>
  );
}

function Records({ records }: { records: TournamentRecord[] }) {
  const [tab, setTab] = useState<'seasons' | 'career'>('seasons');
  const [filter, setFilter] = useState('');
  const [seasonSort, setSeasonSort] = useState<Sort>(null);
  const [seasonDirs, setSeasonDirs] = useState<Record<number, Direction>>({ 0: 'desc' });
  const [careerSort, setCareerSort] = useState<Sort>(null);
  const [careerDirs, setCareerDirs] = useState<Record<number, Direction>>({});

  // Each sort starts from the order shown, so ties keep the previous sort, as upstream's script sorts the table in place.
  const [shownSeasons, setShownSeasons] = useState(() => records.filter((r) => !r.career));
  const [shownCareers, setShownCareers] = useState(() => records.filter((r) => r.career));

  const sortSeasonsBy = (col: number) => {
    const dir = next(seasonDirs, col);
    setSeasonDirs({ ...seasonDirs, [col]: dir });
    setSeasonSort({ col, dir });
    setShownSeasons(sortSeasons(shownSeasons, col, dir));
  };
  const sortCareerBy = (col: number) => {
    const dir = next(careerDirs, col);
    setCareerDirs({ ...careerDirs, [col]: dir });
    setCareerSort({ col, dir });
    setShownCareers(sortCareer(shownCareers, col, dir));
  };
  const seasonArrow = (col: number) =>
    seasonSort ? (seasonSort.col === col ? (seasonSort.dir === 'asc' ? '▲' : '▼') : '') : col === 0 ? '▼' : '';
  const careerArrow = (col: number) =>
    careerSort ? (careerSort.col === col ? (careerSort.dir === 'asc' ? '▲' : '▼') : '') : col === 0 ? '▲' : '';

  return (
    <div className="rating-card ach-records">
      <div className="ach-records-top">
        <div>
          <h2 className="ach-records-title">Historical Tournament Records</h2>
          <div className="ach-tabs">
            <button type="button" className={`records-tab-btn${tab === 'seasons' ? ' active' : ''}`} onClick={() => setTab('seasons')}>
              🏆 Individual Seasons &amp; Tournaments ({shownSeasons.length})
            </button>
            <button type="button" className={`records-tab-btn${tab === 'career' ? ' active' : ''}`} onClick={() => setTab('career')}>
              🎖️ Career Totals &amp; Hall of Fame ({shownCareers.length})
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

      {tab === 'seasons' ? (
        <div>
          <div className="scroll-hint">
            <span>Swipe table horizontally for full achievement details ➔</span>
          </div>
          <div className="table-responsive">
            <table className="records-table">
              <thead>
                <tr>
                  {SEASON_HEADS.map(([head, hint], col) => (
                    <th key={head} className="ach-sortable" title={hint} onClick={() => sortSeasonsBy(col)}>
                      {head} <span>{seasonArrow(col)}</span>
                    </th>
                  ))}
                  <th>Details</th>
                </tr>
              </thead>
              <tbody>
                {shownSeasons.length === 0 ? (
                  <tr>
                    <td colSpan={6} className="ach-empty">
                      No individual tournament seasons registered.
                    </td>
                  </tr>
                ) : (
                  shownSeasons.map((r, i) => {
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
          </div>
        </div>
      ) : (
        <div>
          <div className="scroll-hint">
            <span>Swipe table horizontally for full career details ➔</span>
          </div>
          <div className="table-responsive">
            <table className="records-table">
              <thead>
                <tr>
                  {CAREER_HEADS.map((head, col) => (
                    <th key={head} className="ach-sortable" onClick={() => sortCareerBy(col)}>
                      {head} <span>{careerArrow(col)}</span>
                    </th>
                  ))}
                  <th>Career Details</th>
                </tr>
              </thead>
              <tbody>
                {shownCareers.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="ach-empty">
                      No career Hall of Fame records found for this player.
                    </td>
                  </tr>
                ) : (
                  shownCareers.map((r, i) => (
                    <tr key={i} hidden={!matchesFilter(careerCells(r), filter)}>
                      <td className="ach-career-tournament">
                        <TournamentName r={r} />
                      </td>
                      <td className="ach-career-scope">{r.season}</td>
                      <td>{r.division}</td>
                      <td className="ach-strong">{r.placement}</td>
                      <td>{r.points || '-'}</td>
                      <td>
                        <MedalCell r={r} />
                      </td>
                      <td className="ach-details">{r.details}</td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

/** AchievementsPage renders the achievements of the player named in the address, from the current run. */
export default function AchievementsPage({ apiUrl }: { apiUrl: string }) {
  const [state, setState] = useState<Load>({ kind: 'loading' });

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
        setState({ kind: 'ready', player, ach });
      })
      .catch((e: unknown) => !stale && setState({ kind: 'error', message: errorText(e) }));
    return () => {
      stale = true;
    };
  }, [apiUrl]);

  if (state.kind === 'loading') return <p className="ach-status">Loading…</p>;
  if (state.kind === 'error') return <p className="ach-status">Could not load this player: {state.message}</p>;
  const { player, ach } = state;
  const s = ach.summary;
  return (
    <div className="achievements-page">
      <Header player={player} />
      <div className="medals-grid">
        {(
          [
            ['🏆', totalTitles(s), 'trophy-accent', 'Total Championships'],
            ['🥇', s?.gold ?? 0, 'gold-accent', 'Gold Medals / 1st'],
            ['🥈', s?.silver ?? 0, 'silver-accent', 'Silver Medals / 2nd'],
            ['🥉', s?.bronze ?? 0, 'bronze-accent', 'Bronze Medals / 3rd'],
          ] as const
        ).map(([icon, count, accent, label]) => (
          <div className="medal-card" key={label}>
            <div className="medal-icon" aria-hidden="true">
              {icon}
            </div>
            <div className={`medal-count ${accent}`}>{count}</div>
            <div className="medal-label">{label}</div>
          </div>
        ))}
      </div>
      {s && (
        <div className="rating-card ach-card">
          <h3 className="ach-card-title">
            <span>🏅 Career Tournament Honor Roll</span>
          </h3>
          <p className="ach-honor-roll">{honorRoll(ach.name, s)}</p>
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
