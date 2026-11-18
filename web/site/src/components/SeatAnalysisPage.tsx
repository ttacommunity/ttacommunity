/** Turn order and starting seat advantage page: win rates and placement rates by player count and skill tier. */
import { useState } from 'react';
import { currentRun, seatAnalysis, type Format, type SeatAnalysisTier, type SeatStatsGroup, type SeatWinrateRow, type SeatPlaceRate } from '@ttac/shared/ratings';
import Segmented, { type Option } from '@ttac/shared/Segmented';
import { SEAT_TIERS, seatDelta, seatDeltaText, seatSpread } from '../lib/analysis';
import MetricCard, { MetricGrid } from './MetricCard';
import ScrollTable from './ScrollTable';
import Waiting from './Waiting';
import { count } from '@ttac/shared/numbers';
import { useKeptLoad } from './useLoad';
import PanelHeader from './PanelHeader';
import { OptionsPanel, OptionsSection, OptionsToggle } from './SideDrawer';
import ViewBar, { FormatPick } from './ViewBar';

const TIER_OPTIONS: Option<SeatAnalysisTier>[] = SEAT_TIERS.map((t) => ({
  value: t.value,
  label: t.label,
  tooltip: t.tooltip,
}));

const SEAT_NAMES: Record<number, string> = {
  1: 'Seat 1 (Starting Player)',
  2: 'Seat 2 (Second Player)',
  3: 'Seat 3 (Third Player)',
  4: 'Seat 4 (Fourth Player)',
};

/** SeatAnalysisPage displays empirical win rates and finishing placements by turn order. */
export default function SeatAnalysisPage({ apiUrl }: { apiUrl: string }) {
  const [tier, setTier] = useState<SeatAnalysisTier>('all');
  const [format, setFormat] = useState<Format>(0);
  const [optionsOpen, setOptionsOpen] = useState(false);

  const load = useKeptLoad(`${tier}/${format}`, () => currentRun(apiUrl).then((r) => seatAnalysis(apiUrl, r.run_id, tier, format)));

  const bar = (
    <>
      <ViewBar actions={<OptionsToggle title="Skill tier" active={tier !== 'all'} open={optionsOpen} onClick={() => setOptionsOpen(!optionsOpen)} />}>
        <FormatPick value={format} onChange={setFormat} what="seat win rates" />
      </ViewBar>
      {optionsOpen && (
        <OptionsPanel label="Seat analysis options" onClose={() => setOptionsOpen(false)} reset={() => setTier('all')}>
          <OptionsSection icon="⚔️" title="Skill Tier" tip="Glicko-D average of the table">
            <Segmented label="Skill tier" options={TIER_OPTIONS} value={tier} onChange={setTier} />
          </OptionsSection>
        </OptionsPanel>
      )}
    </>
  );

  if (load.kind !== 'ready') {
    return (
      <>
        {bar}
        <Waiting load={load} what="seat win rates" />
      </>
    );
  }

  const data = load.data;
  const totalGames = data.groups.reduce((acc: number, g: SeatStatsGroup) => acc + g.total_games, 0);

  const group4P = data.groups.find((g: SeatStatsGroup) => g.player_count === 4);
  const group3P = data.groups.find((g: SeatStatsGroup) => g.player_count === 3);
  const group2P = data.groups.find((g: SeatStatsGroup) => g.player_count === 2);

  const delta4P = seatSpread(group4P);
  const delta3P = seatSpread(group3P);
  const delta2P = seatSpread(group2P);

  return (
    <>
      {bar}
      <MetricGrid>
        <MetricCard label="Analyzed Matches" value={count(totalGames)} note="Full corpus sample" tone="win" />
        <MetricCard label="4-Player Seat 1 vs 4" value={delta4P !== null ? seatDeltaText(delta4P) : '—'} note="First vs fourth player spread" />
        <MetricCard label="3-Player Seat 1 vs 3" value={delta3P !== null ? seatDeltaText(delta3P) : '—'} note="First vs third player spread" />
        <MetricCard label="2-Player Seat 1 vs 2" value={delta2P !== null ? seatDeltaText(delta2P) : '—'} note="Starting duel advantage" />
      </MetricGrid>

      <div className="seat-groups-stack">
        {data.groups.map((group: SeatStatsGroup) => {
          const pc = group.player_count;
          const parity = (100 / pc).toFixed(2);
          return (
            <div key={pc} className="rating-card panel-card eval-table-card">
              <PanelHeader title={`${pc}-Player Matches • ${count(group.total_games)} games`}>
                <span className="seat-parity-note">Expected Parity: {parity}% per seat</span>
              </PanelHeader>
              <ScrollTable hint="Scroll horizontally for all placement columns">
                <table className="leaderboard-table">
                  <caption className="visually-hidden">
                    {pc}-Player Turn Order Win Rates and Placement Distribution
                  </caption>
                  <thead>
                    <tr>
                      <th scope="col" className="seat-col-order">Turn Order</th>
                      <th scope="col" className="seat-col-num">Matches</th>
                      <th scope="col" className="seat-col-num">Wins</th>
                      <th scope="col" className="seat-col-num">Draws</th>
                      <th scope="col" className="seat-col-num">Win Rate</th>
                      <th scope="col" className="seat-col-num">Delta vs Parity</th>
                      {Array.from({ length: pc }, (_, idx) => (
                        <th key={idx + 1} scope="col" className="seat-col-num">
                          {idx + 1 === 1 ? '1st Place' : idx + 1 === 2 ? '2nd Place' : idx + 1 === 3 ? '3rd Place' : '4th Place'}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {group.seats.map((s: SeatWinrateRow) => {
                      const delta = seatDelta(s.win_rate, pc);
                      const deltaClass = delta > 0 ? 'delta-pos' : delta < 0 ? 'delta-neg' : 'delta-zero';
                      return (
                        <tr key={s.seat}>
                          <td className="seat-cell-order">{SEAT_NAMES[s.seat] ?? `Seat ${s.seat}`}</td>
                          <td className="seat-col-num">{count(s.games)}</td>
                          <td className="seat-cell-wins">{count(s.wins)}</td>
                          <td className="seat-col-num">{count(s.draws)}</td>
                          <td className="seat-cell-rate">
                            {s.win_rate.toFixed(2)}%
                          </td>
                          <td className={`seat-cell-delta ${deltaClass}`}>
                            {seatDeltaText(delta)}
                          </td>
                          {Array.from({ length: pc }, (_, idx) => {
                            const placeNum = idx + 1;
                            const p = s.places.find((pl: SeatPlaceRate) => pl.place === placeNum);
                            const rate = p ? p.rate.toFixed(2) : '0.00';
                            const pCount = p ? count(p.count) : '0';
                            return (
                              <td key={placeNum} className="seat-col-num">
                                <span>{rate}%</span>{' '}
                                <span className="seat-place-count">({pCount})</span>
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
