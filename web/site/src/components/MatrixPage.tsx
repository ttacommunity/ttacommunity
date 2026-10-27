/** A player's career years and historical performance page: every reset mode, and the year-by-year career. */
import { useCallback, useEffect, useState, type ReactNode } from 'react';
import {
  currentRun,
  player as fetchPlayer,
  playerCareer,
  type Player,
  type PlayerCareer,
  type TitleCode,
} from '@ttac/shared/ratings';
import BackLink from '@ttac/shared/BackLink';
import { Flag } from './PlayerMarks';
import * as show from '../lib/leaderboard-display';
import { DEFAULT_VIEW } from '../lib/leaderboard-view';
import { careerPeakYear, scoreYears, wcLabel } from '../lib/career';
import { matrixParams, pageModel, parseMatrixView, profileHref, type MatrixView } from '../lib/player-view';
import { achievementsHref } from '../lib/achievements';
import { recall, remember } from '../lib/remembered';
import './PlayerPage.css';
import './MatrixPage.css';
import ScrollTable from './ScrollTable';
import ViewSelectors from './ViewSelectors';
import Waiting from './Waiting';
import { useLoad } from './useLoad';
import { viewLinks } from '../lib/navigate';
import { errorText, type Load } from '@ttac/shared/load';

function readView(): MatrixView {
  return parseMatrixView(new URLSearchParams(window.location.search), recall(window.localStorage));
}

/** TitleBadge is a year's title as upstream shows it: in the peak card World Champion gets the 💎 badge. */
function TitleBadge({ code, card }: { code: TitleCode; card?: boolean }) {
  if (code === 'WC' && card) return <span className="badge-wc badge-small">💎 World Champion</span>;
  if (code === 'SGM') return <span className="badge badge-small badge-sgm">👑 GM</span>;
  return <span className={`badge badge-small badge-${code.toLowerCase()}`}>{show.titleName(code)}</span>;
}

/** Shows the player's career years and peak season evaluation. */
export default function MatrixPage({ apiUrl }: { apiUrl: string }) {
  const [view, setView] = useState<MatrixView>(readView);
  const [run, setRun] = useState<Load<number>>({ kind: 'loading' });

  const go = useCallback((next: MatrixView) => {
    window.history.pushState(null, '', `?${matrixParams(next)}`);
    remember(window.localStorage, { ...DEFAULT_VIEW, ...recall(window.localStorage), reset: next.reset, format: next.format });
    setView(next);
  }, []);
  const linkTo = viewLinks(matrixParams, go);

  useEffect(() => {
    const onPop = () => setView(readView());
    window.addEventListener('popstate', onPop);
    currentRun(apiUrl)
      .then((r) => setRun({ kind: 'ready', data: r.run_id }))
      .catch((e: unknown) => setRun({ kind: 'error', message: errorText(e) }));
    return () => window.removeEventListener('popstate', onPop);
  }, [apiUrl]);

  const runId = run.kind === 'ready' && view.name !== '' ? run.data : null;
  const { name, format } = view;
  const model = pageModel(view.engine, view.reset);
  const profile = useLoad<Player>(`${runId}|${name}`, runId === null ? null : () => fetchPlayer(apiUrl, runId, name));
  const career = useLoad<PlayerCareer>(
    `${runId}|${name}|${format}|${model}`,
    runId === null ? null : () => playerCareer(apiUrl, runId, name, format, model),
  );

  useEffect(() => {
    document.title = `${name || 'Player'} - Career Years | Through the Ages`;
  }, [name]);

  if (name === '') return <Message>No player was named. Choose one from the leaderboard.</Message>;
  if (run.kind === 'error') return <Waiting load={run} what="the ratings" />;
  if (profile.kind !== 'ready') return <Waiting load={profile} what="this player" />;

  const p = profile.data;
  const years = career.kind === 'ready' ? career.data.years : [];
  const best = careerPeakYear(years, p.peak);
  const formatName = show.formatName(format);

  return (
    <>
      <Header player={p} view={view} />

      <ViewSelectors
        reset={view.reset}
        format={format}
        linkReset={(reset) => linkTo({ ...view, reset })}
        linkFormat={(f) => linkTo({ ...view, format: f })}
      />

      {best !== null && (
        <div className="peak-year-card">
          <div className="peak-year-badge" aria-hidden="true">
            👑
          </div>
          <div className="peak-year-body">
            <div className="peak-year-kicker">Career Benchmark Evaluation</div>
            <div className="peak-year-title">
              Peak Career Season: <span className="peak-year-year">{best.year}</span>
              {best.title !== null && <TitleBadge code={best.title} card />}
            </div>
            <div className="peak-year-text">
              In {best.year}, <strong>{p.name}</strong> achieved a dominant <strong>{best.win_rate.toFixed(1)}% win rate</strong> across{' '}
              <strong>{best.opponents} competitive opponents</strong> ({best.wins} wins, {best.losses} losses)
              {best.peak_rating ? (
                <>
                  {' '}
                  with a seasonal peak rating of <strong className="peak-year-rating">{best.peak_rating.toFixed(1)}</strong>
                </>
              ) : null}
              {best.title !== null && (
                <>
                  {' '}
                  competing in the <strong>{show.titleName(best.title)}</strong> division
                </>
              )}{' '}
              (Composite Performance Index: <strong className="peak-year-score">{best.score.toFixed(1)} pts</strong>).
            </div>
          </div>
        </div>
      )}

      <div className="rating-card matrix-card">
        <div className="matrix-card-head">
          <div>
            <h3>📅 Year-by-Year Career Breakdown</h3>
            <span className="matrix-card-sub">
              {years.length} Seasons Recorded • Format: {formatName}
            </span>
          </div>
          <span className="matrix-card-note">
            Hover on <strong>ℹ️ Score Index</strong> for composite formula details
          </span>
        </div>
        <ScrollTable hint="full career history">
          <table className="career-history-table">
            <caption className="visually-hidden">Career by year, {formatName}</caption>
            <thead>
              <tr>
                <th>Year</th>
                <th>Division Tier</th>
                <th>Season Opps</th>
                <th>Season W-L-D</th>
                <th>Season Win %</th>
                <th>Peak Rating</th>
                <th>Year-End Rating</th>
                <th>Career Opps</th>
                <th>Career Win %</th>
                <th>
                  Score Index
                  <span
                    className="score-info"
                    data-tooltip="Composite Performance Index: Evaluates career seasons balancing win rate (wr × 2.0), match volume (wins × 1.5), peak rating bonus, and World Championship title prestige to objectively rank peak performance."
                  >
                    ℹ️
                  </span>
                </th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {scoreYears(years, p.peak).map((y) => {
                const isBest = best !== null && best.year === y.year;
                const shown = isBest ? best.title : y.title;
                return (
                  <tr key={y.year} className={isBest ? 'peak-row' : undefined}>
                    <td>
                      <strong className="career-year">{y.year}</strong>
                      {shown === 'WC' && <span className="badge-wc badge-tiny">WC</span>}
                    </td>
                    <td>{shown !== null ? <TitleBadge code={shown} /> : <span className="career-none">-</span>}</td>
                    <td>{y.opponents}</td>
                    <td>
                      <span className="wld-num win">{y.wins}</span> - <span className="wld-num loss">{y.losses}</span> -{' '}
                      <span className="wld-num draw">{y.draws}</span>
                    </td>
                    <td>
                      <strong>{y.win_rate.toFixed(1)}%</strong>
                    </td>
                    <td>
                      <strong className="career-peak">{y.peak_rating ? y.peak_rating.toFixed(1) : '-'}</strong>
                    </td>
                    <td>{y.year_end_rating ? y.year_end_rating.toFixed(1) : '-'}</td>
                    <td>{y.career_opponents}</td>
                    <td>{y.career_win_rate.toFixed(1)}%</td>
                    <td className="career-score">{y.score.toFixed(1)}</td>
                    <td>
                      {isBest ? (
                        <span className="badge badge-peak-year">
                          👑 Peak Career Year{shown !== null ? ` (${show.titleName(shown)})` : ''}
                        </span>
                      ) : (
                        <span className="career-active">Active</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </ScrollTable>
      </div>
    </>
  );
}

function Message({ children }: { children: ReactNode }) {
  return <div className="rating-card waiting-message">{children}</div>;
}

function Header({ player, view }: { player: Player; view: MatrixView }) {
  const wc = wcLabel(player.title, player.peak);
  const { format } = view;
  return (
    <div className="matrix-header-card">
      <div className="header-back">
        <BackLink href={profileHref(player.name, view)}>Return to Profile</BackLink>
      </div>
      <div>
        <div className="matrix-identity">
          <Flag country={player.country} />
          <h1>{player.name}</h1>
          {wc !== null ? (
            <span className="badge-wc">💎 {wc}</span>
          ) : (
            player.title !== null && (
              <span className={`badge badge-${player.title.code.toLowerCase()}`}>{show.titleName(player.title.code)}</span>
            )
          )}
          {player.peak !== null && (
            <span className={`badge badge-inline badge-${player.peak.code.toLowerCase()}`}>
              Peak: {show.titleName(player.peak.code)} ({player.peak.year ?? 'Career'})
            </span>
          )}
        </div>
        <div className="matrix-subtitle">
          Career Peak Year Evaluation • Format: <strong>{show.formatName(format)}</strong>
        </div>
      </div>
      <div className="matrix-header-links">
        <a href={achievementsHref(player.name)} className="btn-antique">
          🏆 Achievements »
        </a>
      </div>
    </div>
  );
}
