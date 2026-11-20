/** A player's career years and historical performance page: every reset mode, and the year-by-year career. */
import { useEffect } from 'react';
import {
  currentRun,
  player as fetchPlayer,
  playerCareer,
  type Player,
  type PlayerCareer,
} from '@ttac/shared/ratings';
import * as show from '../lib/leaderboard-display';
import { DEFAULT_VIEW } from '../lib/leaderboard-view';
import { careerPeakYear, scoreYears } from '../lib/career';
import { matrixHref, matrixParams, pageModel, parseMatrixView, profileHref, type MatrixView } from '../lib/player-view';
import { achievementsHref } from '../lib/achievements';
import { recall } from '../lib/remembered';
import { browserStorage } from '@ttac/shared/storage';
import './PlayerPage.css';
import './MatrixPage.css';
import ScrollTable from './ScrollTable';
import ViewSelectors from './ViewSelectors';
import Waiting from './Waiting';
import { useLoad } from './useLoad';
import Message from './Message';
import PlayerHeader from './PlayerHeader';
import { TitleBadge } from './PlayerMarks';
import { useAddressView, type Address } from './useAddressView';
import Panel from './Panel';
import Panels from './Panels';

const ADDRESS: Address<MatrixView> = {
  parse: parseMatrixView,
  params: matrixParams,
  remembers: (v) => ({ ...DEFAULT_VIEW, ...recall(browserStorage()), reset: v.reset, format: v.format }),
};

/** Shows the player's career years and peak season evaluation. */
export default function MatrixPage({ apiUrl }: { apiUrl: string }) {
  const { view, linkTo } = useAddressView(ADDRESS);
  const run = useLoad('run', () => currentRun(apiUrl).then((r) => r.run_id));


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

      <Panels>
        {best !== null && (
          <div className="peak-year-card">
            <div className="peak-year-badge" aria-hidden="true">
              👑
            </div>
            <div className="peak-year-body">
              <div className="peak-year-kicker">Career Benchmark Evaluation</div>
              <div className="peak-year-title">
                Peak Career Season: <span className="peak-year-year">{best.year}</span>
                {best.title !== null && <TitleBadge title={{ code: best.title }} long className="badge-small" />}
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

        <Panel
          id="matrix-years"
          icon="📅"
          title="Year-by-Year Career Breakdown"
          subtitle={`${years.length} Seasons Recorded • Format: ${formatName}`}
          aside={
            <span className="matrix-card-note">
              Hover on <strong>ℹ️ Score Index</strong> for composite formula details
            </span>
          }
        >
          <ScrollTable hint="full career history">
            <table className="leaderboard-table career-history-table">
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
                      <td>{shown !== null ? <TitleBadge title={{ code: shown }} long className="badge-small" /> : <span className="career-none">-</span>}</td>
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
        </Panel>
      </Panels>
    </>
  );
}


function Header({ player, view }: { player: Player; view: MatrixView }) {
  return (
    <PlayerHeader
      player={player}
      back={{ href: profileHref(player.name, view), label: 'Player Profile' }}
      subtitle={
        <>
          Career Peak Year Evaluation • Format: <strong>{show.formatName(view.format)}</strong>
        </>
      }
      page="years"
      hrefs={{ profile: profileHref(player.name, view), achievements: achievementsHref(player.name), years: matrixHref(player.name, view) }}
    />
  );
}
