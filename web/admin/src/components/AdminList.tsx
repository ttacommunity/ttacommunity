/** The admin list, for admins: who is in admin and their role, with adding, changing and removing. */
import { isDiscordId } from '../lib/admin-edits';
import { Fragment, useEffect, useState, type FormEvent } from 'react';
import * as req from '../lib/admin-requests';
import { adminRequest, type AdminPerson } from '../lib/admin-api';
import { loadFailure, outcomeOf, refused, shownAt, type Outcome } from '../lib/outcome';
import Notice, { NoticeRow } from './Notice';

type Role = AdminPerson['role'];
const ROLES: { value: Role; label: string }[] = [
  { value: 'admin', label: 'Admin' },
  { value: 'organiser', label: 'Organiser' },
];

/** AdminList shows everyone in admin; myName is the signed-in admin. Admin always keeps one admin (the API refuses otherwise). */
export default function AdminList({ myName }: { myName: string }) {
  const [people, setPeople] = useState<AdminPerson[] | null>(null);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const close = () => setOutcome(null);
  const [form, setForm] = useState({ id: '', name: '', role: 'organiser' as Role });

  const load = async () => {
    const r = await adminRequest<AdminPerson[]>('/api/admins', {});
    if (r.ok) setPeople(r.data);
    else setOutcome(loadFailure(r.message, 'admins'));
  };
  useEffect(() => {
    void load();
  }, []);

  const save = async (id: string, name: string, role: Role, at: string) => {
    setOutcome(null);
    const r = await req.send(req.setAdmin(id, name, role));
    setOutcome(outcomeOf(r, `Saved ${name} as ${role}.`, at));
    await load();
    return r.ok;
  };
  const remove = async (p: AdminPerson) => {
    if (!window.confirm(`Take ${p.name} out of admin?`)) return;
    const r = await req.send(req.removeAdmin(p.discord_id));
    setOutcome(outcomeOf(r, `Removed ${p.name}. Their sign-in lasts until it ends or is revoked in Cloudflare Access.`, `admin ${p.discord_id}`, 'admins'));
    await load();
  };
  const add = async (e: FormEvent) => {
    e.preventDefault();
    if (!isDiscordId(form.id)) {
      setOutcome(refused('A Discord id is the number in Discord under Copy User ID (Developer Mode).', 'add admin'));
      return;
    }
    if (await save(form.id.trim(), form.name.trim(), form.role, 'add admin')) setForm({ id: '', name: '', role: 'organiser' });
  };

  return (
    <section className="rating-card admin-card" aria-labelledby="admins-heading">
      <h2 id="admins-heading" className="rating-card-heading">
        Admins
      </h2>
      <p className="muted">
        Admins can do everything here; organisers can edit tournaments. Someone also needs to be allowed in the Cloudflare
        Access policy to sign in.
      </p>
      <Notice outcome={shownAt(outcome, 'admins')} onClose={close} />
      {people !== null && (
        <div className="table-responsive">
          <table className="leaderboard-table admin-table">
            <caption className="visually-hidden">Everyone in admin</caption>
            <thead>
              <tr>
                <th scope="col">Name</th>
                <th scope="col">Discord id</th>
                <th scope="col">Role</th>
                <th scope="col">Remove</th>
              </tr>
            </thead>
            <tbody>
              {people.map((p) => {
                const me = p.name === myName;
                return (
                  <Fragment key={p.discord_id}>
                    <tr>
                      <th scope="row">
                        {p.name}
                        {me && <span className="muted"> (you)</span>}
                      </th>
                      <td className="admin-id">{p.discord_id}</td>
                      <td>
                        <select className="input-antique"
                          aria-label={`Role: ${p.name}`}
                          value={p.role}
                          onChange={(e) => void save(p.discord_id, p.name, e.target.value as Role, `admin ${p.discord_id}`)}
                        >
                          {ROLES.map((r) => (
                            <option key={r.value} value={r.value}>
                              {r.label}
                            </option>
                          ))}
                        </select>
                      </td>
                      <td>
                        <button type="button" className="btn-antique" onClick={() => void remove(p)}>
                          {me ? 'Remove me' : 'Remove'}
                        </button>
                      </td>
                    </tr>
                    <NoticeRow outcome={shownAt(outcome, `admin ${p.discord_id}`)} columns={4} onClose={close} />
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      <form className="admin-add" onSubmit={(e) => void add(e)}>
        <h3>Add someone</h3>
        <label>
          Discord id
          <input className="input-antique" inputMode="numeric" value={form.id} onChange={(e) => setForm({ ...form, id: e.target.value })} required />
        </label>
        <label>
          Name
          <input className="input-antique" value={form.name} maxLength={64} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        </label>
        <label>
          Role
          <select className="input-antique" value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value as Role })}>
            {ROLES.map((r) => (
              <option key={r.value} value={r.value}>
                {r.label}
              </option>
            ))}
          </select>
        </label>
        <button type="submit" className="btn-antique">
          Add
        </button>
      </form>
      <Notice outcome={shownAt(outcome, 'add admin')} onClose={close} />
    </section>
  );
}
