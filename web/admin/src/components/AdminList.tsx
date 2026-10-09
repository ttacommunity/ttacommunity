/** The admin list, for admins: who is in admin and their role, with adding, changing and removing. */
import { useEffect, useState, type FormEvent } from 'react';
import { adminRequest, type AdminPerson } from '../lib/admin-api';

type Role = AdminPerson['role'];
const ROLES: { value: Role; label: string }[] = [
  { value: 'admin', label: 'Admin' },
  { value: 'organiser', label: 'Organiser' },
];

/** AdminList shows everyone in admin; myName is the signed-in admin. Admin always keeps one admin (the API refuses otherwise). */
export default function AdminList({ myName }: { myName: string }) {
  const [people, setPeople] = useState<AdminPerson[] | null>(null);
  const [message, setMessage] = useState('');
  const [form, setForm] = useState({ id: '', name: '', role: 'organiser' as Role });

  const load = async () => {
    const r = await adminRequest<AdminPerson[]>('/api/admins', {});
    if (r.ok) setPeople(r.data);
    else setMessage(r.message);
  };
  useEffect(() => {
    void load();
  }, []);

  const save = async (id: string, name: string, role: Role) => {
    setMessage('');
    const r = await adminRequest(`/api/admins/${id}`, { method: 'PUT', json: { name, role } });
    setMessage(r.ok ? `Saved ${name} as ${role}.` : r.message);
    await load();
    return r.ok;
  };
  const remove = async (p: AdminPerson) => {
    if (!window.confirm(`Take ${p.name} out of admin?`)) return;
    const r = await adminRequest(`/api/admins/${p.discord_id}`, { method: 'DELETE' });
    setMessage(r.ok ? `Removed ${p.name}. Their sign-in lasts until it ends or is revoked in Cloudflare Access.` : r.message);
    await load();
  };
  const add = async (e: FormEvent) => {
    e.preventDefault();
    if (!/^[0-9]{5,25}$/.test(form.id.trim())) {
      setMessage('A Discord id is the number in Discord under Copy User ID (Developer Mode).');
      return;
    }
    if (await save(form.id.trim(), form.name.trim(), form.role)) setForm({ id: '', name: '', role: 'organiser' });
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
      {message !== '' && (
        <p className="admin-message" role="status">
          {message}
        </p>
      )}
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
                  <tr key={p.discord_id}>
                    <th scope="row">
                      {p.name}
                      {me && <span className="muted"> (you)</span>}
                    </th>
                    <td className="admin-id">{p.discord_id}</td>
                    <td>
                      <select className="input-antique"
                        aria-label={`Role: ${p.name}`}
                        value={p.role}
                        onChange={(e) => void save(p.discord_id, p.name, e.target.value as Role)}
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
    </section>
  );
}
