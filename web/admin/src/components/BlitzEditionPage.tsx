/** A blitz edition's page: its format, its entrants (added one at a time, dropped, removed), and what players see. */
import { useEffect, useState, type FormEvent } from 'react';
import { formatLabel, offsetLabel, zoneOptions } from '@ttac/shared/blitz';
import { adminRequest, firstError, type AdminBlitzEdition, type AdminMe, type BlitzEntrantIds, type Load } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { tabsFor } from '../lib/admin-tabs';
import { editionParams, playersView, type AdminBlitzEntrant } from '../lib/blitz-page';
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
  const counts = ed.kind === 'ready' ? playersView(ed.data.entrants).length : 0;

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
                {formatLabel(ed.data.format)}. {counts} in, {ed.data.entrants.length - counts} dropped.
              </p>
            </section>
            {message !== '' && (
              <p className="admin-message" role="status">
                {message}
              </p>
            )}
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

/** EntrantsCard lists every entrant with what admin knows of them, and edits, drops, reinstates or removes one. */
function EntrantsCard({ at, ed, onReload, say }: Props) {
  const [editing, setEditing] = useState<{ id: number; discord_name: string; zone: string } | null>(null);
  const [known] = useState(knownZones);
  const change = async (e: AdminBlitzEntrant, status: 'active' | 'dropped') => {
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
        A name the ratings know is shown in their spelling, with the 2-player rating of the current run. Drop someone
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
                      <button type="button" className="btn-antique" onClick={() => void change(e, e.status === 'active' ? 'dropped' : 'active')}>
                        {e.status === 'active' ? 'Drop' : 'Reinstate'}
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
  const [one, setOne] = useState({ name: '', discord_name: '', zone: 'UTC' });
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
