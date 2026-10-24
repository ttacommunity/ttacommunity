/** The Blitz section: every blitz edition, or one edition (?t=&e=) with the signed-in player's own game, its knockout and champion, then tabs for its standings, its Swiss games (the games list's table) and the entrants still in. */
import { useEffect, useState, type ReactNode } from 'react';
import { RatingsApiError, blitzEdition, blitzEditions, type BlitzEdition, type BlitzEditions } from '@ttac/shared/ratings';
import type { components } from '@ttac/shared/api';
import { formatLabel, reachedLabel, scoringSentence, zoneLabel } from '@ttac/shared/blitz';
import BackLink from '@ttac/shared/BackLink';
import SubTabs, { useHash } from '@ttac/shared/SubTabs';
import { hashTab } from '@ttac/shared/tabs';
import { blitzGames, editionHref, editionParams, editionViews, notPlaying, ratingText, resultsUpdated, roundOptions, splitRounds, type BlitzGamesFilter, type EditionView } from '../lib/blitz';
import BlitzReport from './BlitzReport';
import { yourRow, type MyBlitzMatch } from '../lib/blitz-report';
import type { YourRow } from '../lib/blitz';
import { gamesTag } from '../lib/games-table';
import GamesTable, { GamesPages } from './GamesTable';
import PlayerPicker from './PlayerPicker';
import { catalogueHref } from '../lib/catalogue-view';
import { Flag } from './PlayerMarks';
import Waiting from './Waiting';
import './Blitz.css';

type Load<T> = { kind: 'loading' } | { kind: 'missing' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };
type BlitzRound = components['schemas']['BlitzRound'];
type BlitzStanding = components['schemas']['BlitzStanding'];

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
  const [hash, setHash] = useHash();
  // Asked again after the player reports, so their result shows at once.
  const [asked, setAsked] = useState(0);
  const [mine, setMine] = useState<MyBlitzMatch | null>(null);
  useEffect(() => {
    blitzEdition(apiUrl, slug, edition)
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad(failed(e)));
  }, [apiUrl, slug, edition, asked]);
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
  const views = editionViews(ed.rounds);
  const panels: Record<EditionView, { label: string; panel: ReactNode }> = {
    standings: { label: 'Standings', panel: <StandingsPanel standings={ed.standings} scoring={scoringSentence(ed.points, ed.fide_points)} /> },
    pairings: { label: 'Pairings', panel: <PairingsPanel ed={ed} swiss={swiss} you={yourRow(mine)} /> },
    players: { label: 'Players', panel: <PlayersPanel entrants={ed.entrants} /> },
  };
  return (
    <>
      <div className="tournaments-header">
        <div className="header-back">
          <BackLink href="/blitz/">Every blitz tournament</BackLink>
        </div>
        <h1 className="tournaments-title">
          <span>⚡</span> {ed.name} {ed.edition}
        </h1>
        <div className="tournaments-subtitle">
          {formatLabel(ed.format)}. {reachedLabel(latest, ed.format, ed.champion)}. {resultsUpdated(ed.results_updated_at, viewerZone())}
        </div>
      </div>
      <BlitzReport apiUrl={apiUrl} slug={slug} edition={edition} onChanged={() => setAsked((n) => n + 1)} onMatch={setMine} />
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
          <GamesTable
            caption="The knockout's games, first round to final"
            context="Round"
            rows={blitzGames(knockout, ed.format, { round: null, player: '', coded: false, page: 1 }, viewerZone(), yourRow(mine)).rows}
            empty="No knockout games yet."
          />
        </section>
      )}
      <section className="rating-card blitz-card" aria-label="Standings, pairings and players">
        <SubTabs label="Show" tabs={views.map((v) => ({ value: v, ...panels[v] }))} chosen={hashTab(views, hash)} onChoose={setHash} />
      </section>
    </>
  );
}

/** StandingsPanel shows the standings, with how the edition scores and how ties are broken. */
function StandingsPanel({ standings, scoring }: { standings: BlitzStanding[]; scoring: string }) {
  return (
    <section aria-labelledby="blitz-standings">
      <h2 id="blitz-standings" className="rating-card-heading">
        Standings
      </h2>
      <p className="blitz-intro">
        {scoring} Ties are broken by the Swiss tie-breaks: Direct Encounter (the games
        among the tied players), then Buchholz (the sum of the opponents&apos; scores; a bye, a forfeit or a missed round
        counts as a game against an opponent on the player&apos;s own score), then a draw fixed for the edition.
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
                Played
              </th>
              <th scope="col" className="num">
                Byes
              </th>
              <th scope="col" className="num">
                Buchholz
              </th>
            </tr>
          </thead>
          <tbody>
            {standings.map((s) => (
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
                <td className="num">{s.buchholz}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

/** PlayersPanel shows the entrants still in, in sign-up order, with their time zones and ratings. */
function PlayersPanel({ entrants }: { entrants: BlitzEdition['entrants'] }) {
  return (
    <section aria-labelledby="blitz-entrants">
      <h2 id="blitz-entrants" className="rating-card-heading">
        Players
      </h2>
      <p className="blitz-intro">{entrants.length} in. Ratings are the 2-player community rating of the current run; a dash means not rated yet.</p>
      {entrants.length > 0 && (
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
              {entrants.map((e) => (
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
  );
}

/** viewerZone is the browser's time zone, whose clocks the games' dates are on. */
function viewerZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone;
}

/** PairingsPanel lists the Swiss rounds' games as the games list does: narrowed by round (the latest at first), player and replay code, a page at a time, the signed-in player's game marked. */
function PairingsPanel({ ed, swiss, you }: { ed: BlitzEdition; swiss: BlitzRound[]; you: YourRow | null }) {
  const [filter, setFilter] = useState<BlitzGamesFilter>({ round: swiss[0]?.number ?? null, player: '', coded: false, page: 1 });
  const narrow = (next: Partial<BlitzGamesFilter>) => setFilter((f) => ({ ...f, ...next, page: next.page ?? 1 }));
  const zone = viewerZone();
  const games = blitzGames(swiss, ed.format, filter, zone, you);
  return (
    <section aria-labelledby="blitz-rounds">
      <div className="panel-header panel-header-split">
        <h2 id="blitz-rounds" className="panel-header-title">
          Pairings and results
        </h2>
        <span className="panel-tag">{gamesTag(games.games, games.coded, filter.coded)}</span>
      </div>
      <div className="games-filters">
        <label>
          <span>Round</span>
          <select className="input-antique" id="games-round" value={filter.round ?? ''} onChange={(e) => narrow({ round: e.target.value === '' ? null : Number(e.target.value) })}>
            <option value="">All rounds</option>
            {roundOptions(swiss, ed.format).map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </label>
        <PlayerPicker players={ed.entrants.map((e) => e.name)} value={filter.player} onPick={(player) => narrow({ player })} />
        <label className="games-coded check-inline">
          <input id="games-coded" type="checkbox" checked={filter.coded} onChange={(e) => narrow({ coded: e.target.checked })} />
          <span>Only games with a replay code</span>
        </label>
      </div>
      <p className="records-intro">
        Who plays whom and how each game went, ratings beside the names. Arrange your game with your opponent on Discord. Dates are on your clocks ({zone}). Click a
        code to copy it. A recorded game is on the <a href={catalogueHref(ed.tournament)}>tournament&apos;s page</a> too.
      </p>
      <GamesTable caption="The games these filters keep, latest round first" context="Round" rows={games.rows} empty="No games match these filters." />
      <GamesPages page={games.page} pages={games.pages} onPage={(page) => narrow({ page })} />
      {notPlaying(swiss, filter.round, ed.format).map((r) => (
        <p key={r.round} className="blitz-intro">
          Not playing in {r.round}: {r.names.join(', ')}.
        </p>
      ))}
    </section>
  );
}
