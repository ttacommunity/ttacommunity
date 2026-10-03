/** The admin page: who is signed in, rating runs and the admin list for admins, and every tournament's settings and notes. */
import { useEffect, useState, type FormEvent } from 'react';
import { adminRequest, type AdminMe, type AdminTournament, type TournamentSettings } from '../lib/admin-api';
import { podiumNames, podiumRequest, type PodiumEntry } from '../lib/podium';
import AdminList from './AdminList';
import RunsPanel from './RunsPanel';
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
        <button type="button" className="btn-antique" onClick={save} disabled={text.trim() === saved}>
          Save note
        </button>
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </div>
  );
}

const PLACES = ['1st', '2nd', '3rd'];

/** PodiumBox sets a season's podium by hand, over the recorded or derived one, or removes it. */
function PodiumBox({
  slug,
  season,
  saved,
  savedNote,
  onReload,
}: {
  slug: string;
  season: string;
  saved: PodiumEntry[];
  savedNote: string;
  onReload: () => Promise<void>;
}) {
  const [names, setNames] = useState(podiumNames(saved));
  const [note, setNote] = useState(savedNote);
  const [status, setStatus] = useState('');
  const id = `podium-${slug}-${season}`.replace(/[^A-Za-z0-9-]/g, '_');
  const save = async () => {
    const body = podiumRequest(season, names, note);
    if (body === null) return;
    setStatus('Saving…');
    const r = await adminRequest(`/api/tournaments/${slug}/podium`, { method: 'PUT', json: body });
    setStatus(r.ok ? 'Saved. Titles follow at the next run.' : r.message);
    if (r.ok) await onReload();
  };
  const clear = async () => {
    setStatus('Removing…');
    const r = await adminRequest(`/api/tournaments/${slug}/podium?season=${encodeURIComponent(season)}`, { method: 'DELETE' });
    setStatus(r.ok ? 'Removed. The recorded or derived podium shows again.' : r.message);
    if (r.ok) await onReload();
  };
  return (
    <fieldset className="podium-box">
      <legend>Season {season} podium {saved.length > 0 ? '(set by hand)' : '(from the results)'}</legend>
      <div className="podium-places">
        {PLACES.map((label, i) => (
          <label key={label}>
            {label}
            <input
              id={`${id}-${i + 1}`}
              value={names[i]}
              maxLength={64}
              onChange={(e) => setNames(names.map((n, j) => (j === i ? e.target.value : n)))}
            />
          </label>
        ))}
        <label className="podium-note">
          Why, or where it comes from
          <input id={`${id}-note`} value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} />
        </label>
      </div>
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={podiumRequest(season, names, note) === null}>
          Set podium
        </button>
        {saved.length > 0 && (
          <button type="button" className="btn-antique" onClick={() => void clear()}>
            Use the results again
          </button>
        )}
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </fieldset>
  );
}

function TournamentRow({
  t,
  onChange,
  onReload,
}: {
  t: AdminTournament;
  onChange: (t: AdminTournament) => void;
  onReload: () => Promise<void>;
}) {
  const [error, setError] = useState('');
  const [open, setOpen] = useState(false);
  const [newSeason, setNewSeason] = useState('');
  const addSeason = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    const r = await adminRequest(`/api/tournaments/${t.slug}/seasons`, { method: 'POST', json: { label: newSeason.trim() } });
    if (r.ok) {
      setNewSeason('');
      await onReload();
    } else setError(r.message);
  };
  const removeSeason = async (label: string) => {
    if (!window.confirm(`Remove ${t.name} season ${label} and its notes?`)) return;
    setError('');
    const r = await adminRequest(`/api/tournaments/${t.slug}/seasons?label=${encodeURIComponent(label)}`, { method: 'DELETE' });
    if (r.ok) await onReload();
    else setError(r.message);
  };
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
          <button type="button" className="btn-antique btn-notes" aria-expanded={open} onClick={() => setOpen(!open)}>
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
            <div className="notes-panel">
            <form className="season-add" onSubmit={(e) => void addSeason(e)}>
              <label>
                Add a season with no games yet (its label as the site shows it, without "Season")
                <input value={newSeason} maxLength={64} placeholder="e.g. 1 or 2023" onChange={(e) => setNewSeason(e.target.value)} />
              </label>
              <button type="submit" className="btn-antique" disabled={newSeason.trim() === ''}>
                Add season
              </button>
            </form>
            {t.seasons.length === 0 && <p className="muted">No seasons yet.</p>}
            {t.seasons.map((se) => (
              <div key={se.label} className="season-notes">
                {!se.has_games && (
                  <div className="season-empty">
                    <span className="muted">Season {se.label}: no games recorded.</span>
                    <button type="button" className="btn-antique" onClick={() => void removeSeason(se.label)}>
                      Remove season
                    </button>
                  </div>
                )}
                <NoteBox slug={t.slug} season={se.label} stage="" initial={se.note} />
                <PodiumBox slug={t.slug} season={se.label} saved={se.podium} savedNote={se.podium_note} onReload={onReload} />
                {se.stages.map((st) => (
                  <NoteBox key={st.label} slug={t.slug} season={se.label} stage={st.label} initial={st.note} />
                ))}
              </div>
            ))}
            </div>
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

  const loadTournaments = async () => {
    const r = await adminRequest<AdminTournament[]>('/api/tournaments', {});
    setTournaments(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  useEffect(() => {
    void adminRequest<AdminMe>('/api/me', {}).then((r) =>
      setMe(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
    void loadTournaments();
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
      {me.data.role === 'admin' && <RunsPanel />}
      {me.data.role === 'admin' && <AdminList myName={me.data.name} />}
      <section className="rating-card admin-card" aria-labelledby="tournaments-heading">
        <h2 id="tournaments-heading" className="rating-card-heading">
          Tournaments
        </h2>
        <p className="muted">Changes save at once and are recorded with your name. Rating changes apply from the next engine run.</p>
        {tournaments.kind === 'loading' && <p className="muted">Loading tournaments…</p>}
        {tournaments.kind === 'error' && <p className="row-error" role="alert">{tournaments.message}</p>}
        {tournaments.kind === 'ready' && (
          <div className="table-responsive">
            <table className="leaderboard-table admin-table">
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
                  <TournamentRow key={t.slug} t={t} onChange={replace} onReload={loadTournaments} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </>
  );
}
