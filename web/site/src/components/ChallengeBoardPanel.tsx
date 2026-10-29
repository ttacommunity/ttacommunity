/** A challenge's board: its ranking, the history of its record with a chart, the entries that do not count, and every submission. */
import { useState } from 'react';
import { viewerZone } from '@ttac/shared/time';
import { challengeBoard, type ChallengeBoard, type ChallengeRecordStep } from '@ttac/shared/ratings';
import { entryDay } from '../lib/challenges';
import { CHART, notCounted, recordChart } from '../lib/record-chart';
import { BugTag, ChallengePlayerLink, ProofLink, useChoice, VerifiedSwitch } from './ChallengeParts';
import { useLoad } from './useLoad';
import Waiting from './Waiting';

/** ChallengeBoardPanel loads the board of challenge id for the chosen entries and shows it. */
export default function ChallengeBoardPanel({ apiUrl, id }: { apiUrl: string; id: string }) {
  const [verified, setVerified] = useChoice('verified');
  const load = useLoad(`board-${id}-${verified}`, () => challengeBoard(apiUrl, id, verified === 'verified'));
  const zone = viewerZone();
  const day = (instant: string) => entryDay(instant, zone);

  return (
    <>
      <div className="challenges-bar">
        <h2 className="rating-card-heading challenges-bar-title">Ranking</h2>
        <VerifiedSwitch value={verified} onChange={setVerified} />
      </div>
      {load.kind !== 'ready' ? (
        <Waiting load={load} what="the ranking" />
      ) : (
        <Board board={load.data} verifiedOnly={verified === 'verified'} day={day} />
      )}
    </>
  );
}

function Board({ board, verifiedOnly, day }: { board: ChallengeBoard; verifiedOnly: boolean; day: (instant: string) => string }) {
  const bugged = notCounted(board.entries, verifiedOnly);
  return (
    <>
      <section className="rating-card challenges-card" aria-label="Ranking">
        {board.ranking.length === 0 ? (
          <p className="muted">No {verifiedOnly ? 'verified ' : ''}entries yet.</p>
        ) : (
          <div className="table-responsive">
            <table className="leaderboard-table">
              <thead>
                <tr>
                  <th scope="col" className="num">
                    #
                  </th>
                  <th scope="col">Player</th>
                  <th scope="col" className="num" title="Your final score minus the second-place player's score">
                    Domination
                  </th>
                  <th scope="col">Proof</th>
                  <th scope="col">Date</th>
                </tr>
              </thead>
              <tbody>
                {board.ranking.map((b) => (
                  <tr key={b.player} className={b.rank === 1 ? 'challenges-first' : undefined}>
                    <td className="num">{b.rank}</td>
                    <td>
                      <ChallengePlayerLink name={b.player} />
                    </td>
                    <td className="num challenges-score">{b.domination}</td>
                    <td>
                      <ProofLink url={b.proof_url} />
                    </td>
                    <td className="muted">{day(b.entered_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {board.history.length > 0 && <RecordHistory steps={board.history} day={day} />}

      {bugged.length > 0 && (
        <section className="rating-card challenges-card" aria-labelledby="not-counted">
          <h3 id="not-counted" className="rating-card-heading">
            Not counted
          </h3>
          <p className="challenges-lead">
            These were made through a game bug that has since been fixed. They stay listed, but don&apos;t count for records, ranks, ratings or totals.
          </p>
          <div className="table-responsive">
            <table className="leaderboard-table">
              <tbody>
                {bugged.map((e) => (
                  <tr key={`${e.player}-${e.entered_at}-${e.domination}`}>
                    <td>
                      <ChallengePlayerLink name={e.player} />
                    </td>
                    <td className="num">
                      {e.domination}
                      <BugTag />
                    </td>
                    <td>
                      <ProofLink url={e.proof_url} />
                    </td>
                    <td className="muted">{day(e.entered_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {board.entries.length > 0 && (
        <details className="rating-card challenges-card challenges-all">
          <summary>All {board.entries.length} submissions</summary>
          <div className="table-responsive">
            <table className="leaderboard-table">
              <tbody>
                {board.entries.map((e, i) => (
                  <tr key={i}>
                    <td className="muted">{day(e.entered_at)}</td>
                    <td>
                      <ChallengePlayerLink name={e.player} />
                    </td>
                    <td className="num">
                      {e.domination}
                      {e.bugged && <BugTag />}
                    </td>
                    <td>
                      <ProofLink url={e.proof_url} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}
    </>
  );
}

/** RecordHistory charts each rise of the record and lists them, newest first; the table is the chart's text alternative. */
function RecordHistory({ steps, day }: { steps: ChallengeRecordStep[]; day: (instant: string) => string }) {
  const [hover, setHover] = useState<number | null>(null);
  const chart = recordChart(steps, Date.now());
  const active = hover === null ? null : { step: steps[hover], at: chart.points[hover] };
  const tipLeft = active !== null && active.at.x > CHART.width - 200;

  return (
    <section className="rating-card challenges-card" aria-labelledby="record-history">
      <h3 id="record-history" className="rating-card-heading">
        Record history
      </h3>
      {steps.length > 1 && (
        <svg className="challenges-chart" viewBox={`0 0 ${CHART.width} ${CHART.height}`} role="img" aria-label="The record over time; the table below lists each rise" onMouseLeave={() => setHover(null)}>
          {chart.yTicks.map((t) => (
            <g key={t.value}>
              <line className="grid" x1={CHART.left} x2={CHART.width - CHART.right} y1={t.y} y2={t.y} />
              <text className="axis" x={CHART.left - 8} y={t.y} textAnchor="end" dominantBaseline="middle">
                {t.value}
              </text>
            </g>
          ))}
          {chart.xTicks.map((t) => (
            <text key={t.year} className="axis" x={t.x} y={CHART.height - 8} textAnchor="middle">
              {t.year}
            </text>
          ))}
          <path className="record-line" d={chart.path} />
          {chart.points.map((p, i) => (
            <g key={i} onMouseEnter={() => setHover(i)} onFocus={() => setHover(i)} onBlur={() => setHover(null)} tabIndex={0}>
              <circle className="hit" cx={p.x} cy={p.y} r={14} />
              <circle className={hover === i ? 'marker active' : 'marker'} cx={p.x} cy={p.y} r={5} />
            </g>
          ))}
          {active && (
            <g className="chart-tip" transform={`translate(${tipLeft ? active.at.x - 190 : active.at.x + 12}, ${Math.max(active.at.y - 46, 4)})`}>
              <rect width={178} height={42} rx={4} />
              <text x={8} y={17}>
                {active.step.domination} by {active.step.player}
              </text>
              <text className="muted" x={8} y={34}>
                {day(active.step.entered_at)}
                {active.step.gain !== null && ` · +${active.step.gain}`}
              </text>
            </g>
          )}
        </svg>
      )}
      <div className="table-responsive">
        <table className="leaderboard-table">
          <thead>
            <tr>
              <th scope="col">Date</th>
              <th scope="col">Record holder</th>
              <th scope="col" className="num">
                Domination
              </th>
              <th scope="col" className="num">
                Improvement
              </th>
              <th scope="col">Proof</th>
            </tr>
          </thead>
          <tbody>
            {[...steps].reverse().map((s) => (
              <tr key={`${s.entered_at}-${s.domination}`}>
                <td className="muted">{day(s.entered_at)}</td>
                <td>
                  <ChallengePlayerLink name={s.player} />
                </td>
                <td className="num challenges-score">{s.domination}</td>
                <td className="num muted">{s.gain === null ? 'first' : `+${s.gain}`}</td>
                <td>
                  <ProofLink url={s.proof_url} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
