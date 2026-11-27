/** A challenge's board: its ranking, the history of its record with a chart, the entries that do not count, and every submission. */
import { useState } from 'react';
import { formatDate, viewerZone } from '@ttac/shared/time';
import SortHeader, { useSort } from '@ttac/shared/SortHeader';
import type { SortColumns } from '@ttac/shared/sort';
import { challengeBoard, type ChallengeBest, type ChallengeBoard, type ChallengeRecordStep, type ChallengeSubmission } from '@ttac/shared/ratings';
import { CHART, notCounted, recordChart } from '../lib/record-chart';
import { BugTag, ChallengePlayerLink, ProofLink, useChoice, VerifiedSwitch } from './ChallengeParts';
import { useKeptLoad } from './useLoad';
import Waiting from './Waiting';
import Panel from './Panel';
import Panels from './Panels';
import ScrollTable from './ScrollTable';

const RANKING_COLUMNS: SortColumns<ChallengeBest, 'rank' | 'player' | 'domination' | 'date'> = {
  rank: { value: (b) => b.rank, first: 'asc' },
  player: { value: (b) => b.player, first: 'asc' },
  domination: { value: (b) => b.domination, first: 'desc' },
  date: { value: (b) => b.entered_at, first: 'desc' },
};

const ENTRY_COLUMNS: SortColumns<ChallengeSubmission, 'player' | 'domination' | 'date'> = {
  player: { value: (e) => e.player, first: 'asc' },
  domination: { value: (e) => e.domination, first: 'desc' },
  date: { value: (e) => e.entered_at, first: 'desc' },
};

const HISTORY_COLUMNS: SortColumns<ChallengeRecordStep, 'date' | 'player' | 'domination' | 'gain'> = {
  date: { value: (s) => s.entered_at, first: 'desc' },
  player: { value: (s) => s.player, first: 'asc' },
  domination: { value: (s) => s.domination, first: 'desc' },
  gain: { value: (s) => s.gain, first: 'desc' },
};

/** ChallengeBoardPanel loads the board of challenge id for the chosen entries and shows it. */
export default function ChallengeBoardPanel({ apiUrl, id }: { apiUrl: string; id: string }) {
  const [verified, setVerified] = useChoice('verified');
  const load = useKeptLoad(`board-${id}-${verified}`, () => challengeBoard(apiUrl, id, verified === 'verified'));
  const zone = viewerZone();
  const day = (instant: string) => formatDate(instant, zone);

  return (
    <>
      <div className="view-bar challenges-bar">
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
  const [ranking, rankingSort, onRankingSort] = useSort(board.ranking, RANKING_COLUMNS);
  const [bugged, buggedSort, onBuggedSort] = useSort(notCounted(board.entries, verifiedOnly), ENTRY_COLUMNS);
  const [entries, entriesSort, onEntriesSort] = useSort(board.entries, ENTRY_COLUMNS);
  return (
    <Panels>
      <Panel label="Ranking">
        {board.ranking.length === 0 ? (
          <p className="muted">No {verifiedOnly ? 'verified ' : ''}entries yet.</p>
        ) : (
          <ScrollTable label="Leaderboard">
            <table className="leaderboard-table">
              <thead>
                <tr>
                  <SortHeader col="rank" sort={rankingSort} onSort={onRankingSort} align="end">
                    #
                  </SortHeader>
                  <SortHeader col="player" sort={rankingSort} onSort={onRankingSort}>
                    Player
                  </SortHeader>
                  <SortHeader col="domination" sort={rankingSort} onSort={onRankingSort} align="end">
                    <span data-tooltip="Your final score minus the second-place player's score">Domination</span>
                  </SortHeader>
                  <th scope="col">Proof</th>
                  <SortHeader col="date" sort={rankingSort} onSort={onRankingSort}>
                    Date
                  </SortHeader>
                </tr>
              </thead>
              <tbody>
                {ranking.map((b) => (
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
          </ScrollTable>
        )}
      </Panel>

      {board.history.length > 0 && <RecordHistory steps={board.history} day={day} />}

      {bugged.length > 0 && (
        <Panel id="not-counted" title="Not counted">
          <p className="challenges-lead">
            These were made through a game bug that has since been fixed. They stay listed, but don&apos;t count for records, ranks, ratings or totals.
          </p>
          <ScrollTable label="Bug entries">
            <table className="leaderboard-table">
              <caption className="visually-hidden">Entries made through a game bug, not counted</caption>
              <thead>
                <tr>
                  <SortHeader col="player" sort={buggedSort} onSort={onBuggedSort}>
                    Player
                  </SortHeader>
                  <SortHeader col="domination" sort={buggedSort} onSort={onBuggedSort} align="end">
                    Domination
                  </SortHeader>
                  <th scope="col">Proof</th>
                  <SortHeader col="date" sort={buggedSort} onSort={onBuggedSort}>
                    Date
                  </SortHeader>
                </tr>
              </thead>
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
          </ScrollTable>
        </Panel>
      )}

      {board.entries.length > 0 && (
        <Panel label="All submissions">
          <details className="challenges-all">
            <summary>All {board.entries.length} submissions</summary>
            <ScrollTable label="Submissions">
              <table className="leaderboard-table">
                <caption className="visually-hidden">Every submission for the challenge, as entered</caption>
                <thead>
                  <tr>
                    <SortHeader col="date" sort={entriesSort} onSort={onEntriesSort}>
                      Date
                    </SortHeader>
                    <SortHeader col="player" sort={entriesSort} onSort={onEntriesSort}>
                      Player
                    </SortHeader>
                    <SortHeader col="domination" sort={entriesSort} onSort={onEntriesSort} align="end">
                      Domination
                    </SortHeader>
                    <th scope="col">Proof</th>
                  </tr>
                </thead>
                <tbody>
                  {entries.map((e, i) => (
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
            </ScrollTable>
          </details>
        </Panel>
      )}
    </Panels>
  );
}

/** RecordHistory charts each rise of the record and lists them, newest first; the table is the chart's text alternative. */
function RecordHistory({ steps, day }: { steps: ChallengeRecordStep[]; day: (instant: string) => string }) {
  const [hover, setHover] = useState<number | null>(null);
  const chart = recordChart(steps, Date.now());
  const active = hover === null ? null : { step: steps[hover], at: chart.points[hover] };
  const tipLeft = active !== null && active.at.x > CHART.width - 200;
  const [rows, sort, onSort] = useSort([...steps].reverse(), HISTORY_COLUMNS);

  return (
    <Panel id="record-history" title="Record history">
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
      <ScrollTable label="Entries">
        <table className="leaderboard-table">
          <thead>
            <tr>
              <SortHeader col="date" sort={sort} onSort={onSort}>
                Date
              </SortHeader>
              <SortHeader col="player" sort={sort} onSort={onSort}>
                Record holder
              </SortHeader>
              <SortHeader col="domination" sort={sort} onSort={onSort} align="end">
                Domination
              </SortHeader>
              <SortHeader col="gain" sort={sort} onSort={onSort} align="end">
                Improvement
              </SortHeader>
              <th scope="col">Proof</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((s) => (
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
      </ScrollTable>
    </Panel>
  );
}
