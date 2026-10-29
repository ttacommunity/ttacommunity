/** The challenge players: everyone with a counted entry, searchable and sortable, by rating or total. */
import { useState } from 'react';
import SortHeader, { useSort } from '@ttac/shared/SortHeader';
import type { SortColumns } from '@ttac/shared/sort';
import { viewerZone } from '@ttac/shared/time';
import { challengePlayers, type ChallengePlayerSummary } from '@ttac/shared/ratings';
import { byMeasure, entryDay, formatMeasure, matchingPlayers, measureOf, type Measure } from '../lib/challenges';
import { ChallengePlayerLink, MeasureSwitch, useChoice, VerifiedSwitch } from './ChallengeParts';
import FilterInput from './FilterInput';
import { useLoad } from './useLoad';
import Waiting from './Waiting';

type Col = 'player' | 'overall' | 'records' | 'played' | 'verified' | 'original' | 'expansion' | 'mix' | 'last';

function columns(measure: Measure): SortColumns<ChallengePlayerSummary, Col> {
  return {
    player: { value: (p) => p.player, first: 'asc' },
    overall: { value: (p) => measureOf(p, measure), first: 'desc' },
    records: { value: (p) => p.records, first: 'desc' },
    played: { value: (p) => p.played, first: 'desc' },
    verified: { value: (p) => p.verified, first: 'desc' },
    original: { value: (p) => measureOf(p, measure, 'original'), first: 'desc' },
    expansion: { value: (p) => measureOf(p, measure, 'expansion'), first: 'desc' },
    mix: { value: (p) => measureOf(p, measure, 'mix'), first: 'desc' },
    last: { value: (p) => p.last_entry, first: 'desc' },
  };
}

/** ChallengePlayersPage loads the players for the chosen entries and lists them. */
export default function ChallengePlayersPage({ apiUrl }: { apiUrl: string }) {
  const [verified, setVerified] = useChoice('verified');
  const [measure, setMeasure] = useChoice('measure');
  const [query, setQuery] = useState('');
  const load = useLoad(`players-${verified}`, () => challengePlayers(apiUrl, verified === 'verified'));
  const players = load.kind === 'ready' ? matchingPlayers(byMeasure(load.data.players, measure), query) : [];
  const [rows, sort, onSort] = useSort(players, columns(measure));
  const zone = viewerZone();

  return (
    <>
      <div className="analysis-hero-card challenges-hero">
        <div>
          <h1 className="analysis-hero-title">Players</h1>
          <p className="analysis-hero-desc">
            Switch between <strong>Rating</strong>: each best domination as a percentage of the record (holding the record = 100%), averaged over every
            challenge, so unplayed ones count as 0%; and <strong>Total</strong>: the plain sum of each player&apos;s best dominations. Click a column to sort.
          </p>
        </div>
      </div>
      <div className="challenges-bar">
        <FilterInput value={query} onChange={setQuery} label="Search players by name" placeholder="Search players…" />
        <div className="challenges-bar-group">
          <MeasureSwitch value={measure} onChange={setMeasure} />
          <VerifiedSwitch value={verified} onChange={setVerified} />
        </div>
      </div>
      {load.kind !== 'ready' ? (
        <Waiting load={load} what="the players" />
      ) : (
        <section className="rating-card challenges-card" aria-label="Players">
          <div className="table-responsive">
            <table className="leaderboard-table">
              <thead>
                <tr>
                  <SortHeader col="player" sort={sort} onSort={onSort}>
                    Player
                  </SortHeader>
                  <SortHeader col="overall" sort={sort} onSort={onSort} className="num">
                    {measure === 'rating' ? 'Rating' : 'Total'}
                  </SortHeader>
                  <SortHeader col="records" sort={sort} onSort={onSort} className="num">
                    Records
                  </SortHeader>
                  <SortHeader col="played" sort={sort} onSort={onSort} className="num">
                    Played
                  </SortHeader>
                  <SortHeader col="verified" sort={sort} onSort={onSort} className="num">
                    Verified
                  </SortHeader>
                  <SortHeader col="original" sort={sort} onSort={onSort} className="num">
                    Original
                  </SortHeader>
                  <SortHeader col="expansion" sort={sort} onSort={onSort} className="num">
                    Expansion
                  </SortHeader>
                  <SortHeader col="mix" sort={sort} onSort={onSort} className="num">
                    Mix
                  </SortHeader>
                  <SortHeader col="last" sort={sort} onSort={onSort}>
                    Last entry
                  </SortHeader>
                </tr>
              </thead>
              <tbody>
                {rows.map((p) => (
                  <tr key={p.player}>
                    <td>
                      <ChallengePlayerLink name={p.player} />
                    </td>
                    <td className="num challenges-score">{formatMeasure(measureOf(p, measure), measure)}</td>
                    <td className="num">{p.records || ''}</td>
                    <td className="num">{p.played}</td>
                    <td className="num">{p.verified}</td>
                    <td className="num">{formatMeasure(measureOf(p, measure, 'original'), measure)}</td>
                    <td className="num">{formatMeasure(measureOf(p, measure, 'expansion'), measure)}</td>
                    <td className="num">{formatMeasure(measureOf(p, measure, 'mix'), measure)}</td>
                    <td className="muted">{entryDay(p.last_entry, zone)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </>
  );
}
