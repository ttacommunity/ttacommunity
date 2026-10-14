/** The Blitz section: every blitz edition, or one edition (?t=&e=) with its knockout and champion, its standings, its Swiss pairings and results, and the entrants still in. */
import { useEffect, useState } from 'react';
import { RatingsApiError, blitzEdition, blitzEditions, type BlitzEdition, type BlitzEditions } from '@ttac/shared/ratings';
import type { components } from '@ttac/shared/api';
import { formatLabel, reachedLabel, resultSummary, roundShown, roundSummary, roundTitle, seededName, zoneLabel, type BlitzFormat } from '@ttac/shared/blitz';
import { editionHref, editionParams, ratingText, splitRounds } from '../lib/blitz';
import { catalogueHref } from '../lib/catalogue-view';
import { Flag } from './PlayerMarks';
import Waiting from './Waiting';
import './Blitz.css';

type Load<T> = { kind: 'loading' } | { kind: 'missing' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };
type BlitzRound = components['schemas']['BlitzRound'];

/** BlitzPage shows the edition the address names, or the list of every edition. */
export default function BlitzPage({ apiUrl }: { apiUrl: string }) {
  const [at] = useState(() => editionParams(new URLSearchParams(window.location.search)));
  return at === null ? <BlitzList apiUrl={apiUrl} /> : <EditionPage apiUrl={apiUrl} slug={at.slug} edition={at.edition} />;
}

/** failed turns a failed load into the page's state: a 404 is an edition the site does not show. */
function failed(e: unknown): Load<never> {
  if (e instanceof RatingsApiError && e.status === 404) return { kind: 'missing' };
  return { kind: 'error', message: e instanceof Error ? e.message : String(e) };
}

function Header() {
  return (
    <div className="tournaments-header">
      <h1 className="tournaments-title">
        <span>⚡</span> Blitz
      </h1>
      <div className="tournaments-subtitle">
        Blitz tournaments run by the community&apos;s organisers: Super Blitz games on CGE, a round a week. Who is in,
        who plays whom, the results and the standings, as the organisers record them.
      </div>
    </div>
  );
}

function BlitzList({ apiUrl }: { apiUrl: string }) {
  const [load, setLoad] = useState<Load<BlitzEditions>>({ kind: 'loading' });
  useEffect(() => {
    blitzEditions(apiUrl)
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad(failed(e)));
  }, [apiUrl]);
  return (
    <>
      <Header />
      {load.kind !== 'ready' ? (
        <Waiting load={load} what="the blitz tournaments" />
      ) : (
        <section className="rating-card blitz-card" aria-labelledby="blitz-editions">
          <h2 id="blitz-editions" className="rating-card-heading">
            Tournaments
          </h2>
          {load.data.editions.length === 0 && <p className="muted">No blitz tournament is running yet.</p>}
          {load.data.editions.length > 0 && (
            <div className="blitz-table-wrap">
              <table className="blitz-table">
                <caption>Every blitz tournament, newest edition first</caption>
                <thead>
                  <tr>
                    <th scope="col">Tournament</th>
                    <th scope="col">Edition</th>
                    <th scope="col">Format</th>
                    <th scope="col">Reached</th>
                    <th scope="col" className="num">
                      Players
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {load.data.editions.map((r) => (
                    <tr key={`${r.tournament}/${r.edition}`}>
                      <th scope="row">
                        <a href={editionHref(r.tournament, r.edition)}>{r.name}</a>
                      </th>
                      <td>{r.edition}</td>
                      <td>{formatLabel(r.format)}</td>
                      <td>{reachedLabel(r.round, r.format, r.champion)}</td>
                      <td className="num">{r.entrants}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      )}
    </>
  );
}

function EditionPage({ apiUrl, slug, edition }: { apiUrl: string; slug: string; edition: string }) {
  const [load, setLoad] = useState<Load<BlitzEdition>>({ kind: 'loading' });
  // toggled holds the rounds whose Show or Hide button was pressed: only the latest Swiss and the latest knockout
  // round show their matches until then.
  const [toggled, setToggled] = useState<ReadonlySet<number>>(new Set());
  useEffect(() => {
    blitzEdition(apiUrl, slug, edition)
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad(failed(e)));
  }, [apiUrl, slug, edition]);
  if (load.kind === 'missing') {
    return (
      <>
        <Header />
        <div className="rating-card waiting-message">
          No blitz tournament has that edition. <a href="/blitz/">See every blitz tournament</a>.
        </div>
      </>
    );
  }
  if (load.kind !== 'ready') {
    return (
      <>
        <Header />
        <Waiting load={load} what="this edition" />
      </>
    );
  }
  const ed = load.data;
  const { swiss, knockout } = splitRounds(ed.rounds);
  const latest = ed.rounds.length === 0 ? null : Math.max(...ed.rounds.map((r) => r.number));
  const countryOf = new Map(ed.standings.map((s) => [s.name, s.country]));
  const toggle = (n: number) => setToggled((t) => (t.has(n) ? new Set([...t].filter((x) => x !== n)) : new Set([...t, n])));
  return (
    <>
      <div className="tournaments-header">
        <h1 className="tournaments-title">
          <span>⚡</span> {ed.name} {ed.edition}
        </h1>
        <div className="tournaments-subtitle">
          {formatLabel(ed.format)}. {reachedLabel(latest, ed.format, ed.champion)}.
        </div>
      </div>
      {ed.rounds.length === 0 && (
        <section className="rating-card blitz-card" aria-labelledby="blitz-not-started">
          <h2 id="blitz-not-started" className="rating-card-heading">
            Pairing not started
          </h2>
          <p className="blitz-intro">Round 1&apos;s pairings, the results and the standings appear here once the organisers publish them.</p>
        </section>
      )}
      {knockout.length > 0 && (
        <section className="rating-card blitz-card" aria-labelledby="blitz-knockout">
          <h2 id="blitz-knockout" className="rating-card-heading">
            Knockout
          </h2>
          {ed.champion !== null && (
            <p className="blitz-champion">
              Champion:{' '}
              <span className="blitz-flag">
                <Flag country={countryOf.get(ed.champion) ?? null} />
              </span>
              <strong>{ed.champion}</strong>
            </p>
          )}
          <p className="blitz-intro">
            The top {ed.format.knockout_size} after the Swiss rounds, seeded by the standings: the best seed meets the worst,
            and the winners of neighbouring matches meet next, through to the final. A drawn game sends the higher seed
            through.
          </p>
          {knockout.map((r) => (
            <RoundTable
              key={r.number}
              round={r}
              format={ed.format}
              shown={roundShown(r.number, knockout[knockout.length - 1].number, toggled)}
              onToggle={() => toggle(r.number)}
            />
          ))}
        </section>
      )}
      {ed.rounds.length > 0 && (
        <section className="rating-card blitz-card" aria-labelledby="blitz-standings">
          <h2 id="blitz-standings" className="rating-card-heading">
            Standings
          </h2>
          <p className="blitz-intro">
            A win is 1 point, a draw a half, a bye 1. Players on equal points who have all met are ordered by the games among
            them, then by their opponents&apos; points, then by a draw fixed for the edition.
          </p>
          <div className="blitz-table-wrap">
            <table className="blitz-table">
              <caption>The standings after the results recorded so far</caption>
              <thead>
                <tr>
                  <th scope="col" className="num">
                    Rank
                  </th>
                  <th scope="col">Player</th>
                  <th scope="col" className="num">
                    Points
                  </th>
                  <th scope="col" className="num">
                    Games
                  </th>
                  <th scope="col" className="num">
                    Byes
                  </th>
                  <th scope="col" className="num">
                    Opponents&apos; points
                  </th>
                </tr>
              </thead>
              <tbody>
                {ed.standings.map((s) => (
                  <tr key={s.name} className={s.status === 'dropped' ? 'blitz-dropped' : ''}>
                    <td className="num">{s.rank}</td>
                    <th scope="row">
                      <span className="blitz-flag">
                        <Flag country={s.country} />
                      </span>
                      {s.name}
                      {s.status === 'dropped' && <span className="blitz-note">dropped</span>}
                    </th>
                    <td className="num">{s.points}</td>
                    <td className="num">{s.games}</td>
                    <td className="num">{s.byes}</td>
                    <td className="num">{s.opponents_points}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
      {swiss.length > 0 && (
        <section className="rating-card blitz-card" aria-labelledby="blitz-rounds">
          <h2 id="blitz-rounds" className="rating-card-heading">
            Pairings and results
          </h2>
          <p className="blitz-intro">
            Who plays whom, the latest round first, and how each game went. Arrange your game with your opponent on Discord.
            A recorded game is on the <a href={catalogueHref(ed.tournament)}>tournament&apos;s page</a> too.
          </p>
          {swiss.map((r) => (
            <RoundTable
              key={r.number}
              round={r}
              format={ed.format}
              shown={roundShown(r.number, swiss[0].number, toggled)}
              onToggle={() => toggle(r.number)}
            />
          ))}
        </section>
      )}
      <section className="rating-card blitz-card" aria-labelledby="blitz-entrants">
        <a className="blitz-back" href="/blitz/">
          ← Every blitz tournament
        </a>
        <h2 id="blitz-entrants" className="rating-card-heading">
          Players
        </h2>
        <p className="blitz-intro">
          {ed.entrants.length} in. Ratings are the 2-player community rating of the current run; a dash means not rated yet.
        </p>
        {ed.entrants.length > 0 && (
          <div className="blitz-table-wrap">
            <table className="blitz-table">
              <caption>The players still in, in sign-up order</caption>
              <thead>
                <tr>
                  <th scope="col">Player</th>
                  <th scope="col">Time zone</th>
                  <th scope="col" className="num">
                    Rating
                  </th>
                </tr>
              </thead>
              <tbody>
                {ed.entrants.map((e) => (
                  <tr key={e.name}>
                    <th scope="row">
                      <span className="blitz-flag">
                        <Flag country={e.country} />
                      </span>
                      {e.name}
                    </th>
                    <td>{zoneLabel(e.zone, e.utc_offset)}</td>
                    <td className="num">{ratingText(e.rating)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </>
  );
}

/** RoundTable shows one round's pairings and results, knockout players with their seeds. */
type RoundTableProps = { round: BlitzRound; format: BlitzFormat; shown: boolean; onToggle: () => void };

/** RoundTable shows a round's title and summary, and its matches while shown. */
function RoundTable({ round: r, format, shown, onToggle }: RoundTableProps) {
  const title = roundTitle(r.number, format);
  return (
    <div className="blitz-round">
      <h3>{title}</h3>
      <p className="blitz-round-summary">
        <span className="muted">{roundSummary(r.matches)}</span>
        <button type="button" className="btn-antique blitz-round-toggle" aria-expanded={shown} onClick={onToggle}>
          {shown ? 'Hide matches' : 'Show matches'}
        </button>
      </p>
      {shown && (
        <div className="blitz-table-wrap">
          <table className="blitz-table">
            <caption>{title}: pairings and results</caption>
            <thead>
              <tr>
                <th scope="col">Player</th>
                <th scope="col" className="num">
                  Rating
                </th>
                <th scope="col">Opponent</th>
                <th scope="col" className="num">
                  Rating
                </th>
                <th scope="col">Result</th>
              </tr>
            </thead>
            <tbody>
              {r.matches.map((m) => (
                <tr key={`${m.a}-${m.b ?? 'bye'}`}>
                  <th scope="row">{seededName(m.a, m.a_seed)}</th>
                  <td className="num">{ratingText(m.a_rating)}</td>
                  <td>{m.b === null ? <em>bye</em> : seededName(m.b, m.b_seed)}</td>
                  <td className="num">{m.b === null ? '' : ratingText(m.b_rating)}</td>
                  <td>
                    {m.b === null ? '' : m.result === null ? <span className="muted">to be played</span> : resultSummary(m)}
                    {m.game !== null && <span className="blitz-note">{m.game.share_code}</span>}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
