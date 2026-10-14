/** Turn order and starting seat advantage page: win rates and placement rates by player count and skill tier. */
import { useEffect, useState } from 'react';
import {
  currentRun,
  seatAnalysis,
  type Format,
  type SeatAnalysis,
  type SeatAnalysisTier,
  type SeatStatsGroup,
  type SeatWinrateRow,
  type SeatPlaceRate,
} from '@ttac/shared/ratings';
import Segmented, { type Option } from '@ttac/shared/Segmented';
import { SEAT_TIERS, seatDelta, seatDeltaText } from '../lib/analysis';
import ScrollTable from './ScrollTable';
import Waiting from './Waiting';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: SeatAnalysis };

const FORMAT_OPTIONS: Option<Format>[] = [
  { value: 0, label: 'All Player Counts', tooltip: 'Show 2-Player, 3-Player, and 4-Player tables' },
  { value: 2, label: '2-Player (Duels)', tooltip: 'Direct heads-up duels' },
  { value: 3, label: '3-Player Games', tooltip: 'Most popular tournament format' },
  { value: 4, label: '4-Player Games', tooltip: 'Full table political strategy' },
];

const TIER_OPTIONS: Option<SeatAnalysisTier>[] = SEAT_TIERS.map((t) => ({
  value: t.value,
  label: t.label,
  tooltip: t.tooltip,
}));

const count = (n: number) => n.toLocaleString('en-US');

const SEAT_NAMES: Record<number, string> = {
  1: 'Seat 1 (Starting Player)',
  2: 'Seat 2 (Second Player)',
  3: 'Seat 3 (Third Player)',
  4: 'Seat 4 (Fourth Player)',
};

/** SeatAnalysisPage displays empirical win rates and finishing placements by turn order. */
export default function SeatAnalysisPage({ apiUrl }: { apiUrl: string }) {
  const [load, setLoad] = useState<Load>({ kind: 'loading' });
  const [tier, setTier] = useState<SeatAnalysisTier>('all');
  const [format, setFormat] = useState<Format>(0);

  useEffect(() => {
    let cancelled = false;
    currentRun(apiUrl)
      .then((r) => seatAnalysis(apiUrl, r.run_id, tier, format))
      .then((data) => {
        if (!cancelled) setLoad({ kind: 'ready', data });
      })
      .catch((e: unknown) => {
        if (!cancelled) setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) });
      });
    return () => {
      cancelled = true;
    };
  }, [apiUrl, tier, format]);

  if (load.kind !== 'ready') return <Waiting load={load} what="seat win rates" />;

  const data = load.data;
  const totalGames = data.groups.reduce((acc: number, g: SeatStatsGroup) => acc + g.total_games, 0);

  const group4P = data.groups.find((g: SeatStatsGroup) => g.player_count === 4);
  const group3P = data.groups.find((g: SeatStatsGroup) => g.player_count === 3);
  const group2P = data.groups.find((g: SeatStatsGroup) => g.player_count === 2);

  const delta4P = group4P && group4P.seats.length >= 4 ? (group4P.seats[0].win_rate - group4P.seats[3].win_rate).toFixed(2) : null;
  const delta3P = group3P && group3P.seats.length >= 3 ? (group3P.seats[0].win_rate - group3P.seats[2].win_rate).toFixed(2) : null;
  const delta2P = group2P && group2P.seats.length >= 2 ? (group2P.seats[0].win_rate - group2P.seats[1].win_rate).toFixed(2) : null;

  return (
    <>
      <div className="stats-grid-4">
        <div className="metric-card">
          <div className="metric-label">Analyzed Matches</div>
          <div className="metric-number metric-active">{count(totalGames)}</div>
          <div className="metric-note">Full corpus sample</div>
        </div>
        <div className="metric-card">
          <div className="metric-label">4-Player Seat 1 vs 4</div>
          <div className="metric-number">{delta4P !== null ? `+${delta4P}%` : '—'}</div>
          <div className="metric-note">First vs fourth player spread</div>
        </div>
        <div className="metric-card">
          <div className="metric-label">3-Player Seat 1 vs 3</div>
          <div className="metric-number">{delta3P !== null ? `+${delta3P}%` : '—'}</div>
          <div className="metric-note">First vs third player spread</div>
        </div>
        <div className="metric-card">
          <div className="metric-label">2-Player Seat 1 vs 2</div>
          <div className="metric-number">{delta2P !== null ? `${Number(delta2P) > 0 ? '+' : ''}${delta2P}%` : '—'}</div>
          <div className="metric-note">Starting duel advantage</div>
        </div>
      </div>

      <div className="card-head" style={{ marginTop: '1.5rem', marginBottom: '0.75rem' }}>
        <h3>Filter by Player Strength &amp; Match Format</h3>
      </div>

      <div className="eval-table-card" style={{ marginBottom: '1.5rem' }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
          <div>
            <div style={{ fontSize: '0.82rem', fontWeight: 600, color: 'var(--text-muted)', marginBottom: '0.35rem' }}>Skill Tier (Glicko-D Table Average):</div>
            <Segmented
              label="Skill Tier"
              options={TIER_OPTIONS}
              value={tier}
              look="raised"
              onChange={(t) => setTier(t)}
            />
          </div>
          <div>
            <div style={{ fontSize: '0.82rem', fontWeight: 600, color: 'var(--text-muted)', marginBottom: '0.35rem' }}>Format:</div>
            <Segmented
              label="Player Count"
              options={FORMAT_OPTIONS}
              value={format}
              look="bar"
              onChange={(f) => setFormat(f)}
            />
          </div>
        </div>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem', marginBottom: '2rem' }}>
        {data.groups.map((group: SeatStatsGroup) => {
          const pc = group.player_count;
          const parity = (100 / pc).toFixed(2);
          return (
            <div key={pc} className="eval-table-card">
              <div className="card-head">
                <h3>
                  {pc}-Player Matches &bull; {count(group.total_games)} games
                </h3>
                <span style={{ fontSize: '0.85rem', color: 'var(--text-muted)' }}>
                  Expected Parity: {parity}% per seat
                </span>
              </div>
              <ScrollTable hint="Scroll horizontally for all placement columns">
                <table className="site-table">
                  <caption className="sr-only">
                    {pc}-Player Turn Order Win Rates and Placement Distribution
                  </caption>
                  <thead>
                    <tr>
                      <th scope="col" style={{ width: '22%' }}>Turn Order</th>
                      <th scope="col" style={{ textAlign: 'right' }}>Matches</th>
                      <th scope="col" style={{ textAlign: 'right' }}>Wins (1st)</th>
                      <th scope="col" style={{ textAlign: 'right' }}>Win Rate</th>
                      <th scope="col" style={{ textAlign: 'right' }}>Delta vs Parity</th>
                      {Array.from({ length: pc }, (_, idx) => (
                        <th key={idx + 1} scope="col" style={{ textAlign: 'right' }}>
                          {idx + 1 === 1 ? '1st Place' : idx + 1 === 2 ? '2nd Place' : idx + 1 === 3 ? '3rd Place' : '4th Place'}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {group.seats.map((s: SeatWinrateRow) => {
                      const delta = seatDelta(s.win_rate, pc);
                      const deltaColor = delta > 0 ? 'var(--accent-green, #4ade80)' : delta < 0 ? 'var(--accent-red, #ef5350)' : 'var(--text-muted)';
                      return (
                        <tr key={s.seat}>
                          <td style={{ fontWeight: 600 }}>{SEAT_NAMES[s.seat] ?? `Seat ${s.seat}`}</td>
                          <td style={{ textAlign: 'right' }}>{count(s.games)}</td>
                          <td style={{ textAlign: 'right', fontWeight: 600 }}>{count(s.wins)}</td>
                          <td style={{ textAlign: 'right', fontWeight: 700, color: 'var(--accent-gold)' }}>
                            {s.win_rate.toFixed(2)}%
                          </td>
                          <td style={{ textAlign: 'right', fontWeight: 600, color: deltaColor }}>
                            {seatDeltaText(delta)}
                          </td>
                          {Array.from({ length: pc }, (_, idx) => {
                            const placeNum = idx + 1;
                            const p = s.places.find((pl: SeatPlaceRate) => pl.place === placeNum);
                            const rate = p ? p.rate.toFixed(2) : '0.00';
                            const pCount = p ? count(p.count) : '0';
                            return (
                              <td key={placeNum} style={{ textAlign: 'right' }}>
                                <span>{rate}%</span>{' '}
                                <span style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>({pCount})</span>
                              </td>
                            );
                          })}
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </ScrollTable>
            </div>
          );
        })}
      </div>
    </>
  );
}
