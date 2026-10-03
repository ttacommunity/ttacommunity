/** The admin page: who is signed in, and every tournament's settings and notes. */
import { useEffect, useState } from 'react';
import { adminRequest, type AdminMe, type AdminTournament, type TournamentSettings } from '../lib/admin-api';
import './AdminApp.css';

type Load<T> = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };
type Setting = keyof Required<TournamentSettings>;

const SETTINGS: { key: Setting; label: string; hint: string }[] = [
  { key: 'visible', label: 'Shown', hint: 'Shown on the site even when not rated' },
  { key: 'rated', label: 'Rated', hint: 'In the community ratings, from the next engine run' },
  { key: 'community', label: 'Community leaderboard', hint: 'Counted in the overall community leaderboard' },
];

function NoteBox({ slug, season, stage, initial }: { slug: string; season: string; stage: string; initial: string }) {
  const [text, setText] = useState(initial);
  const [saved, setSaved] = useState(initial);
  const [status, setStatus] = useState('');
  const id = `note-${slug}-${season}-${stage}`.replace(/[^A-Za-z0-9-]/g, '_');
  const save = async () => {
    setStatus('Saving…');
    const r = await adminRequest(`/api/tournaments/${slug}/notes`, { method: 'PUT', json: { season, stage, note: text.trim() } });
    if (r.ok) {
      setSaved(text.trim());
      setStatus(text.trim() === '' ? 'Removed' : 'Saved');
    } else {
      setStatus(r.message);
    }
  };
  return (
    <div className="note-box">
      <label htmlFor={id}>{stage === '' ? `Season ${season}` : `${season} · ${stage}`}</label>
      <textarea id={id} value={text} maxLength={2000} rows={2} onChange={(e) => setText(e.target.value)} />
      <div className="note-actions">
        <button type="button" onClick={save} disabled={text.trim() === saved}>
          Save note
        </button>
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </div>
  );
}

function TournamentRow({ t, onChange }: { t: AdminTournament; onChange: (t: AdminTournament) => void }) {
  const [error, setError] = useState('');
  const [open, setOpen] = useState(false);
  const set = async (key: Setting, value: boolean) => {
    setError('');
    onChange({ ...t, [key]: value });
    const r = await adminRequest(`/api/tournaments/${t.slug}`, { method: 'PATCH', json: { [key]: value } });
    if (!r.ok) {
      onChange({ ...t, [key]: !value });
      setError(r.message);
    }
  };
  return (
    <>
      <tr>
        <th scope="row">{t.name}</th>
        {SETTINGS.map((s) => (
          <td key={s.key}>
            <input
              type="checkbox"
              aria-label={`${s.label}: ${t.name}`}
              title={s.hint}
              checked={t[s.key]}
              onChange={(e) => void set(s.key, e.target.checked)}
            />
          </td>
        ))}
        <td>
          <button type="button" className="btn-notes" aria-expanded={open} onClick={() => setOpen(!open)}>
            Notes {open ? '▾' : '▸'}
          </button>
        </td>
      </tr>
      {error !== '' && (
        <tr>
          <td colSpan={5} className="row-error">
            <span role="alert">{error}</span>
          </td>
        </tr>
      )}
      {open && (
        <tr>
          <td colSpan={5} className="notes-cell">
            {t.seasons.length === 0 && <p className="muted">No seasons yet.</p>}
            {t.seasons.map((se) => (
              <div key={se.label} className="season-notes">
                <NoteBox slug={t.slug} season={se.label} stage="" initial={se.note} />
                {se.stages.map((st) => (
                  <NoteBox key={st.label} slug={t.slug} season={se.label} stage={st.label} initial={st.note} />
                ))}
              </div>
            ))}
          </td>
        </tr>
      )}
    </>
  );
}

/** AdminApp loads who is signed in, then the tournaments they can edit. */
export default function AdminApp() {
  const [me, setMe] = useState<Load<AdminMe>>({ kind: 'loading' });
  const [tournaments, setTournaments] = useState<Load<AdminTournament[]>>({ kind: 'loading' });

  useEffect(() => {
    void adminRequest<AdminMe>('/api/me', {}).then((r) =>
      setMe(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
    void adminRequest<AdminTournament[]>('/api/tournaments', {}).then((r) =>
      setTournaments(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
  }, []);

  if (me.kind === 'loading') return <p className="muted">Loading…</p>;
  if (me.kind === 'error') return <p className="row-error" role="alert">{me.message}</p>;

  const replace = (next: AdminTournament) =>
    setTournaments((cur) =>
      cur.kind === 'ready' ? { kind: 'ready', data: cur.data.map((t) => (t.slug === next.slug ? next : t)) } : cur,
    );

  return (
    <>
      <p className="signed-in">
        Signed in as <strong>{me.data.name}</strong> ({me.data.role})
      </p>
      <section aria-labelledby="tournaments-heading">
        <h2 id="tournaments-heading">Tournaments</h2>
        <p className="muted">Changes save at once and are recorded with your name. Rating changes apply from the next engine run.</p>
        {tournaments.kind === 'loading' && <p className="muted">Loading tournaments…</p>}
        {tournaments.kind === 'error' && <p className="row-error" role="alert">{tournaments.message}</p>}
        {tournaments.kind === 'ready' && (
          <div className="table-wrap">
            <table className="admin-table">
              <caption className="visually-hidden">Each tournament's settings</caption>
              <thead>
                <tr>
                  <th scope="col">Tournament</th>
                  {SETTINGS.map((s) => (
                    <th scope="col" key={s.key} title={s.hint}>
                      {s.label}
                    </th>
                  ))}
                  <th scope="col">Notes</th>
                </tr>
              </thead>
              <tbody>
                {tournaments.data.map((t) => (
                  <TournamentRow key={t.slug} t={t} onChange={replace} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </>
  );
}
