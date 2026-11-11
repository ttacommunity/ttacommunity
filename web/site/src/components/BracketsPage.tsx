/** The Brackets page: every tournament shown as a bracket, or one tournament's bracket (?t=, season ?s=). */
import { useEffect, useState } from 'react';
import { blitzEditions, catalogue, catalogueTournament, currentRun, type BlitzEditionRow, type Catalogue, type CatalogueTournament } from '@ttac/shared/ratings';
import { failed, type Found } from '@ttac/shared/load';
import { bracketCard, bracketHref, bracketParams, openingSeason } from '../lib/bracket';
import { catalogueHref, entryRows, seasonName, shownName } from '../lib/catalogue-view';
import { cardView } from '../lib/tournament-card';
import BracketView from './BracketView';
import TournamentCard from './TournamentCard';
import Waiting from './Waiting';

/** BracketsPage shows the bracket the address names, or the list of every tournament shown as one. */
export default function BracketsPage({ apiUrl }: { apiUrl: string }) {
  const [asked] = useState(() => bracketParams(new URLSearchParams(window.location.search)));
  return asked.slug === null ? <BracketList apiUrl={apiUrl} /> : <TournamentBracket apiUrl={apiUrl} slug={asked.slug} asked={asked.season} />;
}

function Missing() {
  return (
    <div className="rating-card waiting-message">
      No tournament shown as a bracket has that name. <a href={bracketHref()}>See every bracket</a>.
    </div>
  );
}

function BracketList({ apiUrl }: { apiUrl: string }) {
  const [load, setLoad] = useState<Found<{ cat: Catalogue; editions: BlitzEditionRow[] }>>({ kind: 'loading' });
  useEffect(() => {
    Promise.all([currentRun(apiUrl).then((r) => catalogue(apiUrl, r.run_id)), blitzEditions(apiUrl)])
      .then(([cat, blitz]) => setLoad({ kind: 'ready', data: { cat, editions: blitz.editions } }))
      .catch((e: unknown) => setLoad(failed(e)));
  }, [apiUrl]);
  const brackets = load.kind === 'ready' ? load.data.cat.tournaments.filter((t) => t.bracket) : [];
  const editions = load.kind === 'ready' ? load.data.editions : [];
  return (
    <>
      <div className="tournaments-header">
        <h1 className="tournaments-title">Brackets</h1>
        <div className="tournaments-subtitle">
          Tournaments played as a knockout, in groups or head-to-head matches, the best of each going through round by
          round to the final. Choose one to follow its bracket, and any player through it.
        </div>
      </div>
      {load.kind !== 'ready' && <Waiting load={load} what="the brackets" />}
      {load.kind === 'ready' && brackets.length === 0 && <div className="rating-card waiting-message">No tournament is shown as a bracket yet.</div>}
      {brackets.length > 0 && (
        <div className="series-grid">
          {brackets.map((t) => {
            const link = bracketCard(t.slug, editions);
            return <TournamentCard key={t.slug} href={link.href} name={shownName(t.name, t.rankings.length > 0)} card={cardView(t.card)} rows={entryRows(t)} cta={link.cta} />;
          })}
        </div>
      )}
    </>
  );
}

function TournamentBracket({ apiUrl, slug, asked }: { apiUrl: string; slug: string; asked: string | null }) {
  const [load, setLoad] = useState<Found<{ run: number; t: CatalogueTournament }>>({ kind: 'loading' });
  const [season, setSeason] = useState<string | null>(null);
  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => catalogueTournament(apiUrl, r.run_id, slug).then((t) => ({ run: r.run_id, t })))
      .then((data) => {
        setLoad({ kind: 'ready', data });
        setSeason(openingSeason(data.t.seasons, asked));
      })
      .catch((e: unknown) => setLoad(failed(e)));
  }, [apiUrl, slug, asked]);
  if (load.kind === 'missing') return <Missing />;
  if (load.kind !== 'ready') return <Waiting load={load} what="this bracket" />;
  const { run, t } = load.data;
  if (!t.bracket || season === null) return <Missing />;
  const pick = (next: string) => {
    setSeason(next);
    window.history.replaceState(null, '', bracketHref(slug, next));
  };
  return (
    <>
      <div className="tournaments-header">
        <h1 className="tournaments-title">
          {t.name} {seasonName(season, t.season_name)}
        </h1>
        <div className="tournaments-subtitle">
          <a href={catalogueHref(slug)}>The tournament: champions, rules and games</a> · <a href={bracketHref()}>Every bracket</a>
        </div>
      </div>
      <BracketView apiUrl={apiUrl} run={run} slug={slug} seasons={t.seasons} seasonWord={t.season_name} season={season} onSeason={pick} />
    </>
  );
}
