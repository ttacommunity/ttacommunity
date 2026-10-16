/** A blitz edition's page, in three tabs: its format and rounds (paired, changed by hand, published, their results recorded), its entrants (searched for and added, edited, dropped, removed), and the standings. */
import { Fragment, useEffect, useState, type FormEvent, type ReactNode } from 'react';
import Segmented from '@ttac/shared/Segmented';
import BackLink from '@ttac/shared/BackLink';
import SubTabs from '@ttac/shared/SubTabs';
import { formatLabel, offsetLabel, resultSummary, roundShown, roundSummary, roundTitle, scoringSentence, seededName } from '@ttac/shared/blitz';
import { formatInstant } from '@ttac/shared/time';
import { adminDownload, adminRequest, firstError, type AdminBlitzEdition, type AdminMe, type BlitzEntrantIds, type Load } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { tabsFor } from '../lib/admin-tabs';
import { editionFormatWarnings, editionParams, formatAdvice, pointsChanged, pointsForm, pointsFrom, pointsSummary, type AdminBlitzEntrant, type PointsForm } from '../lib/blitz-page';
import { emptyGameForm, gameRecordFrom, type BlitzStanding, type GameForm } from '../lib/blitz-results';
import { deadlineForm, deadlineLabel, deadlineLocked, organiserChoices, organisersSummary, threadTextSavable, threadTextSummary, threadsBlocker, threadsFileName, type DeadlineForm, type ThreadText } from '../lib/blitz-threads';
import { entrantToAdd, wildcardChoices, type PersonChoice, type PersonDetails } from '../lib/person-form';
import { countryNames, type Country } from '../lib/countries';
import { clockIn, type Region } from '../lib/zones';
import { asChanges, byeTakers, canDeleteRound, decidedOnly, entrantButtons, leftUnpaired, newestFirst, openRound, pairAction, pairCounts, partnersFor, reconcilePending, replaceableSide, rulingChoices, rulingLabel, samePairs, shownAfter, unpaired, unsavedMatch, zoneGapLabel, type AdminBlitzMatch, type AdminBlitzRound, type Pending } from '../lib/blitz-rounds';
import AdminHeader from './AdminHeader';
import CountryPick from './CountryPick';
import InfoTip from './InfoTip';
import PageError from './PageError';
import PersonForm from './PersonForm';
import PickBox from './PickBox';
import usePeopleSearch from './usePeopleSearch';
import ZonePick from './ZonePick';
import './AdminApp.css';

const KNOCKOUTS = [0, 2, 4, 8, 16, 32];

type Edition = { slug: string; edition: string };
type Props = { at: Edition; ed: AdminBlitzEdition; regions: readonly Region[]; countries: readonly Country[]; onReload: () => Promise<void>; say: (message: string) => void };

/** BlitzEditionPage loads who is signed in, the edition the address names (?t=&e=), the regions a zone may be and the countries, then its tabs. */
export default function BlitzEditionPage() {
  const [me, setMe] = useState<Load<AdminMe>>({ kind: 'loading' });
  const [ed, setEd] = useState<Load<AdminBlitzEdition>>({ kind: 'loading' });
  const [regions, setRegions] = useState<Region[]>([]);
  const [countries, setCountries] = useState<Country[]>([]);
  const [at, setAt] = useState<Edition | null>(null);
  const [message, setMessage] = useState('');
  // The address is read once mounted: the page is prerendered, where there is no window.
  const load = async () => {
    const named = editionParams(new URLSearchParams(window.location.search));
    setAt(named);
    if (named === null) {
      setEd({ kind: 'error', message: 'This address names no blitz edition. Go back to the Blitz tab.' });
      return;
    }
    const r = await adminRequest<AdminBlitzEdition>(`/api/blitz/${named.slug}/${encodeURIComponent(named.edition)}`, {});
    setEd(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  useEffect(() => {
    void adminRequest<AdminMe>('/api/me', {}).then((r) =>
      setMe(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
    void adminRequest<Region[]>('/api/regions', {}).then((r) => (r.ok ? setRegions(r.data) : setMessage(r.message)));
    void adminRequest<string[]>('/api/countries', {}).then((r) => (r.ok ? setCountries(countryNames(r.data)) : setMessage(r.message)));
    void load();
  }, []);
  const error = firstError(me, ed);
  const count = (status: AdminBlitzEntrant['status']) => (ed.kind === 'ready' ? ed.data.entrants.filter((e) => e.status === status).length : 0);

  return (
    <>
      <AdminHeader tabs={me.kind === 'ready' ? tabsFor(me.data.role) : []} current="blitz" />
      <main className="admin-main">
        {error !== null && <PageError message={error} />}
        {me.kind === 'ready' && (
          <p className="signed-in">
            Signed in as <strong>{me.data.name}</strong> ({me.data.role})
          </p>
        )}
        {ed.kind === 'loading' && <p className="muted">Loading…</p>}
        {ed.kind === 'ready' && at !== null && (
          <>
            <section className="rating-card admin-card page-header" aria-labelledby="page-title">
              <div className="header-back">
                <BackLink href="/#blitz">Blitz tournaments</BackLink>
              </div>
              <h2 id="page-title" className="tournament-title">
                {ed.data.name} {ed.data.edition}
              </h2>
              <p className="muted">
                {formatLabel(ed.data.format)}. {count('active')} in, {count('pending')} pending, {count('dropped')} dropped.
                {ed.data.champion !== null && (
                  <>
                    {' '}
                    Champion: <strong>{ed.data.champion}</strong>.
                  </>
                )}
              </p>
            </section>
            {message !== '' && (
              <p className="admin-message" role="status">
                {message}
              </p>
            )}
            <EditionTabs at={at} ed={ed.data} regions={regions} countries={countries} onReload={load} say={setMessage} />
          </>
        )}
      </main>
    </>
  );
}

/** viewerZone is the browser's time zone, for the dates admin shows. */
function viewerZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone;
  } catch {
    return 'UTC';
  }
}

type RoundView = 'round' | 'unpaired';

/** UnpairedList shows the open round's unpaired players, with what pairing them by hand needs: rating and time zone. */
function UnpairedList({ round, players }: { round: number; players: readonly AdminBlitzEntrant[] }) {
  if (players.length === 0) return <p className="muted">Everyone is paired.</p>;
  return (
    <div className="table-responsive">
      <table className="leaderboard-table admin-table">
        <caption className="visually-hidden">Round {round}&apos;s unpaired players</caption>
        <thead>
          <tr>
            <th scope="col">Player</th>
            <th scope="col">Rating</th>
            <th scope="col">Time zone</th>
          </tr>
        </thead>
        <tbody>
          {players.map((e) => (
            <tr key={e.id}>
              <th scope="row">{e.name}</th>
              <td>{e.rating === null ? '–' : Math.round(e.rating)}</td>
              <td>
                {e.zone} <span className="muted">{offsetLabel(e.utc_offset)}</span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** RoundsPanel pairs rounds, changes the open one by hand and publishes it, and shows every round newest first, each match's result recorded in a row below it. */
function RoundsPanel({ at, ed, regions, onReload, say }: Props) {
  const [criteria, setCriteria] = useState({ timezone: true, rating: false });
  const [pick, setPick] = useState({ a: '', b: '', bye: '' });
  const [recording, setRecording] = useState<number | null>(null);
  const [replacing, setReplacing] = useState<number | null>(null);
  const [roundView, setRoundView] = useState<RoundView>('round');
  // toggled holds the rounds whose Show or Hide button was pressed: the newest shows its matches until then.
  const [toggled, setToggled] = useState<ReadonlySet<number>>(new Set());
  const newest = Math.max(0, ...ed.rounds.map((r) => r.number));
  const shown = (r: AdminBlitzRound) => roundShown(r.number, newest, toggled);
  const toggleRound = (n: number) => setToggled((t) => (t.has(n) ? new Set([...t].filter((x) => x !== n)) : new Set([...t, n])));
  // resultZone is the time zone a result form starts in: the browser's, then the last one a game was saved in.
  const [resultZone, setResultZone] = useState(viewerZone);
  const [zone] = useState(viewerZone);
  const byId = new Map(ed.entrants.map((e) => [e.id, e]));
  const nameOf = (id: number) => byId.get(id)?.name ?? `entrant ${id}`;
  const summary = (m: AdminBlitzMatch) => resultSummary({ a: nameOf(m.a), b: m.b === null ? null : nameOf(m.b), result: m.result, game: m.game, ruled: m.ruling !== '' });
  const ratingOf = (id: number) => {
    const r = byId.get(id)?.rating ?? null;
    return r === null ? '–' : String(Math.round(r));
  };
  const open = openRound(ed.rounds);
  // pending is the open round's matches as changed here and not yet saved, kept true to each reload (someone removed
  // drops out of it); null while the saved ones are shown.
  const [unsaved, setPending] = useState<Pending | null>(null);
  const pending = reconcilePending(unsaved, ed.rounds, ed.entrants);
  const shownOf = (r: AdminBlitzRound) => (pending !== null && pending.round === r.id ? pending.matches : r.matches);
  const setShown = (r: AdminBlitzRound, matches: AdminBlitzMatch[]) => {
    const shown = shownAfter(r.matches, matches);
    setPending(shown === null ? null : { round: r.id, matches: shown });
  };
  const action = pairAction(ed, open === null ? null : shownOf(open));
  const free = open === null ? [] : unpaired(ed.entrants, { matches: shownOf(open) });
  const counts = pairCounts(ed.entrants, open === null ? [] : shownOf(open));
  // openSwiss is the round whose counts and unpaired players show; listingUnpaired, while its Unpaired view is chosen.
  const openSwiss = (r: AdminBlitzRound) => r.id === open?.id && r.kind === 'swiss';
  const listingUnpaired = (r: AdminBlitzRound) => openSwiss(r) && roundView === 'unpaired';
  const run = async (r: req.Request, done: string) => {
    const res = await req.send(r);
    say(res.ok ? done : res.message);
    await onReload();
    return res.ok;
  };
  const pair = async () => {
    if (action.kind === 'next') {
      await run(req.pairBlitzRound(at.slug, at.edition, criteria), `${roundTitle(action.round.number, ed.format)} paired as a draft.`);
      return;
    }
    if (action.kind !== 'fill' || open === null) return;
    const res = await req.send<AdminBlitzRound>(req.proposeBlitzPairing(at.slug, at.edition, open.id, { ...criteria, keep: asChanges(shownOf(open)) }));
    if (!res.ok) {
      say(res.message);
      return;
    }
    const same = shownAfter(open.matches, res.data.matches) === null;
    setShown(open, res.data.matches);
    say(same ? 'Paired as saved: nothing to update.' : `Paired. Not saved yet: ${open.published_at === null ? 'publish' : 'update'} it, or discard.`);
  };
  const addPair = (r: AdminBlitzRound) => {
    const a = Number(pick.a);
    const b = Number(pick.b);
    if (!a || !b || a === b) return;
    setPick({ ...pick, a: '', b: '' });
    setShown(r, [...shownOf(r), unsavedMatch(a, b)]);
  };
  const giveBye = (r: AdminBlitzRound) => {
    const a = Number(pick.bye);
    if (!a) return;
    setPick({ ...pick, bye: '' });
    setShown(r, [...shownOf(r).filter((m) => m.b !== null), unsavedMatch(a, null)]);
  };
  const unpair = (r: AdminBlitzRound, i: number) => setShown(r, shownOf(r).filter((_, j) => j !== i));
  const changed = (r: AdminBlitzRound) => pending !== null && pending.round === r.id && !samePairs(r.matches, pending.matches);
  const save = async (r: AdminBlitzRound) => {
    if (changed(r) && !(await run(req.setBlitzRoundMatches(at.slug, at.edition, r.id, asChanges(shownOf(r))), 'Saved.'))) return;
    setPending(null);
    if (r.published_at === null) await run(req.publishBlitzRound(at.slug, at.edition, r.id), `${roundTitle(r.number, ed.format)} published.`);
    else if (changed(r)) say(`${roundTitle(r.number, ed.format)} updated: the site shows it now.`);
  };
  const remove = (r: AdminBlitzRound) => {
    const title = roundTitle(r.number, ed.format);
    const what = r.published_at === null ? `the draft of ${title}` : `${title}, published but not yet played`;
    if (!window.confirm(`Delete ${what}? Pair again afterwards.`)) return;
    setPending(null);
    void run(req.deleteBlitzRound(at.slug, at.edition, r.id), `${title} deleted.`);
  };
  const swiss = (open?.kind ?? (action.kind === 'next' ? action.round.kind : null)) === 'swiss';
  // choose offers the players given, the unpaired ones a pick may take.
  const choose = (value: string, set: (v: string) => void, label: string, players: readonly AdminBlitzEntrant[]) => (
    <label>
      {label}
      <select className="input-antique" value={value} onChange={(e) => set(e.target.value)}>
        <option value="">choose</option>
        {players.map((e) => (
          <option key={e.id} value={e.id}>
            {e.name}
          </option>
        ))}
      </select>
    </label>
  );
  return (
    <div className="admin-subsection">
      <h3>Rounds</h3>
      <p className="muted">Pair each round, change it by hand, publish it, then record its results.</p>
      <div className="admin-add">
        {swiss && (
          <>
            <label className="setting-inline check-inline">
              <input type="checkbox" checked={criteria.timezone} onChange={(e) => setCriteria({ ...criteria, timezone: e.target.checked })} />
              Time-zone fit
            </label>
            <label className="setting-inline check-inline">
              <input type="checkbox" checked={criteria.rating} onChange={(e) => setCriteria({ ...criteria, rating: e.target.checked })} />
              Closest ratings
            </label>
          </>
        )}
        <button type="button" className="btn-antique" disabled={action.kind === 'none'} onClick={() => void pair()}>
          {action.kind === 'next' ? `Pair ${roundTitle(action.round.number, ed.format)}` : 'Pair'}
        </button>
        {open !== null && open.kind === 'swiss' && (
          <button type="button" className="btn-antique" disabled={decidedOnly(shownOf(open)).length === shownOf(open).length} onClick={() => setShown(open, decidedOnly(shownOf(open)))}>
            Unpair all
          </button>
        )}
        {open !== null && (open.kind === 'swiss' || open.published_at === null) && (
          <button type="button" className="btn-antique" disabled={open.published_at !== null && !changed(open)} onClick={() => void save(open)}>
            {open.published_at === null ? 'Publish' : 'Update'}
          </button>
        )}
        {open !== null && changed(open) && (
          <button type="button" className="btn-antique" onClick={() => setPending(null)}>
            Discard changes
          </button>
        )}
        {open !== null && canDeleteRound(open) && (
          <span className="with-tip">
            <button type="button" className="btn-antique" onClick={() => remove(open)}>
              {open.published_at === null ? 'Delete draft' : 'Delete round'}
            </button>
            <InfoTip label="When to delete a round">
              Removes the round and its pairings, so Pair makes it again from scratch. Use it to re-seed a knockout round
              after a Swiss result is corrected, to change the format once the knockout is paired, or to take a round off
              the site. Only while no match in it has a result.
            </InfoTip>
          </span>
        )}
      </div>
      {open !== null && open.kind === 'swiss' && free.length > 0 && (
        <>
          <div className="admin-add">
            {choose(pick.a, (a) => setPick({ ...pick, a }), 'Pair', partnersFor(pick.b === '' ? null : Number(pick.b), free, ed.rounds, open.id))}
            {choose(pick.b, (b) => setPick({ ...pick, b }), 'with', partnersFor(pick.a === '' ? null : Number(pick.a), free, ed.rounds, open.id))}
            <button type="button" className="btn-antique" disabled={!pick.a || !pick.b || pick.a === pick.b} onClick={() => addPair(open)}>
              Pair them
            </button>
          </div>
          <div className="admin-add">
            {choose(pick.bye, (bye) => setPick({ ...pick, bye }), 'Bye to', byeTakers(free, ed.rounds, open.id))}
            <button type="button" className="btn-antique" disabled={!pick.bye} onClick={() => giveBye(open)}>
              Give the bye
            </button>
          </div>
        </>
      )}
      {open !== null && open.kind === 'swiss' && <p className="muted round-hint">To pair everyone again: Unpair all, then Pair.</p>}
      {ed.rounds.length === 0 && <p className="muted">No rounds yet.</p>}
      {newestFirst(ed.rounds).map((r) => (
        <div key={r.id} className="round-block">
          <h3>
            {roundTitle(r.number, ed.format)} · {r.published_at === null ? 'draft' : `published ${formatInstant(r.published_at, zone)}`}
            {changed(r) && <span className="row-detail">unsaved changes</span>}
          </h3>
          <p className="season-summary">
            <span className="muted">{roundSummary(shownOf(r))}</span>{' '}
            <button type="button" className="btn-antique btn-settings" aria-expanded={shown(r)} onClick={() => toggleRound(r.number)}>
              {shown(r) ? 'Hide matches' : 'Show matches'}
            </button>
          </p>
          <RoundThreads at={at} ed={ed} round={r} regions={regions} unsaved={changed(r)} onReload={onReload} say={say} />
          {shown(r) && (
            <>
              {openSwiss(r) && (
                <>
                  <p className="muted round-counts">
                    {counts.paired} paired · {counts.unpaired} unpaired
                  </p>
                  <div className="format-bar round-view">
                    <Segmented<RoundView>
                      label="Show"
                      options={[
                        { value: 'round', label: 'Round' },
                        { value: 'unpaired', label: 'Unpaired' },
                      ]}
                      value={roundView}
                      onChange={setRoundView}
                    />
                  </div>
                </>
              )}
              {listingUnpaired(r) && <UnpairedList round={r.number} players={free} />}
              {!listingUnpaired(r) && shownOf(r).length === 0 && <p className="muted">No matches.</p>}
              {!listingUnpaired(r) && shownOf(r).length > 0 && (
                <div className="table-responsive">
                  <table className="leaderboard-table admin-table">
                    <caption className="visually-hidden">Round {r.number}&apos;s matches</caption>
                    <thead>
                      <tr>
                        <th scope="col">Player</th>
                        <th scope="col">Rating</th>
                        <th scope="col">Opponent</th>
                        <th scope="col">Rating</th>
                        <th scope="col">Clocks</th>
                        <th scope="col">Rating gap</th>
                        <th scope="col">Result</th>
                        <th scope="col">
                          <span className="visually-hidden">Actions</span>
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {shownOf(r).map((m, i) => (
                        <Fragment key={m.id === 0 ? `new-${i}` : m.id}>
                          <tr className={m.repeat ? 'row-error' : ''}>
                            <th scope="row">{seededName(nameOf(m.a), m.a_seed)}</th>
                            <td>{ratingOf(m.a)}</td>
                            <td>{m.b === null ? <em>bye</em> : seededName(nameOf(m.b), m.b_seed)}</td>
                            <td>{m.b === null ? '' : ratingOf(m.b)}</td>
                            <td>{m.b === null ? '' : zoneGapLabel(m.zone_gap_minutes)}</td>
                            <td>
                              {m.rating_gap === null ? '' : Math.round(m.rating_gap)}
                              {m.repeat && <span className="row-detail">met before</span>}
                            </td>
                            <td>
                              {m.b !== null && m.result === null ? <span className="muted">not yet</span> : summary(m)}
                              {m.ruling !== '' && <span className="row-detail muted">{m.ruling}</span>}
                            </td>
                            <td className="run-buttons">
                              {m.b !== null && m.id !== 0 && !changed(r) && (
                                <button
                                  type="button"
                                  className="btn-antique"
                                  aria-expanded={recording === m.id}
                                  onClick={() => {
                                    setReplacing(null);
                                    setRecording(recording === m.id ? null : m.id);
                                  }}
                                >
                                  {m.result === null ? 'Record' : 'Correct'}
                                </button>
                              )}
                              {r.id === open?.id && r.kind === 'swiss' && m.result === null && (
                                <button type="button" className="btn-antique" onClick={() => unpair(r, i)}>
                                  Unpair
                                </button>
                              )}
                              {r.id === open?.id && replaceableSide(m, r, ed.entrants) !== null && (
                                <button
                                  type="button"
                                  className="btn-antique"
                                  aria-expanded={replacing === m.id}
                                  onClick={() => {
                                    setRecording(null);
                                    setReplacing(replacing === m.id ? null : m.id);
                                  }}
                                >
                                  Replace
                                </button>
                              )}
                            </td>
                          </tr>
                          {recording === m.id && m.b !== null && !changed(r) && (
                            <tr className="form-row">
                              <td colSpan={8}>
                                <ResultForm
                                  at={at}
                                  round={r}
                                  match={m}
                                  nameOf={nameOf}
                                  regions={regions}
                                  zone={resultZone}
                                  onZone={setResultZone}
                                  onDone={async (message) => {
                                    setRecording(null);
                                    say(message);
                                    await onReload();
                                  }}
                                  onCancel={() => setRecording(null)}
                                />
                              </td>
                            </tr>
                          )}
                          {replacing === m.id && replaceableSide(m, r, ed.entrants) !== null && (
                            <tr className="form-row">
                              <td colSpan={8}>
                                <ReplaceForm
                                  at={at}
                                  round={r}
                                  match={m}
                                  out={replaceableSide(m, r, ed.entrants) as number}
                                  ed={ed}
                                  nameOf={nameOf}
                                  onDone={async (message) => {
                                    setReplacing(null);
                                    say(message);
                                    await onReload();
                                  }}
                                  onCancel={() => setReplacing(null)}
                                />
                              </td>
                            </tr>
                          )}
                        </Fragment>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </>
          )}
        </div>
      ))}
    </div>
  );
}

/** EntrantsPanel lists every entrant with what admin knows of them, and edits, drops, reinstates or removes one. */
function EntrantsPanel({ at, ed, regions, countries, onReload, say }: Props) {
  const buttons = (e: AdminBlitzEntrant) => entrantButtons(e, ed.rounds);
  const [editing, setEditing] = useState<{ id: number; discord_name: string; email: string; country: string; zone: string } | null>(null);
  const change = async (e: AdminBlitzEntrant, status: AdminBlitzEntrant['status']) => {
    const r = await req.send(req.setBlitzEntrant(at.slug, at.edition, e.id, { status }));
    say(r.ok ? `${e.name} is ${status}.` : r.message);
    await onReload();
  };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (editing === null) return;
    const r = await req.send(
      req.setBlitzEntrant(at.slug, at.edition, editing.id, { discord_name: editing.discord_name, email: editing.email, country: editing.country, zone: editing.zone }),
    );
    say(r.ok ? 'Saved.' : r.message);
    if (r.ok) setEditing(null);
    await onReload();
  };
  const remove = async (e: AdminBlitzEntrant) => {
    const left = leftUnpaired(e.id, ed.rounds).map((id) => ed.entrants.find((x) => x.id === id)?.name ?? `entrant ${id}`);
    const unpaired = left.length === 0 ? '' : ` ${left.join(' and ')} will be unpaired.`;
    if (!window.confirm(`Remove ${e.name} from the edition?${unpaired}`)) return;
    const r = await req.send(req.removeBlitzEntrant(at.slug, at.edition, e.id));
    say(r.ok ? `Removed ${e.name}.${left.length === 0 ? '' : ` ${left.join(' and ')} ${left.length === 1 ? 'is' : 'are'} unpaired now: Pair fills them in.`}` : r.message);
    await onReload();
  };
  return (
    <div className="admin-subsection">
      <h3>Entrants</h3>
      <p className="muted">
        A name the ratings know is shown in their spelling, with the 2-player rating of the current run. Pending
        players are named but not confirmed: neither paired nor shown on the site until you confirm them. Drop someone
        who withdrew once they have played: their results stand and they are not paired again. Remove someone who has
        not played: they come out of the round they are paired in.
      </p>
      {ed.entrants.length === 0 && <p className="muted">No entrants yet.</p>}
      {ed.entrants.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Every entrant, in sign-up order</caption>
            <thead>
              <tr>
                <th scope="col">Username</th>
                <th scope="col">Discord</th>
                <th scope="col">Email</th>
                <th scope="col">Country</th>
                <th scope="col">Time zone</th>
                <th scope="col">Rating</th>
                <th scope="col">Status</th>
                <th scope="col">
                  <span className="visually-hidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {ed.entrants.map((e) =>
                editing?.id === e.id ? (
                  <tr key={e.id}>
                    <th scope="row">{e.name}</th>
                    <td>
                      <input
                        className="input-antique"
                        aria-label={`Discord username of ${e.name}`}
                        value={editing.discord_name}
                        maxLength={100}
                        onChange={(ev) => setEditing({ ...editing, discord_name: ev.target.value })}
                      />
                    </td>
                    <td>
                      <input className="input-antique" aria-label={`Email of ${e.name}`} type="email" value={editing.email} maxLength={254} onChange={(ev) => setEditing({ ...editing, email: ev.target.value })} />
                    </td>
                    <td>
                      <CountryPick label={`Country of ${e.name}`} hideLabel countries={countries} value={editing.country} onChange={(country) => setEditing({ ...editing, country })} />
                    </td>
                    <td>
                      <ZonePick label={`Time zone of ${e.name}`} hideLabel regions={regions} value={editing.zone} onChange={(zone) => setEditing({ ...editing, zone })} />
                    </td>
                    <td>{e.rating === null ? '–' : Math.round(e.rating)}</td>
                    <td>{e.status}</td>
                    <td className="run-buttons">
                      <button type="button" className="btn-antique" onClick={(ev) => void save(ev)}>
                        Save
                      </button>
                      <button type="button" className="btn-antique" onClick={() => setEditing(null)}>
                        Cancel
                      </button>
                    </td>
                  </tr>
                ) : (
                  <tr key={e.id}>
                    <th scope="row">
                      {e.name}
                      {e.player === '' && <span className="row-detail muted">not in the ratings</span>}
                    </th>
                    <td>{e.discord_name}</td>
                    <td>{e.email}</td>
                    <td>{e.country ?? ''}</td>
                    <td>
                      {e.zone} <span className="muted">{offsetLabel(e.utc_offset)}</span>
                    </td>
                    <td>{e.rating === null ? '–' : Math.round(e.rating)}</td>
                    <td>{e.status}</td>
                    <td className="run-buttons">
                      <button type="button" className="btn-antique" onClick={() => setEditing({ id: e.id, discord_name: e.discord_name, email: e.email, country: e.country ?? '', zone: e.zone })}>
                        Edit
                      </button>
                      {buttons(e).confirm && (
                        <button type="button" className="btn-antique" onClick={() => void change(e, 'active')}>
                          Confirm
                        </button>
                      )}
                      {buttons(e).drop !== null && (
                        <button type="button" className="btn-antique" onClick={() => void change(e, e.status === 'dropped' ? 'active' : 'dropped')}>
                          {buttons(e).drop === 'reinstate' ? 'Reinstate' : 'Drop'}
                        </button>
                      )}
                      {buttons(e).remove !== null && (
                        <button type="button" className="btn-antique" onClick={() => void remove(e)}>
                          Remove
                        </button>
                      )}
                    </td>
                  </tr>
                ),
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

/** AddEntrantPanel enters someone found by searching the members not yet entered and the players the ratings know, or someone new by the name typed. Their details fill in where known; what the form then holds is saved on them. */
function AddEntrantPanel({ at, ed, regions, countries, onReload, say }: Props) {
  const [pending, setPending] = useState(false);
  const enter = async (details: PersonDetails) => {
    const r = await req.send<BlitzEntrantIds>(req.addBlitzEntrants(at.slug, at.edition, [entrantToAdd(details, pending)]));
    say(r.ok ? `Entered ${details.name.trim()}.` : r.message);
    if (r.ok) await onReload();
    return r.ok;
  };
  return (
    <div className="admin-subsection">
      <h3>Add an entrant</h3>
      <p className="muted">
        Search the members and the players the ratings know, or add someone new by their username as on CGE. What we
        know of them fills in; anything you add or change is saved on them.
      </p>
      <PersonForm
        searchBase={`/api/blitz/${at.slug}/${encodeURIComponent(at.edition)}/candidates`}
        taken={{ entered: ed.entrants.flatMap((e) => [e.name, e.player]).filter((n) => n !== ''), membersTaken: false }}
        regions={regions}
        countries={countries}
        submitLabel="Add"
        onSubmit={enter}
        refresh={ed}
      >
        <label className="setting-inline check-inline">
          <input type="checkbox" checked={pending} onChange={(e) => setPending(e.target.checked)} />
          Not confirmed yet
        </label>
      </PersonForm>
    </div>
  );
}

type View = 'rounds' | 'entrants' | 'standings';

/** EditionTabs shows one of the format and rounds, the entrants (with the form to add one first) or the standings, chosen by its tabs, the site's sub tabs. */
function EditionTabs(props: Props) {
  const [view, setView] = useState<View | null>(null);
  return (
    <section className="rating-card admin-card" aria-label="Rounds, entrants and standings">
      <SubTabs<View>
        label="Show"
        tabs={[
          {
            value: 'rounds',
            label: 'Rounds',
            panel: (
              <>
                <SettingsSection title="Format" summary={formatLabel(props.ed.format)} what="format" whileHidden={<FormatWarnings warnings={editionFormatWarnings(props.ed)} />}>
                  <FormatPanel {...props} />
                </SettingsSection>
                <SettingsSection title="Points" summary={pointsSummary(props.ed.points, props.ed.fide_points)} what="points">
                  <PointsPanel key={JSON.stringify(props.ed.points)} {...props} />
                </SettingsSection>
                <SettingsSection title="Organisers" summary={organisersSummary(props.ed.organisers)} what="organisers">
                  <OrganisersPanel {...props} />
                </SettingsSection>
                <SettingsSection title="Thread text" summary={threadTextSummary(props.ed.thread_text)} what="thread text">
                  <ThreadTextPanel key={`${props.ed.thread_text.message}\n${props.ed.thread_text.no_discord}`} {...props} />
                </SettingsSection>
                <RoundsPanel {...props} />
              </>
            ),
          },
          {
            value: 'entrants',
            label: 'Entrants',
            panel: (
              <>
                <AddEntrantPanel {...props} />
                <EntrantsPanel {...props} />
              </>
            ),
          },
          { value: 'standings', label: 'Standings', panel: <StandingsPanel standings={props.ed.standings} scoring={scoringSentence(props.ed.points, props.ed.fide_points)} /> },
        ]}
        chosen={view}
        onChoose={setView}
      />
    </section>
  );
}

/** SettingsSection is one of the edition's settings under its heading: a summary, and a button that shows or hides the setting, hidden at first, with whileHidden below the summary until it is shown. Hidden, the setting stays mounted, so what is half typed is kept. */
function SettingsSection({ title, summary, what, whileHidden, children }: { title: string; summary: string; what: string; whileHidden?: ReactNode; children: ReactNode }) {
  const [shown, setShown] = useState(false);
  return (
    <div className="admin-subsection">
      <h3>{title}</h3>
      <p className="season-summary">
        <span className="muted">{summary}</span>{' '}
        <button type="button" className="btn-antique btn-settings" aria-expanded={shown} onClick={() => setShown(!shown)}>
          {shown ? `Hide ${what}` : `Show ${what}`}
        </button>
      </p>
      {!shown && whileHidden}
      <div hidden={!shown}>{children}</div>
    </div>
  );
}

/** FormatWarnings shows warnings about a format, one note each. */
function FormatWarnings({ warnings }: { warnings: readonly string[] }) {
  return warnings.map((w) => (
    <p key={w} className="format-warning" role="note">
      {w}
    </p>
  ));
}

/** OrganisersPanel lists the members every round's Discord threads tell players to ping, removes one, and adds a member found by their username. */
function OrganisersPanel({ at, ed, onReload, say }: Props) {
  const [typed, setTyped] = useState('');
  const found = usePeopleSearch('/api/people', typed, false, ed);
  const choices = organiserChoices(found, ed.organisers);
  const change = async (r: req.Request, done: string) => {
    const res = await req.send(r);
    say(res.ok ? done : res.message);
    if (res.ok) setTyped('');
    await onReload();
  };
  return (
    <>
      <p className="muted">Each game&apos;s Discord thread tells its players to ping them, by Discord username.</p>
      {ed.organisers.length === 0 && <p className="muted">None yet.</p>}
      {ed.organisers.length > 0 && (
        <ul className="organiser-list">
          {ed.organisers.map((o) => (
            <li key={o.member}>
              {o.name} <span className="muted">{o.discord_name === '' ? 'no Discord username yet' : `@${o.discord_name}`}</span>{' '}
              <button type="button" className="btn-antique" onClick={() => void change(req.removeBlitzOrganiser(at.slug, at.edition, o.member), `${o.name} no longer organises it.`)}>
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}
      <div className="admin-add">
        <PickBox
          label="Add an organiser"
          text={typed}
          placeholder="Type a member's name"
          options={choices}
          onText={setTyped}
          onPick={(o) => {
            const c = choices.find((x) => x.key === o.key);
            if (c !== undefined && !c.disabled) void change(req.addBlitzOrganiser(at.slug, at.edition, Number(c.key)), `${c.label} organises it now.`);
          }}
        />
      </div>
    </>
  );
}

/** ThreadTextPanel edits what each game's Discord thread says, the welcome message and the sentence for a player without Discord, and resets it to the default. Keyed by the saved text, so a save or reset starts the form again from it. */
function ThreadTextPanel({ at, ed, onReload, say }: Props) {
  const saved = ed.thread_text;
  const [form, setForm] = useState<ThreadText>({ message: saved.message, no_discord: saved.no_discord });
  const send = async (r: req.Request, done: string) => {
    const res = await req.send(r);
    say(res.ok ? done : res.message);
    await onReload();
  };
  return (
    <>
      <p className="muted">
        Each game&apos;s Discord thread starts with the message, then whom to ping, then the sentence for each player without Discord. Wherever they appear, {'{players}'} becomes
        the two players, by Discord username (their TTA username without one), {'{deadline}'} the round&apos;s deadline, on each reader&apos;s clock, and {'{name}'} the TTA
        username of the player without Discord. Leave a box empty to leave it out.
      </p>
      <div className="note-box">
        <label htmlFor="thread-message">Message</label>
        <textarea id="thread-message" className="regulations-text" rows={5} maxLength={1500} value={form.message} onChange={(e) => setForm({ ...form, message: e.target.value })} />
      </div>
      <div className="note-box">
        <label htmlFor="thread-no-discord">For a player without Discord</label>
        <textarea id="thread-no-discord" className="regulations-text" rows={3} maxLength={300} value={form.no_discord} onChange={(e) => setForm({ ...form, no_discord: e.target.value })} />
      </div>
      <div className="note-actions">
        <button type="button" className="btn-antique" disabled={!threadTextSavable(form, saved)} onClick={() => void send(req.setBlitzThreadText(at.slug, at.edition, form), 'Thread text saved.')}>
          Save
        </button>
        {saved.edited && (
          <button type="button" className="btn-antique" onClick={() => void send(req.resetBlitzThreadText(at.slug, at.edition), 'Thread text back to the default.')}>
            Reset to default
          </button>
        )}
      </div>
    </>
  );
}

type RoundThreadsProps = Pick<Props, 'at' | 'ed' | 'regions' | 'onReload' | 'say'> & { round: AdminBlitzRound; unsaved: boolean };

/** RoundThreads shows a round's deadline and sets it (a day and a time on a zone's clocks) until the round is finished, and downloads the round's Discord threads once they can be made. */
function RoundThreads({ at, ed, round, regions, unsaved, onReload, say }: RoundThreadsProps) {
  const [form, setForm] = useState<DeadlineForm | null>(null);
  const blocker = threadsBlocker(round, ed.organisers.length, unsaved);
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (form === null) return;
    const r = await req.send(req.setBlitzRoundDeadline(at.slug, at.edition, round.id, form));
    say(r.ok ? `${roundTitle(round.number, ed.format)}: deadline saved.` : r.message);
    if (r.ok) setForm(null);
    await onReload();
  };
  const download = async () => {
    const r = await adminDownload(req.blitzThreadsPath(at.slug, at.edition, round.id));
    if (!r.ok) {
      say(r.message);
      return;
    }
    const url = URL.createObjectURL(r.data);
    const a = document.createElement('a');
    a.href = url;
    a.download = threadsFileName(at.slug, at.edition, round.number);
    a.click();
    URL.revokeObjectURL(url);
    say(`${roundTitle(round.number, ed.format)}: Discord threads downloaded.`);
  };
  return (
    <>
      <p className="season-summary">
        <span className="muted">{deadlineLabel(regions, round.deadline)}</span>{' '}
        {!deadlineLocked(round) && (
          <button type="button" className="btn-antique btn-settings" aria-expanded={form !== null} onClick={() => setForm(form === null ? deadlineForm(regions, round.deadline, viewerZone()) : null)}>
            {round.deadline === null ? 'Set deadline' : 'Change deadline'}
          </button>
        )}{' '}
        <button type="button" className="btn-antique btn-settings" disabled={blocker !== ''} onClick={() => void download()}>
          Discord threads
        </button>
        {blocker !== '' && <span className="row-detail muted">{blocker}</span>}
      </p>
      {form !== null && !deadlineLocked(round) && (
        <form className="result-row" onSubmit={(e) => void save(e)}>
          <label>
            Deadline
            <input className="input-antique" type="date" required value={form.day} onChange={(e) => setForm({ ...form, day: e.target.value })} />
          </label>
          <label>
            Time
            <input className="input-antique" type="time" required value={form.time} onChange={(e) => setForm({ ...form, time: e.target.value })} />
          </label>
          <ZonePick label="Time zone" regions={regions} value={form.zone} onChange={(zone) => setForm({ ...form, zone })} />
          <button type="submit" className="btn-antique">
            Save
          </button>
          <button type="button" className="btn-antique" onClick={() => setForm(null)}>
            Cancel
          </button>
        </form>
      )}
    </>
  );
}

// The points form's fields, in the order the summary lists them.
const POINT_FIELDS: { key: keyof PointsForm; label: string }[] = [
  { key: 'win', label: 'Win' },
  { key: 'draw', label: 'Draw' },
  { key: 'loss', label: 'Loss' },
  { key: 'forfeit_win', label: 'Forfeit win' },
  { key: 'forfeit_loss', label: 'Forfeit loss' },
  { key: 'bye', label: 'Bye' },
  { key: 'skip', label: 'Skip' },
];

/** PointsPanel sets what each outcome of the edition is worth, and resets them to FIDE's, until the knockout has a round. Keyed by the saved points, so a save or reset starts the form again from them. */
function PointsPanel({ at, ed, onReload, say }: Props) {
  const [form, setForm] = useState<PointsForm>(pointsForm(ed.points));
  const locked = ed.rounds.some((r) => r.kind === 'knockout');
  const send = async (r: req.Request, done: string) => {
    const res = await req.send(r);
    say(res.ok ? done : res.message);
    await onReload();
  };
  const save = (e: FormEvent) => {
    e.preventDefault();
    void send(req.setBlitzPoints(at.slug, at.edition, pointsFrom(form)), 'Points saved.');
  };
  return (
    <>
      <p className="muted">
        0 to 10 in halves. The standings, the next pairing and the bye follow at once; rounds already paired stay. A ruled draw scores as a draw, and &ldquo;both
        lose&rdquo; as a forfeit loss each.
      </p>
      {locked && <p className="muted">The knockout has started: the points no longer change.</p>}
      <form className="admin-add" onSubmit={save}>
        {POINT_FIELDS.map((f) => (
          <label key={f.key}>
            {f.label}
            <input
              className="input-antique"
              type="number"
              min={0}
              max={10}
              step={0.5}
              required
              disabled={locked}
              value={form[f.key]}
              onChange={(e) => setForm({ ...form, [f.key]: e.target.value })}
            />
          </label>
        ))}
        <button type="submit" className="btn-antique" disabled={locked || !pointsChanged(pointsFrom(form), ed.points)}>
          Save
        </button>
        {!ed.fide_points && (
          <button type="button" className="btn-antique" disabled={locked} onClick={() => void send(req.resetBlitzPoints(at.slug, at.edition), "Points back to FIDE's.")}>
            Reset to FIDE&apos;s
          </button>
        )}
      </form>
    </>
  );
}

/** FormatPanel sets the edition's Swiss rounds and knockout size. */
function FormatPanel({ at, ed, onReload, say }: Props) {
  const [format, setFormat] = useState(ed.format);
  const locked = ed.rounds.some((r) => r.kind === 'knockout');
  const advice = formatAdvice(format, ed.entrants.filter((e) => e.status === 'active').length);
  const save = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send(req.setBlitzFormat(at.slug, at.edition, format));
    say(r.ok ? `Format saved: ${formatLabel(format)}.` : r.message);
    await onReload();
  };
  return (
    <>
      {locked && <p className="muted">The knockout has started: the format no longer changes.</p>}
      <form className="admin-add" onSubmit={(e) => void save(e)}>
        <label>
          Swiss rounds
          <input
            className="input-antique"
            type="number"
            min={1}
            max={30}
            value={format.swiss_rounds}
            onChange={(e) => setFormat({ ...format, swiss_rounds: Number(e.target.value) })}
          />
        </label>
        <label>
          Knockout
          <select className="input-antique" value={format.knockout_size} onChange={(e) => setFormat({ ...format, knockout_size: Number(e.target.value) })}>
            {KNOCKOUTS.map((n) => (
              <option key={n} value={n}>
                {n === 0 ? 'none' : `top ${n}`}
              </option>
            ))}
          </select>
        </label>
        <button type="submit" className="btn-antique" disabled={locked}>
          Save
        </button>
      </form>
      {!locked && (
        <p className="muted round-hint">
          Up to {advice.limit} Swiss {advice.limit === 1 ? 'round' : 'rounds'} can always be paired without anyone meeting twice.
        </p>
      )}
      {!locked && <FormatWarnings warnings={advice.warnings} />}
    </>
  );
}

/** today is the day in the viewer's zone as YYYY-MM-DD, the result form's default. */
function today(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** formFromGame fills the game form from a recorded game, its finish on the clocks of zone, for a correction; else the empty form in zone. */
function formFromGame(m: AdminBlitzMatch, zone: string, regions: readonly Region[]): GameForm {
  if (m.game === null) return emptyGameForm(today(), zone);
  const { day, clock } = clockIn(regions, zone, m.game.finished_at);
  return {
    share_code: m.game.share_code,
    a_culture: String(m.game.a_culture),
    a_outcome: m.game.a_outcome,
    b_culture: String(m.game.b_culture),
    b_outcome: m.game.b_outcome,
    finished_on: day,
    finished_time: clock,
    zone,
  };
}

const OUTCOMES: { value: GameForm['a_outcome']; label: string }[] = [
  { value: 'finished', label: 'finished' },
  { value: 'resigned', label: 'resigned' },
  { value: 'timed_out', label: 'timed out' },
];

type ResultFormProps = {
  at: Edition;
  round: AdminBlitzRound;
  match: AdminBlitzMatch;
  nameOf: (id: number) => string;
  regions: readonly Region[];
  /** zone is the time zone the form starts in; onZone is told the one a game was saved in. */
  zone: string;
  onZone: (zone: string) => void;
  onDone: (message: string) => Promise<void>;
  onCancel: () => void;
};

/** ResultForm records a match's result: the game played (share code, each player's outcome and culture, when it finished and in which time zone), or the organisers' ruling with its reason. A refusal shows in the form. */
function ResultForm({ at, round, match, nameOf, regions, zone, onZone, onDone, onCancel }: ResultFormProps) {
  const [error, setError] = useState('');
  const [kind, setKind] = useState<'game' | 'ruling'>(match.ruling !== '' ? 'ruling' : 'game');
  const [game, setGame] = useState<GameForm>(() => formFromGame(match, zone, regions));
  const choices = rulingChoices(round.kind);
  const [ruling, setRuling] = useState({ result: match.result !== null && choices.includes(match.result) ? match.result : 'a', reason: match.ruling });
  const a = nameOf(match.a);
  const b = nameOf(match.b as number);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (kind === 'game') {
      const r = gameRecordFrom(game);
      if (!r.ok) {
        setError(r.message);
        return;
      }
      const res = await req.send(req.recordBlitzGame(at.slug, at.edition, round.id, match.id, r.body));
      if (!res.ok) {
        setError(res.message);
        return;
      }
      onZone(r.body.zone);
      await onDone(`${a} v ${b} recorded.`);
      return;
    }
    if (ruling.reason.trim() === '') {
      setError('A ruling needs a reason.');
      return;
    }
    const res = await req.send(req.recordBlitzRuling(at.slug, at.edition, round.id, match.id, ruling));
    if (res.ok) await onDone(`${a} v ${b} ruled.`);
    else setError(res.message);
  };
  // side is one player's row: how their game ended, then their culture (0 for one who resigned or timed out).
  const side = (who: 'a' | 'b', name: string) => (
    <div className="result-row">
      <label>
        {name}
        <select className="input-antique" value={game[`${who}_outcome`]} onChange={(e) => setGame({ ...game, [`${who}_outcome`]: e.target.value as GameForm['a_outcome'] })}>
          {OUTCOMES.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </label>
      <label>
        {name}&apos;s culture
        <input
          className="input-antique"
          type="number"
          min={0}
          step="any"
          value={game[`${who}_culture`]}
          placeholder={game[`${who}_outcome`] === 'finished' ? undefined : '0'}
          disabled={game[`${who}_outcome`] !== 'finished'}
          onChange={(e) => setGame({ ...game, [`${who}_culture`]: e.target.value })}
        />
      </label>
    </div>
  );
  return (
    <form className="result-form" onSubmit={(e) => void submit(e)}>
      <h3>
        Result of {a} v {b}
      </h3>
      <div className="result-row">
        <label className="setting-inline">
          <input type="radio" name="kind" checked={kind === 'game'} onChange={() => setKind('game')} />
          A played game
        </label>
        <label className="setting-inline">
          <input type="radio" name="kind" checked={kind === 'ruling'} onChange={() => setKind('ruling')} />
          A ruling
        </label>
      </div>
      {kind === 'game' && (
        <>
          <div className="result-row">
            <label>
              Share code
              <input className="input-antique" value={game.share_code} maxLength={32} onChange={(e) => setGame({ ...game, share_code: e.target.value })} />
            </label>
          </div>
          <div className="result-row">
            <label>
              Finished on
              <input className="input-antique" type="date" value={game.finished_on} onChange={(e) => setGame({ ...game, finished_on: e.target.value })} />
            </label>
            <label>
              Time
              <input className="input-antique" type="time" value={game.finished_time} onChange={(e) => setGame({ ...game, finished_time: e.target.value })} />
            </label>
            <ZonePick label="Time zone" regions={regions} value={game.zone} onChange={(z) => setGame({ ...game, zone: z })} />
          </div>
          {side('a', a)}
          {side('b', b)}
        </>
      )}
      {kind === 'ruling' && (
        <div className="result-row">
          <label>
            Ruling
            <select className="input-antique" value={ruling.result} onChange={(e) => setRuling({ ...ruling, result: e.target.value as typeof ruling.result })}>
              {choices.map((c) => (
                <option key={c} value={c}>
                  {rulingLabel(c, seededName(a, match.a_seed), seededName(b, match.b_seed), match.game !== null)}
                </option>
              ))}
            </select>
          </label>
          <label>
            Reason
            <input className="input-antique" value={ruling.reason} maxLength={1000} onChange={(e) => setRuling({ ...ruling, reason: e.target.value })} />
          </label>
        </div>
      )}
      {error !== '' && (
        <p className="row-error form-error" role="alert">
          {error}
        </p>
      )}
      <div className="result-row">
        <button type="submit" className="btn-antique">
          Save
        </button>
        <button type="button" className="btn-antique" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  );
}

type ReplaceFormProps = {
  at: Edition;
  round: AdminBlitzRound;
  match: AdminBlitzMatch;
  /** out is the player who dropped, whose slot and seed the replacement takes. */
  out: number;
  ed: AdminBlitzEdition;
  nameOf: (id: number) => string;
  onDone: (message: string) => Promise<void>;
  onCancel: () => void;
};

/** ReplaceForm puts someone in the place of a knockout player who dropped: the round's reserve, the best Swiss seed not in it, or a wildcard: an entrant not in the round, or anyone else, entered on the spot. A refusal shows in the form. */
function ReplaceForm({ at, round, match, out, ed, nameOf, onDone, onCancel }: ReplaceFormProps) {
  const reserve = round.reserve;
  const [how, setHow] = useState<'reserve' | 'wildcard'>(reserve !== null ? 'reserve' : 'wildcard');
  const [typed, setTyped] = useState('');
  const [picked, setPicked] = useState<PersonChoice | null>(null);
  const [error, setError] = useState('');
  const found = usePeopleSearch(`/api/blitz/${at.slug}/${encodeURIComponent(at.edition)}/candidates`, typed, picked !== null, ed);
  const inRound = new Set(round.matches.flatMap((m) => (m.b === null ? [m.a] : [m.a, m.b])));
  const outsiders = ed.entrants.filter((e) => e.status === 'active' && !inRound.has(e.id)).map((e) => ({ id: e.id, name: e.name }));
  const entered = ed.entrants.flatMap((e) => [e.name, e.player]).filter((n) => n !== '');
  const choices = picked === null ? wildcardChoices(outsiders, found, typed, entered) : [];
  const seed = match.a === out ? match.a_seed : match.b_seed;
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    let id: number | null = how === 'reserve' ? reserve : null;
    if (how === 'wildcard' && picked !== null) {
      if (picked.key.startsWith('entrant ')) id = Number(picked.key.slice('entrant '.length));
      else {
        const added = await req.send<BlitzEntrantIds>(req.addBlitzEntrants(at.slug, at.edition, [entrantToAdd(picked.fill, false)]));
        if (!added.ok) {
          setError(added.message);
          return;
        }
        id = added.data.ids[0];
      }
    }
    if (id === null) return;
    const name = how === 'reserve' ? nameOf(id) : (picked?.fill.name ?? nameOf(id));
    const res = await req.send(req.replaceBlitzPlayer(at.slug, at.edition, round.id, match.id, { out, in: id }));
    if (res.ok) await onDone(`${name} takes ${nameOf(out)}'s place.`);
    else setError(res.message);
  };
  return (
    <form className="result-form" onSubmit={(e) => void submit(e)}>
      <h3>Replace {nameOf(out)}, who dropped</h3>
      <p className="muted">The replacement takes their slot{seed === null ? '' : ` and seed (${seed})`}.</p>
      <div className="result-row">
        <label className="setting-inline">
          <input type="radio" name="how" checked={how === 'reserve'} disabled={reserve === null} onChange={() => setHow('reserve')} />
          {reserve === null ? 'No seed left' : `The next highest seed: ${nameOf(reserve)}`}
        </label>
        <label className="setting-inline">
          <input type="radio" name="how" checked={how === 'wildcard'} onChange={() => setHow('wildcard')} />A wildcard
        </label>
      </div>
      {how === 'wildcard' && (
        <div className="result-row">
          <PickBox
            label="Wildcard"
            text={typed}
            placeholder="Type a name"
            options={choices.map((c) => ({ key: c.key, label: c.label, detail: c.detail, disabled: c.disabled }))}
            onText={(text) => {
              setTyped(text);
              setPicked(null);
            }}
            onPick={(o) => {
              const c = choices.find((x) => x.key === o.key);
              if (c === undefined) return;
              setPicked(c);
              setTyped(c.fill.name);
            }}
          />
          {picked !== null && <span className="muted pick-note">{picked.detail}</span>}
        </div>
      )}
      {error !== '' && (
        <p className="row-error form-error" role="alert">
          {error}
        </p>
      )}
      <div className="result-row">
        <button type="submit" className="btn-antique" disabled={how === 'wildcard' && picked === null}>
          Replace
        </button>
        <button type="button" className="btn-antique" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </form>
  );
}

/** StandingsPanel shows the standings as the site does: how the edition scores, then rank, points, games played, byes and Buchholz, dropped players last. */
function StandingsPanel({ standings, scoring }: { standings: BlitzStanding[]; scoring: string }) {
  return (
    <>
      <p className="muted">
        {scoring} Ties are broken by the Swiss tie-breaks: Direct Encounter (the games
        among the tied players), then Buchholz (the sum of the opponents&apos; scores; a bye, a forfeit or a missed round
        counts as a game against an opponent on the player&apos;s own score), then a draw fixed for the edition.
      </p>
      {standings.length === 0 && <p className="muted">Nobody to rank yet.</p>}
      {standings.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">The standings</caption>
            <thead>
              <tr>
                <th scope="col">Rank</th>
                <th scope="col">Player</th>
                <th scope="col">Points</th>
                <th scope="col">Played</th>
                <th scope="col">Byes</th>
                <th scope="col">Buchholz</th>
              </tr>
            </thead>
            <tbody>
              {standings.map((s) => (
                <tr key={s.name} className={s.status === 'dropped' ? 'muted' : ''}>
                  <td>{s.rank}</td>
                  <th scope="row">
                    {s.name}
                    {s.status === 'dropped' && <span className="row-detail muted">dropped</span>}
                  </th>
                  <td>{s.points}</td>
                  <td>{s.games}</td>
                  <td>{s.byes}</td>
                  <td>{s.buchholz}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
