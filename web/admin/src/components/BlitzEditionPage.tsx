/** A blitz edition's page, in three tabs: its format and rounds (paired, changed by hand, published, their results recorded), its entrants (searched for and added, edited, dropped, removed), and the standings. */
import { Fragment, useEffect, useState, type FormEvent } from 'react';
import Segmented from '@ttac/shared/Segmented';
import BackLink from '@ttac/shared/BackLink';
import { useDialog } from '@ttac/shared/Dialog';
import SubTabs, { useHash } from '@ttac/shared/SubTabs';
import SortHeader, { useSort } from '@ttac/shared/SortHeader';
import { nextSort, sortRows, type Sort } from '@ttac/shared/sort';
import { hashTab } from '@ttac/shared/tabs';
import { ENTRANT_COLUMNS, STANDING_COLUMNS, byFinish, formatLabel, latestFinishes, offsetLabel, placeLabel, resultSummary, roundShown, roundSummary, roundTitle, scoringSentence, seededName, turnMark, turnOrder } from '@ttac/shared/blitz';
import { formatInstant } from '@ttac/shared/time';
import { adminDownload, adminRequest, firstError, type AdminBlitzEdition, type AdminMe, type BlitzEntrantIds, type Load } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { tabsFor } from '../lib/admin-tabs';
import { ADMIN_ENTRANT_COLUMNS, editionFormatWarnings, editionParams, formatAdvice, pointsChanged, pointsForm, pointsFrom, settingsRows, type AdminBlitzEntrant, type PointsForm } from '../lib/blitz-page';
import { chooseTurn, emptyGameForm, gameRecordFrom, TURNS, type BlitzStanding, type GameForm } from '@ttac/shared/game-form';
import { deadlineForm, deadlineLabel, deadlineLocked, organiserChoices, threadTextSavable, threadsBlocker, threadsFileName, type DeadlineForm, type ThreadText } from '../lib/blitz-threads';
import { entrantToAdd, wildcardChoices, type PersonChoice, type PersonDetails } from '../lib/person-form';
import { countryNames, type Country } from '../lib/countries';
import { clockIn, type Region } from '../lib/zones';
import { loadFailure, outcomeOf, refused, shownAt, succeeded, type Outcome } from '../lib/outcome';
import { asChanges, byeTakers, canDeleteRound, canResetEdition, decidedOnly, finishRounds, flaggedColumns, matchColumns, type MatchColumn, resetsOf, entrantActions, entrantState, leftUnpaired, newestFirst, openRound, flaggedMatches, pairAction, pairCounts, partnersFor, reconcilePending, replaceableSide, reportNote, rulingChoices, rulingLabel, samePairs, shownAfter, unpaired, unsavedMatch, zoneGapLabel, type AdminBlitzMatch, type AdminBlitzRound, type Pending } from '../lib/blitz-rounds';
import AdminHeader from './AdminHeader';
import CountryPick from './CountryPick';
import InfoTip from './InfoTip';
import Notice, { NoticeRow } from './Notice';
import PageError from './PageError';
import PersonForm from './PersonForm';
import PickBox from './PickBox';
import usePeopleSearch from './usePeopleSearch';
import ZonePick from './ZonePick';
import './AdminApp.css';

const KNOCKOUTS = [0, 2, 4, 8, 16, 32];

type Edition = { slug: string; edition: string };
/** say shows an action's outcome at its place on the page, replacing the last one; null closes it. */
type Props = { at: Edition; ed: AdminBlitzEdition; regions: readonly Region[]; countries: readonly Country[]; onReload: () => Promise<void>; outcome: Outcome | null; say: (o: Outcome | null) => void };

/** BlitzEditionPage loads who is signed in, the edition the address names (?t=&e=), the regions a zone may be and the countries, then its tabs. */
export default function BlitzEditionPage() {
  const [me, setMe] = useState<Load<AdminMe>>({ kind: 'loading' });
  const [ed, setEd] = useState<Load<AdminBlitzEdition>>({ kind: 'loading' });
  const [regions, setRegions] = useState<Region[]>([]);
  const [countries, setCountries] = useState<Country[]>([]);
  const [at, setAt] = useState<Edition | null>(null);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
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
    void adminRequest<Region[]>('/api/regions', {}).then((r) => (r.ok ? setRegions(r.data) : setOutcome(loadFailure(r.message, 'page'))));
    void adminRequest<string[]>('/api/countries', {}).then((r) => (r.ok ? setCountries(countryNames(r.data)) : setOutcome(loadFailure(r.message, 'page'))));
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
                {formatLabel(ed.data.format)}. {count('active')} in, {count('dropped')} dropped.
                {ed.data.champion !== null && (
                  <>
                    {' '}
                    Champion: <strong>{ed.data.champion}</strong>.
                  </>
                )}
              </p>
            </section>
            <Notice outcome={shownAt(outcome, 'page')} />
            <FlaggedResults at={at} ed={ed.data} regions={regions} onReload={load} outcome={outcome} say={setOutcome} />
            <EditionTabs at={at} ed={ed.data} regions={regions} countries={countries} onReload={load} outcome={outcome} say={setOutcome} />
          </>
        )}
      </main>
    </>
  );
}

/** FlaggedResults lists the players' reports flagged for an organiser, with a count, as Members lists the requests waiting: each one kept as reported, corrected in place, or reset for the players to report again; it says when none is flagged. */
function FlaggedResults({ at, ed, regions, onReload, outcome, say }: Omit<Props, 'countries'>) {
  const dialog = useDialog();
  const [correcting, setCorrecting] = useState<number | null>(null);
  const [resultZone, setResultZone] = useState(viewerZone);
  const flagged = flaggedMatches(ed.rounds);
  const resets = resetsOf(ed.rounds);
  const byId = new Map(ed.entrants.map((e) => [e.id, e]));
  const nameOf = (id: number) => byId.get(id)?.name ?? `entrant ${id}`;
  const pairOf = (m: AdminBlitzMatch) => `${nameOf(m.a)} v ${nameOf(m.b as number)}`;
  const view = (m: AdminBlitzMatch) => ({ a: nameOf(m.a), b: nameOf(m.b as number), result: m.result, game: m.game, ruled: m.ruling !== '' });
  const [flaggedRows, flaggedSort, sortFlagged] = useSort(flagged, flaggedColumns(nameOf));
  const run = async (r: req.Request, done: string) => {
    say(outcomeOf(await req.send(r), done, 'flagged'));
    await onReload();
  };
  const keep = async (r: AdminBlitzRound, m: AdminBlitzMatch) => {
    const sure = await dialog.confirm({ heading: `Keep the reported result of ${pairOf(m)}?`, text: 'The flag is resolved and the result stays as reported. Players still cannot change it; Reset opens it to them again.', action: 'Keep result' });
    if (sure) await run(req.keepBlitzReport(at.slug, at.edition, r.id, m.id), `${pairOf(m)}: report kept.`);
  };
  const reset = async (r: AdminBlitzRound, m: AdminBlitzMatch) => {
    const sure = await dialog.confirm({ heading: `Reset ${pairOf(m)}?`, text: 'Its result and game go, as if never played; the players can report it again.', action: 'Reset game', final: true });
    if (sure) await run(req.resetBlitzResult(at.slug, at.edition, r.id, m.id), `${pairOf(m)} reset: the players can report it again.`);
  };
  return (
    <section className="rating-card admin-card flagged-results" aria-labelledby="flagged-title">
      {dialog.element}
      <h3 id="flagged-title">
        Flagged results
        {flagged.length > 0 && <span className="count-badge">{flagged.length}</span>}
      </h3>
      <p className="muted">A player says their opponent&apos;s report is wrong. Ask both on Discord, then keep it, correct it or reset it.</p>
      <Notice outcome={shownAt(outcome, 'flagged')} onClose={() => say(null)} />
      {flagged.length === 0 && <p className="muted">No reports flagged.</p>}
      {flagged.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Reports flagged by a player, oldest round first</caption>
            <thead>
              <tr>
                <SortHeader col="round" sort={flaggedSort} onSort={sortFlagged}>
                  Round
                </SortHeader>
                <SortHeader col="game" sort={flaggedSort} onSort={sortFlagged}>
                  Game
                </SortHeader>
                <SortHeader col="result" sort={flaggedSort} onSort={sortFlagged}>
                  Reported result
                </SortHeader>
                <th scope="col">
                  <span className="visually-hidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {flaggedRows.map(({ round: r, match: m }) => (
                <Fragment key={m.id}>
                  <tr>
                    <td>{roundTitle(r.number, ed.format)}</td>
                    <th scope="row">{pairOf(m)}</th>
                    <td>
                      {resultSummary(view(m))}
                      {turnOrder(view(m)) !== '' && <span className="row-detail muted">{turnOrder(view(m))}</span>}
                      <span className="row-detail row-flagged">{reportNote(m, nameOf)}</span>
                    </td>
                    <td className="run-buttons">
                      <button type="button" className="btn-antique" onClick={() => void keep(r, m)}>
                        Keep
                      </button>
                      <button type="button" className="btn-antique" aria-expanded={correcting === m.id} onClick={() => setCorrecting(correcting === m.id ? null : m.id)}>
                        Correct
                      </button>
                      {resets.matches.includes(m.id) && (
                        <button type="button" className="btn-antique" onClick={() => void reset(r, m)}>
                          Reset
                        </button>
                      )}
                    </td>
                  </tr>
                  {correcting === m.id && (
                    <tr className="form-row">
                      <td colSpan={4}>
                        <ResultForm
                          at={at}
                          round={r}
                          match={m}
                          nameOf={nameOf}
                          regions={regions}
                          zone={resultZone}
                          onZone={setResultZone}
                          onDone={async (message) => {
                            setCorrecting(null);
                            say(succeeded(message, 'flagged'));
                            await onReload();
                          }}
                          onCancel={() => setCorrecting(null)}
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
    </section>
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
  const [rows, sort, sortBy] = useSort(players, ENTRANT_COLUMNS);
  if (players.length === 0) return <p className="muted">Everyone is paired.</p>;
  return (
    <div className="table-responsive">
      <table className="leaderboard-table admin-table">
        <caption className="visually-hidden">Round {round}&apos;s unpaired players</caption>
        <thead>
          <tr>
            <SortHeader col="player" sort={sort} onSort={sortBy}>
              Player
            </SortHeader>
            <SortHeader col="rating" sort={sort} onSort={sortBy}>
              Rating
            </SortHeader>
            <SortHeader col="zone" sort={sort} onSort={sortBy}>
              Time zone
            </SortHeader>
          </tr>
        </thead>
        <tbody>
          {rows.map((e) => (
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
function RoundsPanel({ at, ed, regions, onReload, outcome, say }: Props) {
  const [criteria, setCriteria] = useState({ timezone: true, rating: false });
  const dialog = useDialog();
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
  const view = (m: AdminBlitzMatch) => ({ a: nameOf(m.a), b: m.b === null ? null : nameOf(m.b), result: m.result, game: m.game, ruled: m.ruling !== '' });
  const ratingOf = (id: number) => {
    const r = byId.get(id)?.rating ?? null;
    return r === null ? '–' : String(Math.round(r));
  };
  const open = openRound(ed.rounds);
  const [matchSorts, setMatchSorts] = useState<Record<number, Sort<MatchColumn>>>({});
  const matchCols = matchColumns(nameOf, (id) => byId.get(id)?.rating ?? null);
  // A sorted row keeps i, its place in the round, which unpairing by hand goes by.
  const sortedOf = (r: AdminBlitzRound) => sortRows(shownOf(r).map((m, i) => ({ m, i })), matchCols, matchSorts[r.id] ?? null);
  const sortRound = (r: AdminBlitzRound) => (col: MatchColumn) => setMatchSorts((s) => ({ ...s, [r.id]: nextSort(s[r.id] ?? null, col, matchCols) }));
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
  const free = open === null ? [] : unpaired(ed.entrants, { number: open.number, matches: shownOf(open) });
  const counts = pairCounts(ed.entrants, open === null ? { number: 0, matches: [] } : { number: open.number, matches: shownOf(open) });
  // openSwiss is the round whose counts and unpaired players show; listingUnpaired, while its Unpaired view is chosen.
  const openSwiss = (r: AdminBlitzRound) => r.id === open?.id && r.kind === 'swiss';
  const listingUnpaired = (r: AdminBlitzRound) => openSwiss(r) && roundView === 'unpaired';
  const close = () => say(null);
  const run = async (r: req.Request, done: string, where: string, doneAt = where) => {
    const res = await req.send(r);
    say(outcomeOf(res, done, where, doneAt));
    await onReload();
    return res.ok;
  };
  const pair = async () => {
    if (action.kind === 'next') {
      await run(req.pairBlitzRound(at.slug, at.edition, criteria), `${roundTitle(action.round.number, ed.format)} paired as a draft.`, 'rounds');
      return;
    }
    if (action.kind !== 'fill' || open === null) return;
    const res = await req.send<AdminBlitzRound>(req.proposeBlitzPairing(at.slug, at.edition, open.id, { ...criteria, keep: asChanges(shownOf(open)) }));
    if (!res.ok) {
      say(refused(res.message, 'rounds'));
      return;
    }
    const same = shownAfter(open.matches, res.data.matches) === null;
    setShown(open, res.data.matches);
    say(succeeded(same ? 'Paired as saved: nothing to update.' : `Paired. Not saved yet: ${open.published_at === null ? 'publish' : 'update'} it, or discard.`, 'rounds'));
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
    if (changed(r) && !(await run(req.setBlitzRoundMatches(at.slug, at.edition, r.id, asChanges(shownOf(r))), 'Saved.', 'rounds'))) return;
    setPending(null);
    if (r.published_at === null) await run(req.publishBlitzRound(at.slug, at.edition, r.id), `${roundTitle(r.number, ed.format)} published.`, 'rounds');
    else if (changed(r)) say(succeeded(`${roundTitle(r.number, ed.format)} updated: the site shows it now.`, 'rounds'));
  };
  const remove = async (r: AdminBlitzRound) => {
    const title = roundTitle(r.number, ed.format);
    const what = r.published_at === null ? `the draft of ${title}` : `${title}, published but not yet played`;
    if (!(await dialog.confirm({ heading: `Delete ${what}?`, text: 'Pair again afterwards.', action: 'Delete round', final: true }))) return;
    setPending(null);
    await run(req.deleteBlitzRound(at.slug, at.edition, r.id), `${title} deleted.`, 'rounds');
  };
  const resets = resetsOf(ed.rounds);
  const resetResult = async (r: AdminBlitzRound, m: AdminBlitzMatch) => {
    const pair = `${nameOf(m.a)} v ${nameOf(m.b as number)}`;
    if (!(await dialog.confirm({ heading: `Reset ${pair}?`, text: 'Its result, ruling and game go, as if never played.', action: 'Reset game', final: true }))) return;
    setRecording(null);
    await run(req.resetBlitzResult(at.slug, at.edition, r.id, m.id), `${pair} reset.`, `match ${m.id}`);
  };
  const keepReport = async (r: AdminBlitzRound, m: AdminBlitzMatch) => {
    const pair = `${nameOf(m.a)} v ${nameOf(m.b as number)}`;
    if (!(await dialog.confirm({ heading: `Keep the reported result of ${pair}?`, text: 'The flag is resolved and the result stays as reported. Players still cannot change it; Reset opens it to them again.', action: 'Keep result' }))) return;
    await run(req.keepBlitzReport(at.slug, at.edition, r.id, m.id), `${pair}: report kept.`, `match ${m.id}`);
  };
  const resetRound = async (r: AdminBlitzRound) => {
    const title = roundTitle(r.number, ed.format);
    const sure = await dialog.confirm({
      heading: `Reset every result of ${title}?`,
      text: 'Its results, rulings and games go; the pairing stays, and can then be deleted.',
      action: 'Reset round',
      final: true,
    });
    if (!sure) return;
    setRecording(null);
    await run(req.resetBlitzRound(at.slug, at.edition, r.id), `${title}'s results reset.`, `round ${r.id}`);
  };
  const resetEdition = async () => {
    const games = ed.rounds.flatMap((r) => r.matches).filter((m) => m.game !== null).length;
    const what = `${ed.rounds.length} round${ed.rounds.length === 1 ? '' : 's'} and ${games} recorded game${games === 1 ? '' : 's'}`;
    const sure = await dialog.confirm({
      heading: `Reset ${ed.name} ${ed.edition}?`,
      text: `This deletes ${what}, puts every dropped player back in and clears every skip. Entrants, organisers, format, points and thread text stay. It cannot be undone.`,
      action: 'Reset edition',
      final: true,
    });
    if (!sure) return;
    setPending(null);
    setRecording(null);
    await run(req.resetBlitzEdition(at.slug, at.edition), `${ed.name} ${ed.edition} reset: pair round 1 again.`, 'reset edition', 'rounds');
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
      {dialog.element}
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
            <button type="button" className="btn-antique" onClick={() => void remove(open)}>
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
      <Notice outcome={shownAt(outcome, 'rounds')} onClose={close} />
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
            {resets.round === r.id && !changed(r) && (
              <>
                {' '}
                <button type="button" className="btn-antique btn-settings" onClick={() => void resetRound(r)}>
                  Reset results
                </button>
              </>
            )}
          </p>
          <RoundThreads at={at} ed={ed} round={r} regions={regions} unsaved={changed(r)} onReload={onReload} say={say} />
          <Notice outcome={shownAt(outcome, `round ${r.id}`)} onClose={close} />
          {shown(r) && (
            <>
              {openSwiss(r) && (
                <>
                  <p className="muted round-counts">
                    {counts.paired} paired · {counts.unpaired} unpaired{counts.skipping > 0 && ` · ${counts.skipping} skipping`}
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
                        <SortHeader col="player" sort={matchSorts[r.id] ?? null} onSort={sortRound(r)}>
                          Player
                        </SortHeader>
                        <SortHeader col="rating" sort={matchSorts[r.id] ?? null} onSort={sortRound(r)}>
                          Rating
                        </SortHeader>
                        <SortHeader col="opponent" sort={matchSorts[r.id] ?? null} onSort={sortRound(r)}>
                          Opponent
                        </SortHeader>
                        <SortHeader col="opponentRating" sort={matchSorts[r.id] ?? null} onSort={sortRound(r)}>
                          Rating
                        </SortHeader>
                        <SortHeader col="clocks" sort={matchSorts[r.id] ?? null} onSort={sortRound(r)}>
                          Clocks
                        </SortHeader>
                        <SortHeader col="gap" sort={matchSorts[r.id] ?? null} onSort={sortRound(r)}>
                          Rating gap
                        </SortHeader>
                        <SortHeader col="result" sort={matchSorts[r.id] ?? null} onSort={sortRound(r)}>
                          Result
                        </SortHeader>
                        <th scope="col">
                          <span className="visually-hidden">Actions</span>
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {sortedOf(r).map(({ m, i }) => (
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
                              {m.b !== null && m.result === null ? <span className="muted">not yet</span> : resultSummary(view(m))}
                              {turnOrder(view(m)) !== '' && <span className="row-detail muted">{turnOrder(view(m))}</span>}
                              {m.ruling !== '' && <span className="row-detail muted">{m.ruling}</span>}
                              {reportNote(m, nameOf) !== '' && <span className={m.report === 'flagged' ? 'row-detail row-flagged' : 'row-detail muted'}>{reportNote(m, nameOf)}</span>}
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
                              {m.report === 'flagged' && !changed(r) && (
                                <button type="button" className="btn-antique" onClick={() => void keepReport(r, m)}>
                                  Keep
                                </button>
                              )}
                              {resets.matches.includes(m.id) && !changed(r) && (
                                <button type="button" className="btn-antique" onClick={() => void resetResult(r, m)}>
                                  Reset
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
                                    say(succeeded(message, `match ${m.id}`));
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
                                    say(succeeded(message, `match ${m.id}`));
                                    await onReload();
                                  }}
                                  onCancel={() => setReplacing(null)}
                                />
                              </td>
                            </tr>
                          )}
                          <NoticeRow outcome={shownAt(outcome, `match ${m.id}`)} columns={8} onClose={close} />
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
      {canResetEdition(ed) && (
        <div className="admin-subsection">
          <h3>Reset the tournament</h3>
          <p className="muted">
            Starts the edition again from its entrants: every round, result and game goes, every dropped player is in
            again and every skip goes. Entrants and their details, organisers, format, points and thread text stay.
            To undo less, reset a round&apos;s results (latest round only), then delete the round.
          </p>
          <button type="button" className="btn-antique" onClick={() => void resetEdition()}>
            Reset {ed.name} {ed.edition}
          </button>
          <Notice outcome={shownAt(outcome, 'reset edition')} onClose={close} />
        </div>
      )}
    </div>
  );
}

/** EntrantsPanel lists every entrant with what admin knows of them and where they stand in the newest round, and edits one, or removes, drops, reinstates one or has them skip the next round. */
function EntrantsPanel({ at, ed, regions, countries, onReload, outcome, say }: Props) {
  const actions = (e: AdminBlitzEntrant) => entrantActions(e, ed);
  const [entrants, sort, sortBy] = useSort(ed.entrants, ADMIN_ENTRANT_COLUMNS);
  const nameOf = (id: number) => ed.entrants.find((x) => x.id === id)?.name ?? `entrant ${id}`;
  const dialog = useDialog();
  const [editing, setEditing] = useState<{ id: number; discord_name: string; email: string; country: string; zone: string } | null>(null);
  const change = async (e: AdminBlitzEntrant, status: AdminBlitzEntrant['status']) => {
    const r = await req.send(req.setBlitzEntrant(at.slug, at.edition, e.id, { status }));
    say(outcomeOf(r, `${e.name} is ${status === 'active' ? 'in again' : 'dropped'}.`, `entrant ${e.id}`));
    await onReload();
  };
  const reinstate = async (e: AdminBlitzEntrant) => {
    if (e.status === 'dropped') {
      await change(e, 'active');
      return;
    }
    const r = await req.send(req.unskipBlitzRound(at.slug, at.edition, e.id));
    say(outcomeOf(r, `${e.name} plays the next round after all.`, `entrant ${e.id}`));
    await onReload();
  };
  const skip = async (e: AdminBlitzEntrant, round: number) => {
    const left = leftUnpaired(e.id, ed.rounds.filter((r) => r.number === round)).map(nameOf);
    const unpaired = left.length === 0 ? '' : ` ${left.join(' and ')} will be unpaired.`;
    if (!(await dialog.confirm({ heading: `${e.name} skips round ${round}?`, text: `They score the edition's skip points for it.${unpaired}`, action: 'Skip' }))) return;
    const r = await req.send(req.skipBlitzRound(at.slug, at.edition, e.id));
    say(outcomeOf(r, `${e.name} skips round ${round}.${left.length === 0 ? '' : ` ${left.join(' and ')} ${left.length === 1 ? 'is' : 'are'} unpaired now: Pair fills them in.`}`, `entrant ${e.id}`));
    await onReload();
  };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (editing === null) return;
    const r = await req.send(
      req.setBlitzEntrant(at.slug, at.edition, editing.id, { discord_name: editing.discord_name, email: editing.email, country: editing.country, zone: editing.zone }),
    );
    say(outcomeOf(r, 'Saved.', `entrant ${editing.id}`));
    if (r.ok) setEditing(null);
    await onReload();
  };
  const remove = async (e: AdminBlitzEntrant) => {
    const left = leftUnpaired(e.id, ed.rounds).map((id) => ed.entrants.find((x) => x.id === id)?.name ?? `entrant ${id}`);
    const unpaired = left.length === 0 ? '' : ` ${left.join(' and ')} will be unpaired.`;
    const sure = await dialog.confirm({ heading: `Remove ${e.name} from the edition?`, text: unpaired === '' ? undefined : unpaired.trim(), action: 'Remove', final: true });
    if (!sure) return;
    const r = await req.send(req.removeBlitzEntrant(at.slug, at.edition, e.id));
    say(outcomeOf(r, `Removed ${e.name}.${left.length === 0 ? '' : ` ${left.join(' and ')} ${left.length === 1 ? 'is' : 'are'} unpaired now: Pair fills them in.`}`, `entrant ${e.id}`, 'entrants'));
    await onReload();
  };
  return (
    <div className="admin-subsection">
      {dialog.element}
      <h3>Entrants</h3>
      <p className="muted">
        A name the ratings know is shown in their spelling, with the 2-player rating of the current run. Everyone
        entered is on the site at once. Drop someone who withdrew once they have played: their results stand and they
        are not paired again. Remove someone who has not played, such as a sign-up who did not confirm: they come out
        of the round they are paired in. Skip leaves someone out of the next Swiss round only, scoring the
        edition&apos;s skip points for it (out of a draft too, leaving their opponent unpaired); Reinstate undoes it until
        that round is published. Skips counts the rounds skipped, the next one included.
      </p>
      <Notice outcome={shownAt(outcome, 'entrants')} onClose={() => say(null)} />
      {ed.entrants.length === 0 && <p className="muted">No entrants yet.</p>}
      {ed.entrants.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Every entrant, in sign-up order</caption>
            <thead>
              <tr>
                <SortHeader col="player" sort={sort} onSort={sortBy}>
                  Username
                </SortHeader>
                <SortHeader col="discord" sort={sort} onSort={sortBy}>
                  Discord
                </SortHeader>
                <SortHeader col="email" sort={sort} onSort={sortBy}>
                  Email
                </SortHeader>
                <SortHeader col="country" sort={sort} onSort={sortBy}>
                  Country
                </SortHeader>
                <SortHeader col="zone" sort={sort} onSort={sortBy}>
                  Time zone
                </SortHeader>
                <SortHeader col="rating" sort={sort} onSort={sortBy}>
                  Rating
                </SortHeader>
                <SortHeader col="status" sort={sort} onSort={sortBy}>
                  Status
                </SortHeader>
                <SortHeader col="skips" sort={sort} onSort={sortBy}>
                  Skips
                </SortHeader>
                <th scope="col">
                  <span className="visually-hidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {entrants.map((e) => (
                <Fragment key={e.id}>
                  {editing?.id === e.id ? (
                    <tr>
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
                      <td>{entrantState(e, ed.rounds, nameOf)}</td>
                      <td>{e.skips.length}</td>
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
                    <tr>
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
                      <td>{entrantState(e, ed.rounds, nameOf)}</td>
                      <td>{e.skips.length}</td>
                      <td className="run-buttons">
                        <button type="button" className="btn-antique" onClick={() => setEditing({ id: e.id, discord_name: e.discord_name, email: e.email, country: e.country ?? '', zone: e.zone })}>
                          Edit
                        </button>
                        {actions(e).action === 'reinstate' && (
                          <button type="button" className="btn-antique" onClick={() => void reinstate(e)}>
                            Reinstate
                          </button>
                        )}
                        {actions(e).action === 'drop' && (
                          <button type="button" className="btn-antique" onClick={() => void change(e, 'dropped')}>
                            Drop
                          </button>
                        )}
                        {actions(e).action === 'remove' && (
                          <button type="button" className="btn-antique" onClick={() => void remove(e)}>
                            Remove
                          </button>
                        )}
                        {actions(e).skip !== null && (
                          <button type="button" className="btn-antique" onClick={() => void skip(e, actions(e).skip as number)}>
                            Skip R{actions(e).skip}
                          </button>
                        )}
                      </td>
                    </tr>
                  )}
                  <NoticeRow outcome={shownAt(outcome, `entrant ${e.id}`)} columns={9} onClose={() => say(null)} />
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

/** AddEntrantPanel enters someone found by searching the members not yet entered and the players the ratings know, or someone new by the name typed. Their details fill in where known; what the form then holds is saved on them. */
function AddEntrantPanel({ at, ed, regions, countries, onReload, outcome, say }: Props) {
  const enter = async (details: PersonDetails) => {
    const r = await req.send<BlitzEntrantIds>(req.addBlitzEntrants(at.slug, at.edition, [entrantToAdd(details)]));
    say(outcomeOf(r, `Entered ${details.name.trim()}.`, 'add entrant'));
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
      />
      <Notice outcome={shownAt(outcome, 'add entrant')} onClose={() => say(null)} />
    </div>
  );
}

type View = 'rounds' | 'entrants' | 'standings';

/** EditionTabs shows one of the format and rounds, the entrants (with the form to add one first) or the standings, chosen by its tabs, the site's sub tabs. */
function EditionTabs(props: Props) {
  const [hash, setHash] = useHash();
  const names = new Map(props.ed.entrants.map((e) => [e.id, e.name]));
  const nameOf = (id: number) => names.get(id) ?? '?';
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
                <EditionSettings {...props} />
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
          { value: 'standings', label: 'Standings', panel: <StandingsPanel standings={byFinish(props.ed.standings, latestFinishes(finishRounds(props.ed.rounds, nameOf)))} scoring={scoringSentence(props.ed.points, props.ed.fide_points)} /> },
        ]}
        chosen={hashTab<View>(['rounds', 'entrants', 'standings'], hash)}
        onChoose={setHash}
      />
    </section>
  );
}

/** EditionSettings sums up the edition's settings in a list, with the format's warnings, under one Edit settings button in its header that opens every setting's form together. Each form saves on its own; closed, the forms stay mounted, so what is half typed is kept. */
function EditionSettings(props: Props) {
  const [editing, setEditing] = useState(false);
  return (
    <div className="admin-subsection">
      <div className="settings-head">
        <h3>Settings</h3>
        <button type="button" className="btn-antique btn-settings" aria-expanded={editing} aria-controls="edition-settings-forms" onClick={() => setEditing(!editing)}>
          {editing ? 'Close settings' : 'Edit settings'}
        </button>
      </div>
      {!editing && (
        <>
          <dl className="settings-summary">
            {settingsRows(props.ed).map((r) => (
              <div key={r.key}>
                <dt>{r.key}</dt>
                <dd>{r.value}</dd>
              </div>
            ))}
          </dl>
          <FormatWarnings warnings={editionFormatWarnings(props.ed)} />
        </>
      )}
      <div id="edition-settings-forms" className="settings-forms" hidden={!editing}>
        <h4>Format</h4>
        <FormatPanel {...props} />
        <h4>Points</h4>
        <PointsPanel key={JSON.stringify(props.ed.points)} {...props} />
        <h4>Organisers</h4>
        <OrganisersPanel {...props} />
        <h4>Thread text</h4>
        <ThreadTextPanel key={`${props.ed.thread_text.message}\n${props.ed.thread_text.no_discord}`} {...props} />
      </div>
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
function OrganisersPanel({ at, ed, onReload, outcome, say }: Props) {
  const [typed, setTyped] = useState('');
  const found = usePeopleSearch('/api/people', typed, false, ed);
  const choices = organiserChoices(found, ed.organisers);
  const change = async (r: req.Request, done: string) => {
    const res = await req.send(r);
    say(outcomeOf(res, done, 'organisers'));
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
      <Notice outcome={shownAt(outcome, 'organisers')} onClose={() => say(null)} />
    </>
  );
}

/** ThreadTextPanel edits what each game's Discord thread says, the welcome message and the sentence for a player without Discord, and resets it to the default. Keyed by the saved text, so a save or reset starts the form again from it. */
function ThreadTextPanel({ at, ed, onReload, outcome, say }: Props) {
  const saved = ed.thread_text;
  const [form, setForm] = useState<ThreadText>({ message: saved.message, no_discord: saved.no_discord });
  const send = async (r: req.Request, done: string) => {
    const res = await req.send(r);
    say(outcomeOf(res, done, 'thread text'));
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
      <Notice outcome={shownAt(outcome, 'thread text')} onClose={() => say(null)} />
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
    say(outcomeOf(r, `${roundTitle(round.number, ed.format)}: deadline saved.`, `round ${round.id}`));
    if (r.ok) setForm(null);
    await onReload();
  };
  const download = async () => {
    const r = await adminDownload(req.blitzThreadsPath(at.slug, at.edition, round.id));
    if (!r.ok) {
      say(refused(r.message, `round ${round.id}`));
      return;
    }
    const url = URL.createObjectURL(r.data);
    const a = document.createElement('a');
    a.href = url;
    a.download = threadsFileName(at.slug, at.edition, round.number);
    a.click();
    URL.revokeObjectURL(url);
    say(succeeded(`${roundTitle(round.number, ed.format)}: Discord threads downloaded.`, `round ${round.id}`));
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
function PointsPanel({ at, ed, onReload, outcome, say }: Props) {
  const [form, setForm] = useState<PointsForm>(pointsForm(ed.points));
  const locked = ed.rounds.some((r) => r.kind === 'knockout');
  const send = async (r: req.Request, done: string) => {
    const res = await req.send(r);
    say(outcomeOf(res, done, 'points'));
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
      <Notice outcome={shownAt(outcome, 'points')} onClose={() => say(null)} />
    </>
  );
}

/** FormatPanel sets the edition's Swiss rounds and knockout size. */
function FormatPanel({ at, ed, onReload, outcome, say }: Props) {
  const [format, setFormat] = useState(ed.format);
  const locked = ed.rounds.some((r) => r.kind === 'knockout');
  const advice = formatAdvice(format, ed.entrants.filter((e) => e.status === 'active').length);
  const save = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send(req.setBlitzFormat(at.slug, at.edition, format));
    say(outcomeOf(r, `Format saved: ${formatLabel(format)}.`, 'format'));
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
      <Notice outcome={shownAt(outcome, 'format')} onClose={() => say(null)} />
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
    a_turn: m.game.a_turn,
    b_culture: String(m.game.b_culture),
    b_outcome: m.game.b_outcome,
    b_turn: m.game.b_turn,
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

/** ResultForm records a match's result: the game played (share code, each player's outcome, culture and turn, when it finished and in which time zone), or the organisers' ruling with its reason. A refusal shows in the form. */
function ResultForm({ at, round, match, nameOf, regions, zone, onZone, onDone, onCancel }: ResultFormProps) {
  const [refusal, setRefusal] = useState<Outcome | null>(null);
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
        setRefusal(refused(r.message, ''));
        return;
      }
      const res = await req.send(req.recordBlitzGame(at.slug, at.edition, round.id, match.id, r.body));
      if (!res.ok) {
        setRefusal(refused(res.message, ''));
        return;
      }
      onZone(r.body.zone);
      await onDone(`${a} v ${b} recorded.`);
      return;
    }
    if (ruling.reason.trim() === '') {
      setRefusal(refused('A ruling needs a reason.', ''));
      return;
    }
    const res = await req.send(req.recordBlitzRuling(at.slug, at.edition, round.id, match.id, ruling));
    if (res.ok) await onDone(`${a} v ${b} ruled.`);
    else setRefusal(refused(res.message, ''));
  };
  // side is one player's row: how their game ended, their culture (0 for one who resigned or timed out), then their turn.
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
      <label>
        {name}&apos;s turn
        <select className="input-antique" value={game[`${who}_turn`] ?? ''} onChange={(e) => setGame(chooseTurn(game, who, Number(e.target.value)))}>
          {game[`${who}_turn`] === null && (
            <option value="" disabled>
              –
            </option>
          )}
          {TURNS.map((t) => (
            <option key={t} value={t}>
              {turnMark(t)}
              {t === 1 ? ' (went first)' : ''}
            </option>
          ))}
        </select>
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
              Share code, if any
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
        <>
          <fieldset className="ruling-choices">
            <legend>Ruling</legend>
            {choices.map((c) => (
              <label key={c} className="setting-inline">
                <input type="radio" name={`ruling-${match.id}`} checked={ruling.result === c} onChange={() => setRuling({ ...ruling, result: c })} />
                {rulingLabel(c, seededName(a, match.a_seed), seededName(b, match.b_seed), match.game !== null)}
              </label>
            ))}
          </fieldset>
          <div className="result-row">
            <label>
              Reason
              <input className="input-antique" value={ruling.reason} maxLength={1000} onChange={(e) => setRuling({ ...ruling, reason: e.target.value })} />
            </label>
          </div>
        </>
      )}
      <Notice outcome={refusal} onClose={() => setRefusal(null)} />
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
  const [refusal, setRefusal] = useState<Outcome | null>(null);
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
        const added = await req.send<BlitzEntrantIds>(req.addBlitzEntrants(at.slug, at.edition, [entrantToAdd(picked.fill)]));
        if (!added.ok) {
          setRefusal(refused(added.message, ''));
          return;
        }
        id = added.data.ids[0];
      }
    }
    if (id === null) return;
    const name = how === 'reserve' ? nameOf(id) : (picked?.fill.name ?? nameOf(id));
    const res = await req.send(req.replaceBlitzPlayer(at.slug, at.edition, round.id, match.id, { out, in: id }));
    if (res.ok) await onDone(`${name} takes ${nameOf(out)}'s place.`);
    else setRefusal(refused(res.message, ''));
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
      <Notice outcome={refusal} onClose={() => setRefusal(null)} />
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

/** StandingsPanel shows the standings as the site does, in the order given: how the edition scores, then place, points, games played, byes and Buchholz, dropped players last. */
function StandingsPanel({ standings, scoring }: { standings: BlitzStanding[]; scoring: string }) {
  const [rows, sort, sortBy] = useSort(standings, STANDING_COLUMNS);
  return (
    <>
      <p className="muted">
        {scoring} Ties are broken by the Swiss tie-breaks: Direct Encounter (the games
        among the tied players), then Buchholz (the sum of the opponents&apos; scores; a bye, a forfeit or a missed round
        counts as a game against an opponent on the player&apos;s own score). Players still level share a place, such as
        1–12, listed by when their game in the latest round finished; where one order is needed, as for the knockout, a
        draw fixed for the edition decides.
      </p>
      {standings.length === 0 && <p className="muted">Nobody to rank yet.</p>}
      {standings.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">The standings</caption>
            <thead>
              <tr>
                <SortHeader col="place" sort={sort} onSort={sortBy}>
                  Rank
                </SortHeader>
                <SortHeader col="player" sort={sort} onSort={sortBy}>
                  Player
                </SortHeader>
                <SortHeader col="points" sort={sort} onSort={sortBy}>
                  Points
                </SortHeader>
                <SortHeader col="played" sort={sort} onSort={sortBy}>
                  Played
                </SortHeader>
                <SortHeader col="byes" sort={sort} onSort={sortBy}>
                  Byes
                </SortHeader>
                <SortHeader col="buchholz" sort={sort} onSort={sortBy}>
                  Buchholz
                </SortHeader>
              </tr>
            </thead>
            <tbody>
              {rows.map((s) => (
                <tr key={s.name} className={s.status === 'dropped' ? 'muted' : ''}>
                  <td>{placeLabel(s)}</td>
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
