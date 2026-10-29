/** A player's challenge page: their figures, their best on each challenge, what to play next, and their latest entries. */
import { useEffect, useState } from 'react';
import Segmented from '@ttac/shared/Segmented';
import { viewerZone } from '@ttac/shared/time';
import { challengePlayer, RatingsApiError, type ChallengePlayer, type ChallengePlayerBest, type ChallengePlayerSummary } from '@ttac/shared/ratings';
import type { ChallengeTile } from '../lib/challenge-list';
import { challengeHref, challengeSets, compareHref, entryDay, formatMeasure, measureOf, nameParam, playNext, setLabel, type Measure, type SetId } from '../lib/challenges';
import { BugTag, MeasureSwitch, ProofLink, ProofMark, SetGrid, useChoice, useKeptLoad } from './ChallengeParts';
import Waiting from './Waiting';

const SETS: { value: SetId; label: string }[] = [
  { value: 'original', label: 'Original' },
  { value: 'expansion', label: 'Expansion' },
  { value: 'mix', label: 'Random Mix' },
];

type Counting = 'verified' | 'as-if' | 'all';

/** ChallengePlayerPage loads the player the address names and shows their results. */
export default function ChallengePlayerPage({ apiUrl, list }: { apiUrl: string; list: ChallengeTile[] }) {
  const [name, setName] = useState<string | null>(null);
  const [verified, setVerified] = useChoice('verified');
  const [asIf, setAsIf] = useState(false);
  const [measure, setMeasure] = useChoice('measure');
  const [set, setSet] = useState<SetId>('mix');
  useEffect(() => setName(nameParam(window.location.search)), []);
  const counting: Counting = verified === 'all' ? 'all' : asIf ? 'as-if' : 'verified';
  const load = useKeptLoad(
    `player-${name}-${counting}`,
    name === null || name === ''
      ? null
      : () =>
          challengePlayer(apiUrl, name, verified === 'verified', counting === 'as-if').catch((e: unknown) => {
            if (e instanceof RatingsApiError && e.status === 404) return null;
            throw e;
          }),
  );
  const choose = (c: Counting) => {
    setVerified(c === 'all' ? 'all' : 'verified');
    setAsIf(c === 'as-if');
  };

  if (name === '') return <div className="rating-card waiting-message">Choose a player from the players list.</div>;
  if (load.kind !== 'ready') return <Waiting load={load} what="this player" />;
  if (load.data === null) {
    return (
      <div className="analysis-hero-card challenges-hero">
        <h1 className="analysis-hero-title">{name}</h1>
        <p className="analysis-hero-desc">No entries for this player.</p>
      </div>
    );
  }
  const p = load.data;
  return (
    <>
      <div className="analysis-hero-card challenges-hero">
        <h1 className="analysis-hero-title">{p.player}</h1>
      </div>
      <div className="challenges-bar">
        <div className="challenges-bar-group">
          <Segmented label="Set" look="raised" value={set} onChange={setSet} options={SETS} />
          {p.summary && (
            <span className="muted">
              {measure === 'rating' ? 'Rating' : 'Total'} <strong>{formatMeasure(measureOf(p.summary, measure, set), measure)}</strong> ·{' '}
              {p.bests.filter((b) => list.find((c) => c.id === b.challenge)?.set === set).length} / {list.filter((c) => c.set === set).length} played
            </span>
          )}
        </div>
        <div className="challenges-bar-group">
          <MeasureSwitch value={measure} onChange={setMeasure} />
          <Segmented
            label="Which entries count"
            look="raised"
            value={counting}
            onChange={choose}
            options={[
              { value: 'verified' as const, label: 'Verified only' },
              ...(p.unproven > 0 ? [{ value: 'as-if' as const, label: 'As if verified', title: `Count ${p.player}'s unproven entries as if they were verified` }] : []),
              { value: 'all' as const, label: 'All entries' },
            ]}
          />
        </div>
      </div>
      {counting === 'as-if' && (
        <p className="rating-card challenges-what-if">
          <strong>What if:</strong> all {p.unproven} unproven {p.unproven === 1 ? 'entry' : 'entries'} of {p.player} count as verified, measured against the
          official records (which stay as they are), so a best above the record rates over 100%. Post the screenshots in Discord and add the links to make it
          official.
        </p>
      )}
      {p.summary === null ? (
        <div className="rating-card waiting-message">
          {p.player} has no verified entries yet.{' '}
          <button type="button" className="btn-antique" onClick={() => choose('all')}>
            Show all entries
          </button>
        </div>
      ) : (
        <Results player={p} summary={p.summary} list={list} set={set} measure={measure} />
      )}
    </>
  );
}

interface ResultsProps {
  player: ChallengePlayer;
  summary: ChallengePlayerSummary;
  list: ChallengeTile[];
  set: SetId;
  measure: Measure;
}

function Results({ player: p, summary, list, set, measure }: ResultsProps) {
  const bests = new Map(p.bests.map((b) => [b.challenge, b]));
  const groups = challengeSets(list).find((s) => s.id === set)?.groups ?? [];
  const zone = viewerZone();
  return (
    <>
      <div className="challenges-stats">
        <Stat label={measure === 'rating' ? 'Rating' : 'Total domination'} value={formatMeasure(measureOf(summary, measure), measure)} />
        <Stat label="Records held" value={summary.records} />
        <Stat label="Challenges played" value={`${summary.played} / ${list.length}`} />
        <Stat label="Verified" value={summary.verified} />
      </div>
      <p>
        <a className="btn-antique" href={compareHref(p.player)}>
          Compare with someone…
        </a>
      </p>
      <section className="rating-card challenges-card" aria-label="Best on each challenge">
        <SetGrid
          groups={groups}
          cell={(c) => {
            const b = bests.get(c.id);
            if (b === undefined) return <span className="muted">—</span>;
            return (
              <span className={b.rank === 1 ? 'challenges-record' : undefined} title={measure === 'rating' ? `Domination ${b.domination}` : `${Math.round(b.share)}% of the record`}>
                <strong>{measure === 'rating' ? `${Math.round(b.share)}%` : b.domination}</strong>{' '}
                <span className="muted">
                  #{b.rank}/{b.entrants}
                </span>
                <ProofMark url={b.proof_url} />
              </span>
            );
          }}
        />
      </section>
      <PlayNext player={p} list={list} measure={measure} />
      <section className="rating-card challenges-card" aria-labelledby="latest">
        <h2 id="latest" className="rating-card-heading">
          Latest submissions
        </h2>
        <ul className="challenges-latest">
          {p.recent.map((e, i) => {
            const c = list.find((x) => x.id === e.challenge);
            return (
              <li key={i}>
                <span className="muted">{entryDay(e.entered_at, zone)}</span> — <a href={challengeHref(e.challenge)}>{c?.name ?? e.challenge}</a>{' '}
                {c && <span className="muted">({setLabel(c.set)})</span>}: <strong>{e.domination}</strong>
                {e.bugged && <BugTag />} <ProofLink url={e.proof_url} />
              </li>
            );
          })}
        </ul>
      </section>
    </>
  );
}

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="challenges-stat">
      <div className="challenges-stat-value">{value}</div>
      <div className="challenges-stat-label">{label}</div>
    </div>
  );
}

/** PlayNext suggests records within reach and the lowest results, in the chosen measure. */
function PlayNext({ player: p, list, measure }: { player: ChallengePlayer; list: ChallengeTile[]; measure: Measure }) {
  const { closest, lowest } = playNext(p.bests, measure, 8);
  const unplayed = list.length - p.bests.length;
  if (closest.length === 0 && lowest.length === 0 && unplayed === 0) return null;
  const label = (b: ChallengePlayerBest) => {
    const c = list.find((x) => x.id === b.challenge);
    return (
      <>
        <a href={challengeHref(b.challenge)}>{c?.name ?? b.challenge}</a> {c && <span className="muted">({setLabel(c.set)})</span>}
      </>
    );
  };
  return (
    <section className="rating-card challenges-card" aria-labelledby="play-next">
      <h2 id="play-next" className="rating-card-heading">
        What to play next
      </h2>
      <div className="challenges-play-next">
        {closest.length > 0 && (
          <div>
            <h3 className="challenges-subhead">Records within reach</h3>
            <p className="muted">
              Where {p.player}&apos;s best is closest to the record{measure === 'rating' ? ' (in % of the record)' : ' (fewest points to beat it)'}.
            </p>
            <table className="leaderboard-table">
              <tbody>
                {closest.map((b) => (
                  <tr key={b.challenge}>
                    <td>{label(b)}</td>
                    <td className="num">{measure === 'rating' ? `${Math.round(b.share)}%` : `${b.domination}/${b.record}`}</td>
                    <td className="num muted" title={`Record: ${b.record} by ${b.record_holder}`}>
                      {b.record - b.domination + 1} to beat
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {(lowest.length > 0 || unplayed > 0) && (
          <div>
            <h3 className="challenges-subhead">Lowest results</h3>
            <p className="muted">
              {measure === 'rating' ? `Where ${p.player}'s best is the lowest % of the record: the most rating to gain.` : `${p.player}'s lowest best dominations.`}
            </p>
            {lowest.length > 0 && (
              <table className="leaderboard-table">
                <tbody>
                  {lowest.map((b) => (
                    <tr key={b.challenge}>
                      <td>{label(b)}</td>
                      {measure === 'rating' && <td className="num">{Math.round(b.share)}%</td>}
                      <td className={measure === 'rating' ? 'num muted' : 'num'} title={`Record by ${b.record_holder}`}>
                        {b.domination}/{b.record}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            {unplayed > 0 && (
              <p className="muted">
                Plus {unplayed} {unplayed === 1 ? 'challenge' : 'challenges'} not played yet, each counting as 0%.
              </p>
            )}
          </div>
        )}
      </div>
    </section>
  );
}
