/** The Members tab: the community's people, listed, added and edited by admins and organisers; banned, unbanned and removed by admins. */
import { useEffect, useState, type FormEvent } from 'react';
import { offsetLabel, zoneOptions } from '@ttac/shared/blitz';
import { adminRequest, type AdminMe, type AdminMember, type Load, type MemberAdded } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { isCountryCode, type MemberChange } from '../lib/members';

/** knownZones is every IANA zone the browser knows, or none in one that cannot say. */
function knownZones(): string[] {
  try {
    return Intl.supportedValuesOf('timeZone');
  } catch {
    return [];
  }
}

type Draft = { name: string; discord_name: string; email: string; country: string; zone: string; cge_user: string };

/** draftOf fills the edit form from a member. */
function draftOf(m: AdminMember): Draft {
  return { name: m.name, discord_name: m.discord_name, email: m.email, country: m.country ?? '', zone: m.zone, cge_user: m.cge_user === null ? '' : String(m.cge_user) };
}

export default function MembersPanel({ role }: { role: AdminMe['role'] }) {
  const [members, setMembers] = useState<Load<AdminMember[]>>({ kind: 'loading' });
  const [message, setMessage] = useState('');
  const [editing, setEditing] = useState<{ id: number } & Draft | null>(null);
  const [form, setForm] = useState<Draft>({ name: '', discord_name: '', email: '', country: '', zone: 'UTC', cge_user: '' });
  const [known] = useState(knownZones);
  const load = async () => {
    const r = await adminRequest<AdminMember[]>('/api/members', {});
    setMembers(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  useEffect(() => {
    void load();
  }, []);
  const run = async (r: req.Request, done: string) => {
    const res = await req.send(r);
    setMessage(res.ok ? done : res.message);
    await load();
    return res.ok;
  };
  const add = async (e: FormEvent) => {
    e.preventDefault();
    if (!isCountryCode(form.country)) {
      setMessage('A country is a two-letter code, such as DE.');
      return;
    }
    const r = await req.send<MemberAdded>(req.addMember({ name: form.name, discord_name: form.discord_name, email: form.email, country: form.country.trim(), zone: form.zone }));
    setMessage(r.ok ? `Added ${form.name.trim()}.` : r.message);
    if (r.ok) setForm({ ...form, name: '', discord_name: '', email: '', country: '' });
    await load();
  };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (editing === null) return;
    if (!isCountryCode(editing.country)) {
      setMessage('A country is a two-letter code, such as DE.');
      return;
    }
    const change: MemberChange = { name: editing.name, discord_name: editing.discord_name, email: editing.email, country: editing.country.trim(), zone: editing.zone };
    if (editing.cge_user.trim() !== '') change.cge_user = Number(editing.cge_user);
    if (await run(req.setMember(editing.id, change), 'Saved.')) setEditing(null);
  };
  const ban = (m: AdminMember) => {
    const reason = window.prompt(`Ban ${m.name} from taking part. The reason:`);
    if (reason === null || reason.trim() === '') return;
    void run(req.banMember(m.id, reason), `${m.name} is banned.`);
  };
  const remove = (m: AdminMember) => {
    if (window.confirm(`Remove ${m.name}? Only for a mistaken entry; a member in an edition cannot be removed.`)) void run(req.removeMember(m.id), `Removed ${m.name}.`);
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="members-heading">
      <h2 id="members-heading" className="rating-card-heading">
        Members
      </h2>
      <p className="muted">
        Everyone who takes part in anything we run, by their TTA username as in the app. A name the ratings or a CGE
        account know is linked to them, and the link fills itself in as games are loaded. A banned member cannot be
        entered in an edition; their games and ratings stay. Organisers reach players by Discord name or email.
      </p>
      {message !== '' && (
        <p className="admin-message" role="status">
          {message}
        </p>
      )}
      {members.kind === 'loading' && <p className="muted">Loading…</p>}
      {members.kind === 'error' && (
        <p className="row-error" role="alert">
          {members.message}
        </p>
      )}
      {members.kind === 'ready' && members.data.length === 0 && <p className="muted">No members yet. Add them below.</p>}
      {members.kind === 'ready' && members.data.length > 0 && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Every member</caption>
            <thead>
              <tr>
                <th scope="col">Username</th>
                <th scope="col">CGE id</th>
                <th scope="col">Discord</th>
                <th scope="col">Email</th>
                <th scope="col">Country</th>
                <th scope="col">Time zone</th>
                <th scope="col">In</th>
                <th scope="col">
                  <span className="visually-hidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {members.data.map((m) =>
                editing?.id === m.id ? (
                  <tr key={m.id}>
                    <th scope="row">
                      <input className="input-antique" aria-label={`Username of ${m.name}`} value={editing.name} maxLength={100} onChange={(e) => setEditing({ ...editing, name: e.target.value })} />
                    </th>
                    <td>
                      <input className="input-antique" aria-label={`CGE id of ${m.name}`} inputMode="numeric" value={editing.cge_user} onChange={(e) => setEditing({ ...editing, cge_user: e.target.value })} />
                    </td>
                    <td>
                      <input className="input-antique" aria-label={`Discord name of ${m.name}`} value={editing.discord_name} maxLength={100} onChange={(e) => setEditing({ ...editing, discord_name: e.target.value })} />
                    </td>
                    <td>
                      <input className="input-antique" aria-label={`Email of ${m.name}`} type="email" value={editing.email} maxLength={254} onChange={(e) => setEditing({ ...editing, email: e.target.value })} />
                    </td>
                    <td>
                      <input className="input-antique" aria-label={`Country of ${m.name}`} value={editing.country} maxLength={2} onChange={(e) => setEditing({ ...editing, country: e.target.value })} />
                    </td>
                    <td>
                      <select className="input-antique" aria-label={`Time zone of ${m.name}`} value={editing.zone} onChange={(e) => setEditing({ ...editing, zone: e.target.value })}>
                        {zoneOptions(known, editing.zone).map((z) => (
                          <option key={z} value={z}>
                            {z}
                          </option>
                        ))}
                      </select>
                    </td>
                    <td>{m.entries}</td>
                    <td className="run-buttons">
                      <button type="button" className="btn-antique" onClick={(e) => void save(e)}>
                        Save
                      </button>
                      <button type="button" className="btn-antique" onClick={() => setEditing(null)}>
                        Cancel
                      </button>
                    </td>
                  </tr>
                ) : (
                  <tr key={m.id} className={m.banned ? 'row-error' : ''}>
                    <th scope="row">
                      {m.name}
                      {m.player !== '' && m.player !== m.name && <span className="row-detail muted">{m.player} in the ratings</span>}
                      {m.player === '' && <span className="row-detail muted">not in the ratings</span>}
                      {m.banned && <span className="row-detail">banned: {m.ban_reason}</span>}
                    </th>
                    <td>{m.cge_user ?? ''}</td>
                    <td>{m.discord_name}</td>
                    <td>{m.email}</td>
                    <td>{m.country ?? ''}</td>
                    <td>
                      {m.zone} <span className="muted">{offsetLabel(m.utc_offset)}</span>
                    </td>
                    <td>{m.entries}</td>
                    <td className="run-buttons">
                      <button type="button" className="btn-antique" onClick={() => setEditing({ id: m.id, ...draftOf(m) })}>
                        Edit
                      </button>
                      {role === 'admin' && !m.banned && (
                        <button type="button" className="btn-antique" onClick={() => ban(m)}>
                          Ban
                        </button>
                      )}
                      {role === 'admin' && m.banned && (
                        <button type="button" className="btn-antique" onClick={() => void run(req.unbanMember(m.id), `${m.name} is no longer banned.`)}>
                          Unban
                        </button>
                      )}
                      {role === 'admin' && (
                        <button type="button" className="btn-antique" onClick={() => remove(m)}>
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
      <form className="admin-add" onSubmit={(e) => void add(e)}>
        <h3>Add a member</h3>
        <label>
          Username
          <input className="input-antique" value={form.name} maxLength={100} required onChange={(e) => setForm({ ...form, name: e.target.value })} />
        </label>
        <label>
          Discord name
          <input className="input-antique" value={form.discord_name} maxLength={100} onChange={(e) => setForm({ ...form, discord_name: e.target.value })} />
        </label>
        <label>
          Email
          <input className="input-antique" type="email" value={form.email} maxLength={254} onChange={(e) => setForm({ ...form, email: e.target.value })} />
        </label>
        <label>
          Country
          <input className="input-antique" value={form.country} maxLength={2} placeholder="DE" onChange={(e) => setForm({ ...form, country: e.target.value })} />
        </label>
        <label>
          Time zone
          <select className="input-antique" value={form.zone} onChange={(e) => setForm({ ...form, zone: e.target.value })}>
            {zoneOptions(known, form.zone).map((z) => (
              <option key={z} value={z}>
                {z}
              </option>
            ))}
          </select>
        </label>
        <button type="submit" className="btn-antique" disabled={form.name.trim() === ''}>
          Add
        </button>
      </form>
    </section>
  );
}
