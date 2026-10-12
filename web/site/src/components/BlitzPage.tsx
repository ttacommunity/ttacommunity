/** The Blitz section: every blitz edition, or one edition (?t=&e=) with the entrants still in. */
import { useEffect, useState } from 'react';
import { RatingsApiError, blitzEdition, blitzEditions, type BlitzEdition, type BlitzEditions } from '@ttac/shared/ratings';
import { formatLabel, zoneLabel } from '@ttac/shared/blitz';
import { editionHref, editionParams, ratingText } from '../lib/blitz';
import Waiting from './Waiting';
import './Blitz.css';

type Load<T> = { kind: 'loading' } | { kind: 'missing' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };

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
  return (
    <>
      <div className="tournaments-header">
        <h1 className="tournaments-title">
          <span>⚡</span> {ed.name} {ed.edition}
        </h1>
        <div className="tournaments-subtitle">{formatLabel(ed.format)}. Pairings, results and standings appear here as the rounds are played.</div>
      </div>
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
                    <th scope="row">{e.name}</th>
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
