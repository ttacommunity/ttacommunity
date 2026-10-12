/** A blitz edition's page: its format, its entrants (added one at a time, dropped, removed), its rounds (paired, changed by hand, published, their results recorded), the standings, and what players see. */
import { useEffect, useState, type FormEvent } from 'react';
import { formatLabel, offsetLabel, resultSummary, zoneOptions } from '@ttac/shared/blitz';
import { formatInstant } from '@ttac/shared/time';
import { adminRequest, firstError, type AdminBlitzEdition, type AdminMe, type BlitzEntrantIds, type Load } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { tabsFor } from '../lib/admin-tabs';
import { editionParams, playersView, type AdminBlitzEntrant } from '../lib/blitz-page';
import { emptyGameForm, gameRecordFrom, type BlitzStanding, type GameForm } from '../lib/blitz-results';
import { canPair, editableRound, newestFirst, undecided, unpaired, withBye, withPair, withoutMatch, zoneGapLabel, type AdminBlitzMatch, type AdminBlitzRound } from '../lib/blitz-rounds';
import AdminHeader from './AdminHeader';
import PageError from './PageError';
import './AdminApp.css';

const KNOCKOUTS = [0, 2, 4, 8, 16, 32];

/** knownZones is every IANA zone the browser knows, or none in one that cannot say. */
function knownZones(): string[] {
  try {
    return Intl.supportedValuesOf('timeZone');
  } catch {
    return [];
  }
}

type Edition = { slug: string; edition: string };
type Props = { at: Edition; ed: AdminBlitzEdition; onReload: () => Promise<void>; say: (message: string) => void };

/** BlitzEditionPage loads who is signed in and the edition the address names (?t=&e=), then its cards. */
export default function BlitzEditionPage() {
  const [me, setMe] = useState<Load<AdminMe>>({ kind: 'loading' });
  const [ed, setEd] = useState<Load<AdminBlitzEdition>>({ kind: 'loading' });
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
              <p className="muted">
                <a href="/#blitz">← Blitz tournaments</a>
              </p>
              <h2 id="page-title" className="tournament-title">
                {ed.data.name} {ed.data.edition}
              </h2>
              <p className="muted">
                {formatLabel(ed.data.format)}. {count('active')} in, {count('pending')} pending, {count('dropped')} dropped.
              </p>
            </section>
            {message !== '' && (
              <p className="admin-message" role="status">
                {message}
              </p>
            )}
            <RoundsCard at={at} ed={ed.data} onReload={load} say={setMessage} />
            <StandingsCard standings={ed.data.standings} />
            <EntrantsCard at={at} ed={ed.data} onReload={load} say={setMessage} />
            <AddEntrantCard at={at} ed={ed.data} onReload={load} say={setMessage} />
            <FormatCard at={at} ed={ed.data} onReload={load} say={setMessage} />
            <PlayersViewCard entrants={ed.data.entrants} />
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

/** RoundsCard pairs the next round, shows every round newest first, and lets the latest be changed by hand, published or deleted. */
function RoundsCard({ at, ed, onReload, say }: Props) {
  const [criteria, setCriteria] = useState({ timezone: true, rating: false });
  const [pick, setPick] = useState({ a: '', b: '', bye: '' });
  const [recording, setRecording] = useState<{ round: number; match: number } | null>(null);
  const [zone] = useState(viewerZone);
  const byId = new Map(ed.entrants.map((e) => [e.id, e]));
  const nameOf = (id: number) => byId.get(id)?.name ?? `entrant ${id}`;
  const summary = (m: AdminBlitzMatch) => resultSummary({ a: nameOf(m.a), b: m.b === null ? null : nameOf(m.b), result: m.result, game: m.game, ruled: m.ruling !== '' });
  const blockers = undecided(ed.rounds, nameOf);
  const ratingOf = (id: number) => {
    const r = byId.get(id)?.rating ?? null;
    return r === null ? '–' : String(Math.round(r));
  };
  const editable = editableRound(ed.rounds);
  const run = async (r: req.Request, done: string) => {
    const res = await req.send(r);
    say(res.ok ? done : res.message);
    await onReload();
  };
  const pair = (e: FormEvent) => {
    e.preventDefault();
    void run(req.pairBlitzRound(at.slug, at.edition, criteria), `Round ${ed.rounds.length + 1} paired as a draft.`);
  };
  const addPair = (r: AdminBlitzRound) => {
    const a = Number(pick.a);
    const b = Number(pick.b);
    if (!a || !b || a === b) return;
    setPick({ ...pick, a: '', b: '' });
    void run(req.setBlitzRoundMatches(at.slug, at.edition, r.id, withPair(r.matches, a, b)), `${nameOf(a)} plays ${nameOf(b)}.`);
  };
  const giveBye = (r: AdminBlitzRound) => {
    const a = Number(pick.bye);
    if (!a) return;
    setPick({ ...pick, bye: '' });
    void run(req.setBlitzRoundMatches(at.slug, at.edition, r.id, withBye(r.matches, a)), `${nameOf(a)} has the bye.`);
  };
  const unpair = (r: AdminBlitzRound, id: number) =>
    void run(req.setBlitzRoundMatches(at.slug, at.edition, r.id, withoutMatch(r.matches, id)), 'Unpaired.');
  const pairable = canPair(ed.entrants, ed.rounds);
  return (
    <section className="rating-card admin-card" aria-labelledby="rounds-heading">
      <h2 id="rounds-heading" className="rating-card-heading">
        Rounds
      </h2>
      <p className="muted">
        Pairing puts players on equal points together, never with an opponent they have met; within a score group
        the pairs are the best time-zone fit (and the closest ratings, when ticked), chance deciding the rest. With an
        odd count the lowest-ranked player without a bye sits out. A round is a draft you can change by hand until
        you publish it; the latest round can still be changed afterwards.
      </p>
      <form className="admin-add" onSubmit={pair}>
        <h3>Pair round {ed.rounds.length + 1}</h3>
        <label className="setting-inline">
          <input type="checkbox" checked={criteria.timezone} onChange={(e) => setCriteria({ ...criteria, timezone: e.target.checked })} />
          Time-zone fit
        </label>
        <label className="setting-inline">
          <input type="checkbox" checked={criteria.rating} onChange={(e) => setCriteria({ ...criteria, rating: e.target.checked })} />
          Closest ratings
        </label>
        <button type="submit" className="btn-antique" disabled={!pairable}>
          Pair
        </button>
        {!pairable && blockers.length > 0 && <span className="muted">Waiting for results: {blockers.join(', ')}.</span>}
        {!pairable && blockers.length === 0 && <span className="muted">Needs two active entrants and no draft open.</span>}
      </form>
      {ed.rounds.length === 0 && <p className="muted">No rounds yet.</p>}
      {newestFirst(ed.rounds).map((r) => (
        <div key={r.id} className="round-block">
          <h3>
            Round {r.number} · {r.published_at === null ? 'draft' : `published ${formatInstant(r.published_at, zone)}`}
          </h3>
          {r.matches.length === 0 && <p className="muted">No matches.</p>}
          {r.matches.length > 0 && (
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
                  {r.matches.map((m) => (
                    <tr key={m.id} className={m.repeat ? 'row-error' : ''}>
                      <th scope="row">{nameOf(m.a)}</th>
                      <td>{ratingOf(m.a)}</td>
                      <td>{m.b === null ? <em>bye</em> : nameOf(m.b)}</td>
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
                        {m.b !== null && (
                          <button type="button" className="btn-antique" onClick={() => setRecording({ round: r.id, match: m.id })}>
                            {m.result === null ? 'Record' : 'Correct'}
                          </button>
                        )}
                        {r.id === editable && m.result === null && (
                          <button type="button" className="btn-antique" onClick={() => unpair(r, m.id)}>
                            Unpair
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {recording?.round === r.id && r.matches.some((m) => m.id === recording.match) && (
            <ResultForm
              at={at}
              round={r}
              match={r.matches.find((m) => m.id === recording.match) as AdminBlitzMatch}
              nameOf={nameOf}
              onDone={async () => {
                setRecording(null);
                await onReload();
              }}
              onCancel={() => setRecording(null)}
              say={say}
            />
          )}
          {r.id === editable && (
            <div className="admin-add">
              <h3>Change by hand</h3>
              {unpaired(ed.entrants, r).length === 0 && <span className="muted">Everyone active is in a match.</span>}
              {unpaired(ed.entrants, r).length > 0 && (
                <>
                  <label>
                    Pair
                    <select className="input-antique" value={pick.a} onChange={(e) => setPick({ ...pick, a: e.target.value })}>
                      <option value="">choose</option>
                      {unpaired(ed.entrants, r).map((e) => (
                        <option key={e.id} value={e.id}>
                          {e.name}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    with
                    <select className="input-antique" value={pick.b} onChange={(e) => setPick({ ...pick, b: e.target.value })}>
                      <option value="">choose</option>
                      {unpaired(ed.entrants, r).map((e) => (
                        <option key={e.id} value={e.id}>
                          {e.name}
                        </option>
                      ))}
                    </select>
                  </label>
                  <button type="button" className="btn-antique" disabled={!pick.a || !pick.b || pick.a === pick.b} onClick={() => addPair(r)}>
                    Pair them
                  </button>
                  <label>
                    Bye to
                    <select className="input-antique" value={pick.bye} onChange={(e) => setPick({ ...pick, bye: e.target.value })}>
                      <option value="">choose</option>
                      {unpaired(ed.entrants, r).map((e) => (
                        <option key={e.id} value={e.id}>
                          {e.name}
                        </option>
                      ))}
                    </select>
                  </label>
                  <button type="button" className="btn-antique" disabled={!pick.bye} onClick={() => giveBye(r)}>
                    Give the bye
                  </button>
                </>
              )}
              {r.published_at === null && (
                <>
                  <button type="button" className="btn-antique" onClick={() => void run(req.publishBlitzRound(at.slug, at.edition, r.id), `Round ${r.number} published.`)}>
                    Publish
                  </button>
                  <button
                    type="button"
                    className="btn-antique"
                    onClick={() => {
                      if (window.confirm(`Delete the draft of round ${r.number}?`)) void run(req.deleteBlitzRound(at.slug, at.edition, r.id), 'Draft deleted.');
                    }}
                  >
                    Delete draft
                  </button>
                </>
              )}
            </div>
          )}
        </div>
      ))}
    </section>
  );
}

/** EntrantsCard lists every entrant with what admin knows of them, and edits, drops, reinstates or removes one. */
function EntrantsCard({ at, ed, onReload, say }: Props) {
  const [editing, setEditing] = useState<{ id: number; discord_name: string; zone: string } | null>(null);
  const [known] = useState(knownZones);
  const change = async (e: AdminBlitzEntrant, status: AdminBlitzEntrant['status']) => {
    const r = await req.send(req.setBlitzEntrant(at.slug, at.edition, e.id, { status }));
    say(r.ok ? `${e.name} is ${status}.` : r.message);
    await onReload();
  };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (editing === null) return;
    const r = await req.send(req.setBlitzEntrant(at.slug, at.edition, editing.id, { discord_name: editing.discord_name, zone: editing.zone }));
    say(r.ok ? 'Saved.' : r.message);
    if (r.ok) setEditing(null);
    await onReload();
  };
  const remove = async (e: AdminBlitzEntrant) => {
    if (!window.confirm(`Remove ${e.name} from the edition? Use Drop for someone who withdrew.`)) return;
    const r = await req.send(req.removeBlitzEntrant(at.slug, at.edition, e.id));
    say(r.ok ? `Removed ${e.name}.` : r.message);
    await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="entrants-heading">
      <h2 id="entrants-heading" className="rating-card-heading">
        Entrants
      </h2>
      <p className="muted">
        A name the ratings know is shown in their spelling, with the 2-player rating of the current run. Pending
        players are named but not confirmed: neither paired nor shown on the site until you confirm them. Drop someone
        who withdrew: their results stand and they are not paired again. Remove only a mistaken entry.
      </p>
      {ed.entrants.length === 0 && <p className="muted">No entrants yet. Add them below.</p>}
      {ed.entrants.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Every entrant, in sign-up order</caption>
            <thead>
              <tr>
                <th scope="col">Username</th>
                <th scope="col">Discord</th>
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
                        aria-label={`Discord name of ${e.name}`}
                        value={editing.discord_name}
                        maxLength={100}
                        onChange={(ev) => setEditing({ ...editing, discord_name: ev.target.value })}
                      />
                    </td>
                    <td>
                      <select
                        className="input-antique"
                        aria-label={`Time zone of ${e.name}`}
                        value={editing.zone}
                        onChange={(ev) => setEditing({ ...editing, zone: ev.target.value })}
                      >
                        {zoneOptions(known, editing.zone).map((z) => (
                          <option key={z} value={z}>
                            {z}
                          </option>
                        ))}
                      </select>
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
                    <td>
                      {e.zone} <span className="muted">{offsetLabel(e.utc_offset)}</span>
                    </td>
                    <td>{e.rating === null ? '–' : Math.round(e.rating)}</td>
                    <td>{e.status}</td>
                    <td className="run-buttons">
                      <button type="button" className="btn-antique" onClick={() => setEditing({ id: e.id, discord_name: e.discord_name, zone: e.zone })}>
                        Edit
                      </button>
                      {e.status === 'pending' && (
                        <button type="button" className="btn-antique" onClick={() => void change(e, 'active')}>
                          Confirm
                        </button>
                      )}
                      <button type="button" className="btn-antique" onClick={() => void change(e, e.status === 'dropped' ? 'active' : 'dropped')}>
                        {e.status === 'dropped' ? 'Reinstate' : 'Drop'}
                      </button>
                      <button type="button" className="btn-antique" onClick={() => void remove(e)}>
                        Remove
                      </button>
                    </td>
                  </tr>
                ),
              )}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

/** AddEntrantCard adds one entrant: the TTA username (kept in the ratings' spelling when known), Discord name and zone. */
function AddEntrantCard({ at, onReload, say }: Props) {
  const [one, setOne] = useState({ name: '', discord_name: '', zone: 'UTC', status: 'active' as 'active' | 'pending' });
  const [known] = useState(knownZones);
  const add = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send<BlitzEntrantIds>(req.addBlitzEntrants(at.slug, at.edition, [one]));
    if (r.ok) {
      say(`Added ${one.name}.`);
      setOne({ ...one, name: '', discord_name: '' });
      await onReload();
    } else say(r.message);
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="add-heading">
      <h2 id="add-heading" className="rating-card-heading">
        Add an entrant
      </h2>
      <p className="muted">The username as on CGE, their Discord name, and the time zone they play in (the sheet&apos;s EU, US and Asia are regions: pick the zone).</p>
      <form className="admin-add" onSubmit={(e) => void add(e)}>
        <label>
          Username
          <input className="input-antique" value={one.name} maxLength={100} required onChange={(e) => setOne({ ...one, name: e.target.value })} />
        </label>
        <label>
          Discord name
          <input className="input-antique" value={one.discord_name} maxLength={100} onChange={(e) => setOne({ ...one, discord_name: e.target.value })} />
        </label>
        <label>
          Time zone
          <select className="input-antique" value={one.zone} onChange={(e) => setOne({ ...one, zone: e.target.value })}>
            {zoneOptions(known, one.zone).map((z) => (
              <option key={z} value={z}>
                {z}
              </option>
            ))}
          </select>
        </label>
        <label className="setting-inline">
          <input type="checkbox" checked={one.status === 'pending'} onChange={(e) => setOne({ ...one, status: e.target.checked ? 'pending' : 'active' })} />
          Not confirmed yet
        </label>
        <button type="submit" className="btn-antique" disabled={one.name.trim() === ''}>
          Add
        </button>
      </form>
    </section>
  );
}

/** FormatCard sets the edition's Swiss rounds and knockout size. */
function FormatCard({ at, ed, onReload, say }: Props) {
  const [format, setFormat] = useState(ed.format);
  const save = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send(req.setBlitzFormat(at.slug, at.edition, format));
    say(r.ok ? `Format saved: ${formatLabel(format)}.` : r.message);
    await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="format-heading">
      <h2 id="format-heading" className="rating-card-heading">
        Format
      </h2>
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
        <button type="submit" className="btn-antique">
          Save
        </button>
      </form>
    </section>
  );
}

/** today is the day in the viewer's zone as YYYY-MM-DD, the result form's default. */
function today(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** formFromGame fills the game form from a recorded game, for a correction. */
function formFromGame(m: AdminBlitzMatch): GameForm {
  if (m.game === null) return emptyGameForm(today());
  return {
    share_code: m.game.share_code,
    a_culture: String(m.game.a_culture),
    a_outcome: m.game.a_outcome,
    b_culture: String(m.game.b_culture),
    b_outcome: m.game.b_outcome,
    played_on: m.game.played_on,
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
  onDone: () => Promise<void>;
  onCancel: () => void;
  say: (message: string) => void;
};

/** ResultForm records a match's result: the game played (share code, each player's culture and outcome, the day), or the organisers' ruling with its reason. */
function ResultForm({ at, round, match, nameOf, onDone, onCancel, say }: ResultFormProps) {
  const [kind, setKind] = useState<'game' | 'ruling'>(match.ruling !== '' ? 'ruling' : 'game');
  const [game, setGame] = useState<GameForm>(() => formFromGame(match));
  const [ruling, setRuling] = useState({ result: match.result ?? 'a', reason: match.ruling });
  const a = nameOf(match.a);
  const b = nameOf(match.b as number);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (kind === 'game') {
      const r = gameRecordFrom(game);
      if (!r.ok) {
        say(r.message);
        return;
      }
      const res = await req.send(req.recordBlitzGame(at.slug, at.edition, round.id, match.id, r.body));
      say(res.ok ? `${a} v ${b} recorded.` : res.message);
      if (res.ok) await onDone();
      return;
    }
    if (ruling.reason.trim() === '') {
      say('A ruling needs a reason.');
      return;
    }
    const res = await req.send(req.recordBlitzRuling(at.slug, at.edition, round.id, match.id, ruling));
    say(res.ok ? `${a} v ${b} ruled.` : res.message);
    if (res.ok) await onDone();
  };
  const side = (who: 'a' | 'b', name: string) => (
    <>
      <label>
        {name}&apos;s culture
        <input
          className="input-antique"
          type="number"
          min={0}
          step="any"
          value={game[`${who}_culture`]}
          disabled={game[`${who}_outcome`] !== 'finished'}
          onChange={(e) => setGame({ ...game, [`${who}_culture`]: e.target.value })}
        />
      </label>
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
    </>
  );
  return (
    <form className="admin-add" onSubmit={(e) => void submit(e)}>
      <h3>
        Result of {a} v {b}
      </h3>
      <label className="setting-inline">
        <input type="radio" name="kind" checked={kind === 'game'} onChange={() => setKind('game')} />
        A played game
      </label>
      <label className="setting-inline">
        <input type="radio" name="kind" checked={kind === 'ruling'} onChange={() => setKind('ruling')} />
        A ruling
      </label>
      {kind === 'game' && (
        <>
          <label>
            Share code
            <input className="input-antique" value={game.share_code} maxLength={32} onChange={(e) => setGame({ ...game, share_code: e.target.value })} />
          </label>
          {side('a', a)}
          {side('b', b)}
          <label>
            Played on
            <input className="input-antique" type="date" value={game.played_on} onChange={(e) => setGame({ ...game, played_on: e.target.value })} />
          </label>
        </>
      )}
      {kind === 'ruling' && (
        <>
          <label>
            Ruling
            <select className="input-antique" value={ruling.result} onChange={(e) => setRuling({ ...ruling, result: e.target.value as typeof ruling.result })}>
              <option value="a">{a} wins</option>
              <option value="b">{b} wins</option>
              <option value="draw">a draw</option>
              <option value="double_loss">both lose</option>
            </select>
          </label>
          <label>
            Reason
            <input className="input-antique" value={ruling.reason} maxLength={1000} onChange={(e) => setRuling({ ...ruling, reason: e.target.value })} />
          </label>
        </>
      )}
      <button type="submit" className="btn-antique">
        Save
      </button>
      <button type="button" className="btn-antique" onClick={onCancel}>
        Cancel
      </button>
    </form>
  );
}

/** StandingsCard shows the standings as the site does: rank, points, games, byes and opponents' points, dropped players last. */
function StandingsCard({ standings }: { standings: BlitzStanding[] }) {
  return (
    <section className="rating-card admin-card" aria-labelledby="standings-heading">
      <h2 id="standings-heading" className="rating-card-heading">
        Standings
      </h2>
      <p className="muted">
        A win is 1 point, a draw a half, a bye 1. Players on equal points who have all met are ordered by the games among
        them, then by their opponents&apos; points, then by a draw fixed for the edition.
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
                <th scope="col">Games</th>
                <th scope="col">Byes</th>
                <th scope="col">Opponents&apos; points</th>
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
                  <td>{s.opponents_points}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

/** PlayersViewCard shows the entrants as the site will: those still in, with username, zone and rating. */
function PlayersViewCard({ entrants }: { entrants: AdminBlitzEntrant[] }) {
  const shown = playersView(entrants);
  return (
    <section className="rating-card admin-card" aria-labelledby="players-view-heading">
      <h2 id="players-view-heading" className="rating-card-heading">
        What players see
      </h2>
      <p className="muted">The site shows the entrants still in: username, time zone and rating. Never a Discord name.</p>
      {shown.length === 0 && <p className="muted">Nobody yet.</p>}
      {shown.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">The entrants as the site lists them</caption>
            <thead>
              <tr>
                <th scope="col">Username</th>
                <th scope="col">Time zone</th>
                <th scope="col">Rating</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((e) => (
                <tr key={e.name}>
                  <th scope="row">{e.name}</th>
                  <td>
                    {offsetLabel(e.utc_offset)} <span className="muted">{e.zone}</span>
                  </td>
                  <td>{e.rating === null ? '–' : Math.round(e.rating)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
