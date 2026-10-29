/** Two players' challenge results side by side: wins and ties, totals by set, and each challenge's difference. */
import { useEffect, useState } from 'react';
import Segmented from '@ttac/shared/Segmented';
import { challengePlayer, challengePlayers, RatingsApiError, type ChallengePlayer, type ChallengePlayerSummary } from '@ttac/shared/ratings';
import type { ChallengeTile } from '../lib/challenge-list';
import { challengeSets, compareBests, formatMeasure, measureOf, setLabel, signed, type Measure, type SetId } from '../lib/challenges';
import { ChallengePlayerLink, MeasureSwitch, SetGrid, useChoice, VerifiedSwitch } from './ChallengeParts';
import { useLoad } from './useLoad';
import Waiting from './Waiting';

const SETS: { value: SetId; label: string }[] = [
  { value: 'original', label: 'Original' },
  { value: 'expansion', label: 'Expansion' },
  { value: 'mix', label: 'Random Mix' },
];

/** ChallengeComparePage compares the players the address names (?a= and ?b=), keeping the address in step as they are chosen. */
export default function ChallengeComparePage({ apiUrl, list }: { apiUrl: string; list: ChallengeTile[] }) {
  const [verified, setVerified] = useChoice('verified');
  const [measure, setMeasure] = useChoice('measure');
  const [set, setSet] = useState<SetId>('mix');
  const [a, setA] = useState('');
  const [b, setB] = useState('');
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    setA(params.get('a') ?? '');
    setB(params.get('b') ?? '');
  }, []);
  const v = verified === 'verified';
  const players = useLoad(`players-${verified}`, () => challengePlayers(apiUrl, v));
  const known = players.kind === 'ready' ? players.data.players : [];
  const find = (name: string) => known.find((p) => p.player.toLowerCase() === name.trim().toLowerCase());
  const pa = find(a);
  const pb = find(b);
  const sideA = useLoad(`cmp-${pa?.player}-${verified}`, pa ? () => challengePlayer(apiUrl, pa.player, v).catch(missing) : null);
  const sideB = useLoad(`cmp-${pb?.player}-${verified}`, pb ? () => challengePlayer(apiUrl, pb.player, v).catch(missing) : null);

  const choose = (which: 'a' | 'b', value: string) => {
    (which === 'a' ? setA : setB)(value);
    const url = new URL(window.location.href);
    if (value) url.searchParams.set(which, value);
    else url.searchParams.delete(which);
    history.replaceState(null, '', url);
  };

  return (
    <>
      <div className="analysis-hero-card challenges-hero">
        <h1 className="analysis-hero-title">Compare players</h1>
        <div className="challenges-pickers">
          <label>
            <span className="challenges-side-a">Player A</span>
            <input className="input-antique" list="cmp-names" value={a} placeholder="Name…" autoComplete="off" onChange={(e) => choose('a', e.target.value)} />
          </label>
          <span className="muted">vs</span>
          <label>
            <span className="challenges-side-b">Player B</span>
            <input className="input-antique" list="cmp-names" value={b} placeholder="Name…" autoComplete="off" onChange={(e) => choose('b', e.target.value)} />
          </label>
          <datalist id="cmp-names">
            {known.map((p) => (
              <option key={p.player} value={p.player} />
            ))}
          </datalist>
        </div>
      </div>
      <div className="challenges-bar">
        <div className="challenges-bar-group">
          {pa && pb && <Segmented label="Set" look="raised" value={set} onChange={setSet} options={SETS} />}
          {pa && pb && (
            <span className="muted">
              <span className="challenges-side-a">■</span> {pa.player} ahead · <span className="challenges-side-b">■</span> {pb.player} ahead
            </span>
          )}
        </div>
        <div className="challenges-bar-group">
          <MeasureSwitch value={measure} onChange={setMeasure} />
          <VerifiedSwitch value={verified} onChange={setVerified} />
        </div>
      </div>
      {players.kind !== 'ready' && <Waiting load={players} what="the players" />}
      {players.kind === 'ready' &&
        [a, b].map(
          (name, i) =>
            name.trim() !== '' &&
            !find(name) && (
              <p key={i} className="rating-card waiting-message">
                No {v ? 'verified ' : ''}entries for “{name}”.
              </p>
            ),
        )}
      {pa && pb && sideA.kind === 'ready' && sideB.kind === 'ready' && sideA.data && sideB.data && (
        <Comparison a={pa} b={pb} sideA={sideA.data} sideB={sideB.data} list={list} set={set} measure={measure} />
      )}
    </>
  );
}

function missing(e: unknown): null {
  if (e instanceof RatingsApiError && e.status === 404) return null;
  throw e;
}

interface Props {
  a: ChallengePlayerSummary;
  b: ChallengePlayerSummary;
  sideA: ChallengePlayer;
  sideB: ChallengePlayer;
  list: ChallengeTile[];
  set: SetId;
  measure: Measure;
}

function Comparison({ a, b, sideA, sideB, list, set, measure }: Props) {
  const score = compareBests(sideA.bests, sideB.bests);
  const bestA = new Map(sideA.bests.map((x) => [x.challenge, x.domination]));
  const bestB = new Map(sideB.bests.map((x) => [x.challenge, x.domination]));
  const groups = challengeSets(list).find((s) => s.id === set)?.groups ?? [];
  const diff = (x: number, y: number) => (measure === 'rating' ? Math.round((x - y) * 10) / 10 : x - y);
  return (
    <>
      <div className="challenges-stats">
        <Stat label={`${a.player} wins`} value={score.winsA} side="a" />
        <Stat label={`${b.player} wins`} value={score.winsB} side="b" />
        <Stat label="Ties" value={score.ties} />
        <Stat label="Played by both" value={score.both} />
      </div>
      <section className="rating-card challenges-card" aria-label="Totals">
        <div className="table-responsive">
          <table className="leaderboard-table">
            <thead>
              <tr>
                <th scope="col">
                  <span className="visually-hidden">Player</span>
                </th>
                {SETS.map((s) => (
                  <th key={s.value} scope="col" className="num">
                    {setLabel(s.value)}
                  </th>
                ))}
                <th scope="col" className="num">
                  {measure === 'rating' ? 'Rating' : 'Total'}
                </th>
                <th scope="col" className="num">
                  Records
                </th>
              </tr>
            </thead>
            <tbody>
              {[a, b].map((p, i) => (
                <tr key={p.player}>
                  <th scope="row" className={i === 0 ? 'challenges-side-a' : 'challenges-side-b'}>
                    <ChallengePlayerLink name={p.player} />
                  </th>
                  {SETS.map((s) => (
                    <td key={s.value} className="num">
                      {formatMeasure(measureOf(p, measure, s.value), measure)}
                    </td>
                  ))}
                  <td className="num challenges-score">{formatMeasure(measureOf(p, measure), measure)}</td>
                  <td className="num">{p.records}</td>
                </tr>
              ))}
              <tr>
                <th scope="row">Difference</th>
                {SETS.map((s) => (
                  <td key={s.value} className="num">
                    <Diff value={diff(measureOf(a, measure, s.value), measureOf(b, measure, s.value))} suffix={measure === 'rating' ? '%' : ''} />
                  </td>
                ))}
                <td className="num">
                  <Diff value={diff(measureOf(a, measure), measureOf(b, measure))} suffix={measure === 'rating' ? '%' : ''} />
                </td>
                <td className="num">
                  <Diff value={a.records - b.records} />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
      <section className="rating-card challenges-card" aria-labelledby="per-challenge">
        <h2 id="per-challenge" className="rating-card-heading">
          Per challenge
        </h2>
        <SetGrid
          groups={groups}
          cell={(c) => {
            const x = bestA.get(c.id);
            const y = bestB.get(c.id);
            if (x === undefined && y === undefined) return <span className="muted">—</span>;
            const lead = x === undefined ? 'b' : y === undefined ? 'a' : x > y ? 'a' : y > x ? 'b' : '';
            return (
              <span className={lead ? `challenges-lead-${lead}` : undefined}>
                <span className="challenges-side-a">{x ?? '—'}</span>
                <span className="muted"> : </span>
                <span className="challenges-side-b">{y ?? '—'}</span>
                {x !== undefined && y !== undefined && (
                  <span>
                    {' '}
                    (<Diff value={x - y} />)
                  </span>
                )}
              </span>
            );
          }}
        />
      </section>
    </>
  );
}

function Stat({ label, value, side }: { label: string; value: number; side?: 'a' | 'b' }) {
  return (
    <div className="challenges-stat">
      <div className={side ? `challenges-stat-value challenges-side-${side}` : 'challenges-stat-value'}>{value}</div>
      <div className="challenges-stat-label">{label}</div>
    </div>
  );
}

/** Diff is a difference, in A's colour when A leads and B's when B does. */
function Diff({ value, suffix = '' }: { value: number; suffix?: string }) {
  const side = value > 0 ? 'challenges-side-a' : value < 0 ? 'challenges-side-b' : 'muted';
  return (
    <span className={side}>
      {signed(value)}
      {suffix}
    </span>
  );
}
