/** The Blitz section: every blitz edition, or one edition (?t=&e=) with the signed-in player's own game, its knockout and champion, then tabs for its standings, its Swiss games (the games list's table) and the entrants still in. */
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { blitzEdition, blitzEditions, type BlitzEdition, type BlitzEditions } from '@ttac/shared/ratings';
import { failed, type Found } from '@ttac/shared/load';
import { viewerZone } from '@ttac/shared/time';
import type { components } from '@ttac/shared/api';
import { EDITION_COLUMNS, ENTRANT_COLUMNS, STANDING_COLUMNS, TIE_BREAKS, UNPLAYED_NOTE, drawLabel, editionHref, editionParams, formatLabel, lastTieBreak, reachedLabel, roundLine, scoringLine, startLabel, zoneLabel } from '@ttac/shared/blitz';
import SortHeader, { useSort } from '@ttac/shared/SortHeader';
import { nextSort, type Sort } from '@ttac/shared/sort';
import BackLink from '@ttac/shared/BackLink';
import InfoDialog from '@ttac/shared/InfoDialog';
import SubTabs, { useHash } from '@ttac/shared/SubTabs';
import { hashTab } from '@ttac/shared/tabs';
import { BLITZ_GAME_COLUMNS, blitzGames, editionViews, notPlaying, pairingNotes, ratingText, resultsUpdated, roundOptions, splitRounds, type BlitzGamesFilter, type EditionView } from '../lib/blitz';
import BlitzReport from './BlitzReport';
import { yourRow, type MyBlitzMatch } from '../lib/blitz-report';
import type { YourRow } from '../lib/blitz';
import { gamesTag, type GamesColumn } from '../lib/games-table';
import GamesTable, { GamesPages } from './GamesTable';
import PlayerPicker from './PlayerPicker';
import { catalogueHref } from '../lib/catalogue-view';
import { Flag } from './PlayerMarks';
import Waiting from './Waiting';
import './Blitz.css';

type BlitzRound = components['schemas']['BlitzRound'];
type BlitzStanding = components['schemas']['BlitzStanding'];

/** BlitzPage shows the edition the address names, or the list of every edition. */
export default function BlitzPage({ apiUrl }: { apiUrl: string }) {
  const [at] = useState(() => editionParams(new URLSearchParams(window.location.search)));
  return at === null ? <BlitzList apiUrl={apiUrl} /> : <EditionPage apiUrl={apiUrl} slug={at.slug} edition={at.edition} />;
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
  const [load, setLoad] = useState<Found<BlitzEditions>>({ kind: 'loading' });
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
          {load.data.editions.length > 0 && <EditionsTable editions={load.data.editions} />}
        </section>
      )}
    </>
  );
}

/** EditionsTable lists every blitz edition, newest first, sortable by any column. */
function EditionsTable({ editions }: { editions: BlitzEditions['editions'] }) {
  const [rows, sort, sortBy] = useSort(editions, EDITION_COLUMNS);
  return (
    <div className="blitz-table-wrap">
      <table className="blitz-table">
        <caption>Every blitz tournament, newest edition first</caption>
        <thead>
          <tr>
            <SortHeader col="tournament" sort={sort} onSort={sortBy}>
              Tournament
            </SortHeader>
            <SortHeader col="edition" sort={sort} onSort={sortBy}>
              Edition
            </SortHeader>
            <SortHeader col="format" sort={sort} onSort={sortBy}>
              Format
            </SortHeader>
            <SortHeader col="reached" sort={sort} onSort={sortBy}>
              Reached
            </SortHeader>
            <SortHeader col="players" sort={sort} onSort={sortBy} className="num">
              Players
            </SortHeader>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
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
  );
}

function EditionPage({ apiUrl, slug, edition }: { apiUrl: string; slug: string; edition: string }) {
  const [load, setLoad] = useState<Found<BlitzEdition>>({ kind: 'loading' });
  const [hash, setHash] = useHash();
  // Asked again after the player reports, so their result shows at once.
  const [asked, setAsked] = useState(0);
  const [mine, setMine] = useState<MyBlitzMatch | null>(null);
  // The dialog opens at its top from the header, or at "How ties are broken" from the standings.
  const [howPaired, setHowPaired] = useState<{ opener: HTMLElement; at: 'top' | 'ties' } | null>(null);
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
    standings: { label: 'Standings', panel: <StandingsPanel standings={ed.standings} scoring={scoringLine(ed.points, ed.fide_points)} fixedBy={ed.ranks_fixed_by} onHow={(e) => setHowPaired({ opener: e.currentTarget, at: 'ties' })} /> },
    pairings: { label: 'Pairings', panel: <PairingsPanel ed={ed} swiss={swiss} you={yourRow(mine)} /> },
    players: { label: 'Players', panel: <PlayersPanel entrants={ed.entrants} start={startLabel(ed.ranks_fixed_by)} /> },
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
        <ul className="blitz-pairing-lines">
          <li>Round 1: {roundLine(ed.pairing.first, ed.pairing.first_order)}</li>
          {ed.format.swiss_rounds > 1 && <li>Later rounds: {roundLine(ed.pairing.later, ed.pairing.later_order)}</li>}
        </ul>
        <button type="button" className="btn-antique blitz-how-paired" onClick={(e) => setHowPaired({ opener: e.currentTarget, at: 'top' })}>
          How pairings and ties are calculated
        </button>
      </div>
      {howPaired !== null && <HowPairedDialog ed={ed} opener={howPaired.opener} at={howPaired.at} onClose={() => setHowPaired(null)} />}
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
          <KnockoutGames rounds={knockout} format={ed.format} you={yourRow(mine)} />
        </section>
      )}
      <section className="rating-card blitz-card" aria-label="Standings, pairings and players">
        <SubTabs label="Show" tabs={views.map((v) => ({ value: v, ...panels[v] }))} chosen={hashTab(views, hash)} onChoose={setHash} />
      </section>
    </>
  );
}

/** HowPairedDialog explains how the edition's rounds are paired (its settings, and the rules every method keeps) and how ties in the standings are broken; opened at the top, or at the ties (at). */
function HowPairedDialog({ ed, opener, at, onClose }: { ed: BlitzEdition; opener: HTMLElement; at: 'top' | 'ties'; onClose: () => void }) {
  const notes = pairingNotes(ed);
  const ties = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (at === 'ties') {
      ties.current?.scrollIntoView();
      ties.current?.focus();
    }
  }, [at]);
  return (
    <InfoDialog title="How pairings and ties are calculated" opener={opener} onClose={onClose}>
      {notes.methods.map((m) => (
        <p key={m.when} className="site-dialog-text">
          <strong>
            {m.when}: {m.label}.
          </strong>{' '}
          {m.about}
        </p>
      ))}
      {notes.rules.map((r) => (
        <p key={r} className="site-dialog-text">
          {r}
        </p>
      ))}
      <p className="site-dialog-text" ref={ties} tabIndex={-1}>
        <strong>How ties are broken.</strong> Players on the same points are separated by these steps, each only when the
        ones before leave them level:
      </p>
      <ol className="site-dialog-text blitz-paired-list">
        {TIE_BREAKS.map((t) => (
          <li key={t.label}>
            <strong>{t.label}:</strong> {t.about}
          </li>
        ))}
      </ol>
      <p className="site-dialog-text">{UNPLAYED_NOTE}</p>
    </InfoDialog>
  );
}

/** StandingsPanel shows the standings by rank, with how the edition scores, a marker before the ranks the lot, the seed or the draw ordered (fixedBy: what fixed the starting ranks, null before round 1), the starting ranks once fixed, the tie-breaks on request, and a button to how ties are broken (onHow). */
function StandingsPanel({ standings, scoring, fixedBy, onHow }: { standings: BlitzStanding[]; scoring: string; fixedBy: 'rating' | 'lot' | null; onHow: (e: React.MouseEvent<HTMLButtonElement>) => void }) {
  const [rows, sort, sortBy] = useSort(standings, STANDING_COLUMNS);
  const [tieBreaks, setTieBreaks] = useState(false);
  const start = startLabel(fixedBy);
  return (
    <section aria-labelledby="blitz-standings">
      <h2 id="blitz-standings" className="rating-card-heading">
        Standings
      </h2>
      <div className="blitz-table-bar">
        <span className="blitz-scoring">{scoring}</span>
        <span className="blitz-table-controls">
          <label className="blitz-toggle">
            <input type="checkbox" checked={tieBreaks} onChange={(e) => setTieBreaks(e.target.checked)} /> Show tie-breaks
          </label>
          <button type="button" className="blitz-text-button" onClick={onHow}>
            How ties are broken
          </button>
        </span>
      </div>
      <div className="blitz-table-wrap">
        <table className="blitz-table">
          <caption>The standings after the results recorded so far</caption>
          <thead>
            <tr>
              <SortHeader col="rank" sort={sort} onSort={sortBy} className="num">
                Rank
              </SortHeader>
              <SortHeader col="player" sort={sort} onSort={sortBy}>
                Player
              </SortHeader>
              <SortHeader col="points" sort={sort} onSort={sortBy} className="num">
                Points
              </SortHeader>
              <SortHeader col="played" sort={sort} onSort={sortBy} className="num">
                Played
              </SortHeader>
              <SortHeader col="byes" sort={sort} onSort={sortBy} className="num">
                Byes
              </SortHeader>
              {tieBreaks && (
                <>
                  <SortHeader col="cut1" sort={sort} onSort={sortBy} className="num">
                    <abbr title="Buchholz Cut 1">BH-C1</abbr>
                  </SortHeader>
                  <SortHeader col="buchholz" sort={sort} onSort={sortBy} className="num">
                    <abbr title="Buchholz">BH</abbr>
                  </SortHeader>
                  <SortHeader col="sb" sort={sort} onSort={sortBy} className="num">
                    <abbr title="Sonneborn-Berger">SB</abbr>
                  </SortHeader>
                </>
              )}
              {start !== null && (
                <SortHeader col="start" sort={sort} onSort={sortBy} className="num">
                  {start}
                </SortHeader>
              )}
            </tr>
          </thead>
          <tbody>
            {rows.map((s) => (
              <tr key={s.name} className={s.status === 'dropped' ? 'blitz-dropped' : ''}>
                <td className="num">
                  <DrawMark s={s} fixedBy={fixedBy} />
                  {s.rank}
                </td>
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
                {tieBreaks && (
                  <>
                    <td className="num">{s.buchholz_cut1}</td>
                    <td className="num">{s.buchholz}</td>
                    <td className="num">{s.sonneborn_berger}</td>
                  </>
                )}
                {start !== null && <td className="num">{s.starting_rank ?? '–'}</td>}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {standings.some((s) => s.place_from !== s.place_to) && (
        <p className="blitz-footnote">
          <span className="blitz-draw">*</span> Equal on every tie-break; ordered by {lastTieBreak(fixedBy)}.
        </p>
      )}
    </section>
  );
}

/** DrawMark marks a rank the lot, the seed or the draw ordered, before the number in a slot of its own so the numbers line up, with the ranks it was ordered among as its text; the slot stays empty for a rank held alone. */
function DrawMark({ s, fixedBy }: { s: BlitzStanding; fixedBy: 'rating' | 'lot' | null }) {
  const label = drawLabel(s, fixedBy);
  return (
    <span className="blitz-draw-slot">
      {label !== null && (
        <span className="blitz-draw" title={label}>
          *<span className="visually-hidden">{label}</span>
        </span>
      )}
    </span>
  );
}

/** PlayersPanel shows the entrants still in, in sign-up order, with their time zones, ratings and, once round 1 fixed them, starting ranks (start: the column's heading). */
function PlayersPanel({ entrants, start }: { entrants: BlitzEdition['entrants']; start: string | null }) {
  const [rows, sort, sortBy] = useSort(entrants, ENTRANT_COLUMNS);
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
                <SortHeader col="player" sort={sort} onSort={sortBy}>
                  Player
                </SortHeader>
                <SortHeader col="zone" sort={sort} onSort={sortBy}>
                  Time zone
                </SortHeader>
                <SortHeader col="rating" sort={sort} onSort={sortBy} className="num">
                  Rating
                </SortHeader>
                {start !== null && (
                  <SortHeader col="start" sort={sort} onSort={sortBy} className="num">
                    {start}
                  </SortHeader>
                )}
              </tr>
            </thead>
            <tbody>
              {rows.map((e) => (
                <tr key={e.name}>
                  <th scope="row">
                    <span className="blitz-flag">
                      <Flag country={e.country} />
                    </span>
                    {e.name}
                  </th>
                  <td>{zoneLabel(e.zone, e.utc_offset)}</td>
                  <td className="num">{ratingText(e.rating)}</td>
                  {start !== null && <td className="num">{e.starting_rank ?? '–'}</td>}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

/** KnockoutGames lists the knockout's games, first round to final, sortable by any column. */
function KnockoutGames({ rounds, format, you }: { rounds: BlitzRound[]; format: BlitzEdition['format']; you: YourRow | null }) {
  const [sort, setSort] = useState<Sort<GamesColumn>>(null);
  const games = blitzGames(rounds, format, { round: null, player: '', coded: false, page: 1 }, viewerZone(), you, sort);
  return (
    <GamesTable
      caption="The knockout's games, first round to final"
      context="Round"
      rows={games.rows}
      empty="No knockout games yet."
      sort={sort}
      onSort={(col) => setSort((s) => nextSort(s, col, BLITZ_GAME_COLUMNS))}
    />
  );
}


/** PairingsPanel lists the Swiss rounds' games as the games list does: narrowed by round (the latest at first), player and replay code, a page at a time, the signed-in player's game marked. */
function PairingsPanel({ ed, swiss, you }: { ed: BlitzEdition; swiss: BlitzRound[]; you: YourRow | null }) {
  const [filter, setFilter] = useState<BlitzGamesFilter>({ round: swiss[0]?.number ?? null, player: '', coded: false, page: 1 });
  const [sort, setSort] = useState<Sort<GamesColumn>>(null);
  const narrow = (next: Partial<BlitzGamesFilter>) => setFilter((f) => ({ ...f, ...next, page: next.page ?? 1 }));
  const sortBy = (col: GamesColumn) => {
    setSort((s) => nextSort(s, col, BLITZ_GAME_COLUMNS));
    narrow({});
  };
  const zone = viewerZone();
  const games = blitzGames(swiss, ed.format, filter, zone, you, sort);
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
      <GamesTable caption="The games these filters keep, latest round first" context="Round" rows={games.rows} empty="No games match these filters." sort={sort} onSort={sortBy} />
      <GamesPages page={games.page} pages={games.pages} onPage={(page) => narrow({ page })} />
      {notPlaying(swiss, filter.round, ed.format).map((r) => (
        <p key={r.round} className="blitz-intro">
          Not playing in {r.round}: {r.names.join(', ')}.
        </p>
      ))}
    </section>
  );
}
