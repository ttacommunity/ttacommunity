/** The Blitz section: every blitz edition, or one edition (?t=&e=) with its sign-up box while it takes entries, the signed-in player's own game, its knockout and champion, then tabs for its standings, its Swiss games (the games list's table) and the entrants still in. */
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { blitzEdition, blitzEditions, revalidating, type BlitzEdition, type BlitzEditions } from '@ttac/shared/ratings';
import { viewerZone } from '@ttac/shared/time';
import type { components } from '@ttac/shared/api';
import { EDITION_COLUMNS, ENTRANT_COLUMNS, TIE_BREAKS, UNPLAYED_NOTE, editionHref, editionParams, formatLabel, lastTieBreak, reachedLabel, roundLine, scoringLine, startLabel, zoneLabel } from '@ttac/shared/blitz';
import SortHeader, { useSort } from '@ttac/shared/SortHeader';
import { nextSort, type Sort } from '@ttac/shared/sort';
import Sheet from '@ttac/shared/Sheet';
import SubTabs, { useHash } from '@ttac/shared/SubTabs';
import { hashTab } from '@ttac/shared/tabs';
import { BLITZ_GAME_COLUMNS, asksToSignIn, blitzGames, knockoutRule, editionViews, notPlaying, pairingNotes, ratingText, resultsUpdated, roundOptions, splitRounds, type BlitzGamesFilter, type EditionView } from '../lib/blitz';
import BlitzReport from './BlitzReport';
import { yourRow, type MyBlitz } from '../lib/blitz-report';
import { TTA_DISCORD } from '../lib/links';
import SignInNote from './SignInNote';
import SignUpBox from './SignUpBox';
import type { YourRow } from '../lib/blitz';
import { gamesTag, type GamesColumn } from '../lib/games-table';
import GamesTable from './GamesTable';
import NewerOlder from '@ttac/shared/NewerOlder';
import BlitzStandingsTable from '@ttac/shared/BlitzStandingsTable';
import PlayerPicker from './PlayerPicker';
import { catalogueHref } from '../lib/catalogue-view';
import { bracketHref } from '../lib/bracket';
import { Flag } from './PlayerMarks';
import Waiting from './Waiting';
import './Blitz.css';
import Message from './Message';
import { useFound, useKeptFound } from './useLoad';
import PageHeader from '@ttac/shared/PageHeader';
import Panel from './Panel';
import PanelHeader from './PanelHeader';
import Panels from './Panels';
import ScrollTable from './ScrollTable';

type BlitzRound = components['schemas']['BlitzRound'];
type BlitzStanding = components['schemas']['BlitzStanding'];

/** BlitzPage shows the edition the address names, or the list of every edition. */
export default function BlitzPage({ apiUrl }: { apiUrl: string }) {
  const [at] = useState(() => editionParams(new URLSearchParams(window.location.search)));
  return at === null ? <BlitzList apiUrl={apiUrl} /> : <EditionPage apiUrl={apiUrl} slug={at.slug} edition={at.edition} />;
}

function Header() {
  return (
    <PageHeader
      icon="⚡"
      title="Blitz"
      subtitle="Blitz tournaments run by the community's organisers: Super Blitz games on CGE, a round a week. Who is in, who plays whom, the results and the standings, as the organisers record them."
    />
  );
}

function BlitzList({ apiUrl }: { apiUrl: string }) {
  const load = useFound('blitz', () => blitzEditions(apiUrl));
  return (
    <>
      <Header />
      {load.kind !== 'ready' ? (
        <Waiting load={load} what="the blitz tournaments" />
      ) : (
        <Panel id="blitz-editions" title="Tournaments">
          {load.data.editions.length === 0 && <p className="muted">No blitz tournament is running yet.</p>}
          {load.data.editions.length > 0 && <EditionsTable editions={load.data.editions} />}
        </Panel>
      )}
    </>
  );
}

/** EditionsTable lists every blitz edition, newest first, sortable by any column. */
function EditionsTable({ editions }: { editions: BlitzEditions['editions'] }) {
  const [rows, sort, sortBy] = useSort(editions, EDITION_COLUMNS);
  return (
    <ScrollTable label="Blitz tournaments">
      <table className="leaderboard-table blitz-table">
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
            <SortHeader col="players" sort={sort} onSort={sortBy} align="end">
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
    </ScrollTable>
  );
}

function EditionPage({ apiUrl, slug, edition }: { apiUrl: string; slug: string; edition: string }) {
  const [hash, setHash] = useHash();
  // Asked again after the player reports, so their result shows at once.
  const [asked, setAsked] = useState(0);
  // What the api says to this viewer: undefined while asking, null for nobody signed in.
  const [mine, setMine] = useState<MyBlitz | null | undefined>(undefined);
  // The dialog opens at its top from the header, or at "How ties are broken" from the standings.
  const [howPaired, setHowPaired] = useState<{ opener: HTMLElement; at: 'top' | 'ties' } | null>(null);
  // After the player's own change, read past the browser's cached copy, so the page shows it.
  const load = useKeptFound(JSON.stringify([slug, edition, asked]), () => blitzEdition(apiUrl, slug, edition, asked > 0 ? revalidating() : fetch));
  if (load.kind === 'missing') {
    return (
      <>
        <Header />
        <Message>
          No blitz tournament has that edition. <a href="/blitz/">See every blitz tournament</a>.
        </Message>
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
    pairings: { label: 'Pairings', panel: <PairingsPanel ed={ed} swiss={swiss} you={yourRow(mine?.match ?? null)} reports={mine?.reports ?? []} signIn={asksToSignIn(mine, ed.champion)} /> },
    players: { label: 'Players', panel: <PlayersPanel entrants={ed.entrants} start={startLabel(ed.ranks_fixed_by)} /> },
  };
  return (
    <>
      <PageHeader
        back={{ href: '/blitz/', label: 'Every blitz tournament' }}
        icon="⚡"
        title={`${ed.name} ${ed.edition}`}
        subtitle={`${formatLabel(ed.format)}. ${reachedLabel(latest, ed.format, ed.champion)}. ${resultsUpdated(ed.results_updated_at, viewerZone())}`}
      >
        <ul className="blitz-pairing-lines">
          <li>Round 1: {roundLine(ed.pairing.first, ed.pairing.first_order)}</li>
          {ed.format.swiss_rounds > 1 && <li>Later rounds: {roundLine(ed.pairing.later, ed.pairing.later_order)}</li>}
        </ul>
        <button type="button" className="btn-antique blitz-how-paired" onClick={(e) => setHowPaired({ opener: e.currentTarget, at: 'top' })}>
          How pairings and ties are calculated
        </button>
      </PageHeader>
      {howPaired !== null && <HowPairedDialog ed={ed} opener={howPaired.opener} at={howPaired.at} onClose={() => setHowPaired(null)} />}
      <Panels>
        <BlitzReport apiUrl={apiUrl} slug={slug} edition={edition} onChanged={() => setAsked((n) => n + 1)} onSeen={setMine} />
        <SignUpBox apiUrl={apiUrl} tournament={slug} season={edition} signUps={ed.sign_ups} entered={ed.entrants.length} onChanged={() => setAsked((n) => n + 1)} />
        {ed.rounds.length === 0 && !ed.sign_ups.taking && (
          <Panel id="blitz-not-started" title="Pairing not started">
            <p className="blitz-intro">Round 1&apos;s pairings, the results and the standings appear here once the organisers publish them.</p>
          </Panel>
        )}
        {knockout.length > 0 && (
          <Panel id="blitz-knockout" title="Knockout">
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
              and the winners of neighbouring matches meet next, through to the final. {knockoutRule(knockout)}
            </p>
            {ed.bracket && (
              <p className="blitz-intro">
                <a href={bracketHref(ed.tournament, ed.edition)}>Go to bracket view</a>
              </p>
            )}
            <KnockoutGames rounds={knockout} format={ed.format} you={yourRow(mine?.match ?? null)} reports={mine?.reports ?? []} />
          </Panel>
        )}
          <Panel label="Standings, pairings and players">
            <SubTabs label="Show" tabs={views.map((v) => ({ value: v, ...panels[v] }))} chosen={hashTab(views, hash)} onChoose={setHash} />
          </Panel>
      </Panels>
    </>
  );
}

/** HowPairedDialog explains how the edition's rounds are paired (its settings, and the rules every method keeps) and how ties in the standings are broken; opened at the top, or at the ties (at). */
function HowPairedDialog({ ed, opener, at, onClose }: { ed: BlitzEdition; opener: HTMLElement; at: 'top' | 'ties'; onClose: () => void }) {
  const notes = pairingNotes(ed);
  const box = useRef<HTMLDialogElement>(null);
  const ties = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (at === 'ties') {
      ties.current?.scrollIntoView();
      ties.current?.focus();
    }
  }, [at]);
  return (
    <Sheet dialog={box} id="how-paired" title="How pairings and ties are calculated" opener={opener} onClose={onClose}>
      <div className="sheet-body">
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
      </div>
    </Sheet>
  );
}

/** StandingsPanel shows the standings by rank, with how the edition scores, a marker before the ranks the lot or the seed ordered (fixedBy: what fixed the starting ranks, null before round 1), the starting ranks once fixed, the tie-breaks on request, and a button to how ties are broken (onHow). */
function StandingsPanel({ standings, scoring, fixedBy, onHow }: { standings: BlitzStanding[]; scoring: string; fixedBy: 'rating' | 'lot' | null; onHow: (e: React.MouseEvent<HTMLButtonElement>) => void }) {
  const [tieBreaks, setTieBreaks] = useState(false);
  return (
    <section aria-labelledby="blitz-standings">
      <PanelHeader id="blitz-standings" title="Standings" />
      <div className="blitz-table-bar">
        <span className="blitz-scoring">{scoring}</span>
        <span className="blitz-table-controls">
          <label className="blitz-toggle">
            <input type="checkbox" checked={tieBreaks} onChange={(e) => setTieBreaks(e.target.checked)} /> Show tie-breaks
          </label>
          <button type="button" className="btn-text" onClick={onHow}>
            How ties are broken
          </button>
        </span>
      </div>
      <ScrollTable label="Standings">
        <table className="leaderboard-table blitz-table blitz-standings">
          <caption>The standings after the results recorded so far</caption>
          <BlitzStandingsTable
            standings={standings}
            fixedBy={fixedBy}
            tieBreaks={tieBreaks}
            marks={(s) => (
              <span className="blitz-flag">
                <Flag country={s.country} />
              </span>
            )}
          />
        </table>
      </ScrollTable>
      {fixedBy !== null && standings.some((s) => s.place_from !== s.place_to) && (
        <p className="blitz-footnote">
          <span className="blitz-draw">*</span> Equal on every tie-break; ordered by {lastTieBreak(fixedBy)}.
        </p>
      )}
    </section>
  );
}

/** PlayersPanel shows the entrants still in, in sign-up order, with their time zones, ratings and, once round 1 fixed them, starting ranks (start: the column's heading). */
function PlayersPanel({ entrants, start }: { entrants: BlitzEdition['entrants']; start: string | null }) {
  const [rows, sort, sortBy] = useSort(entrants, ENTRANT_COLUMNS);
  return (
    <section aria-labelledby="blitz-entrants">
      <PanelHeader id="blitz-entrants" title="Players" />
      <p className="blitz-intro">{entrants.length} in. Ratings are the 2-player community rating of the current run; a dash means not rated yet.</p>
      {entrants.length > 0 && (
        <ScrollTable label="Players">
          <table className="leaderboard-table blitz-table">
            <caption>The players still in, in sign-up order</caption>
            <thead>
              <tr>
                <SortHeader col="player" sort={sort} onSort={sortBy}>
                  Player
                </SortHeader>
                <SortHeader col="zone" sort={sort} onSort={sortBy}>
                  Time zone
                </SortHeader>
                <SortHeader col="rating" sort={sort} onSort={sortBy} align="end">
                  Rating
                </SortHeader>
                {start !== null && (
                  <SortHeader col="start" sort={sort} onSort={sortBy} align="end">
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
        </ScrollTable>
      )}
    </section>
  );
}

/** KnockoutGames lists the knockout's games, first round to final, sortable by any column. */
function KnockoutGames({ rounds, format, you, reports }: { rounds: BlitzRound[]; format: BlitzEdition['format']; you: YourRow | null; reports: MyBlitz['reports'] }) {
  const [sort, setSort] = useState<Sort<GamesColumn>>(null);
  const games = blitzGames(rounds, format, { round: null, player: '', coded: false, page: 1 }, viewerZone(), you, sort, reports);
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


/** PairingsPanel lists the Swiss rounds' games as the games list does: narrowed by round (the latest at first), player and replay code, a page at a time, the signed-in player's game marked; and, when signIn, asks a visitor to sign in to report. */
function PairingsPanel({ ed, swiss, you, reports, signIn }: { ed: BlitzEdition; swiss: BlitzRound[]; you: YourRow | null; reports: MyBlitz['reports']; signIn: boolean }) {
  const [filter, setFilter] = useState<BlitzGamesFilter>({ round: swiss[0]?.number ?? null, player: '', coded: false, page: 1 });
  const [sort, setSort] = useState<Sort<GamesColumn>>(null);
  const narrow = (next: Partial<BlitzGamesFilter>) => setFilter((f) => ({ ...f, ...next, page: next.page ?? 1 }));
  const sortBy = (col: GamesColumn) => {
    setSort((s) => nextSort(s, col, BLITZ_GAME_COLUMNS));
    narrow({});
  };
  const zone = viewerZone();
  const games = blitzGames(swiss, ed.format, filter, zone, you, sort, reports);
  return (
    <section aria-labelledby="blitz-rounds">
      <PanelHeader id="blitz-rounds" title="Pairings and results">
        <span className="tag">{gamesTag(games.games, games.coded, filter.coded)}</span>
      </PanelHeader>
      {signIn && <SignInNote lead="Playing in this tournament?" to="report your game’s result." />}
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
        Who plays whom and how each game went, ratings beside the names. Opponents arrange their games through the official{' '}
        <a href={TTA_DISCORD}>TTA Discord</a>. Dates are on your clocks ({zone}). Click a
        code to copy it. A recorded game is on the <a href={catalogueHref(ed.tournament)}>tournament&apos;s page</a> too.
      </p>
      <GamesTable caption="The games these filters keep, latest round first" context="Round" rows={games.rows} empty="No games match these filters." sort={sort} onSort={sortBy} />
      <NewerOlder page={games.page} pages={games.pages} onPage={(page) => narrow({ page })} />
      {notPlaying(swiss, filter.round, ed.format).map((r) => (
        <p key={r.round} className="blitz-intro">
          Not playing in {r.round}: {r.names.join(', ')}.
        </p>
      ))}
    </section>
  );
}
