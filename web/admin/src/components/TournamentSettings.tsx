/** A tournament's settings page: its on/off settings, its format and regulations, and each season's notes and podium. */
import { useEffect, useState, type FormEvent } from 'react';
import { adminRequest, type AdminMe, type AdminTournament } from '../lib/admin-api';
import { podiumNames, podiumRequest, type PodiumEntry } from '../lib/podium';
import { SETTINGS, slugParam, type Setting } from '../lib/tournament-page';
import './AdminApp.css';

type Load<T> = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: T };

/** NoteBox edits the note shown beside a season (stage '') or one of its stages. */
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
      <label htmlFor={id}>{stage === '' ? 'Season note' : `${stage} note`}</label>
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

/** SettingsCard switches a tournament's settings, saving each at once. */
function SettingsCard({ t, onChange }: { t: AdminTournament; onChange: (t: AdminTournament) => void }) {
  const [error, setError] = useState('');
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
    <section className="rating-card admin-card" aria-labelledby="settings-heading">
      <h2 id="settings-heading" className="rating-card-heading">
        Settings
      </h2>
      <p className="muted">Changes save at once and are recorded with your name. Rating changes apply from the next engine run.</p>
      <div className="settings-list">
        {SETTINGS.map((s) => (
          <div key={s.key} className="setting">
            <input
              id={`setting-${s.key}`}
              type="checkbox"
              checked={t[s.key]}
              onChange={(e) => void set(s.key, e.target.checked)}
            />
            <label htmlFor={`setting-${s.key}`}>
              <strong>{s.label}</strong>
              <span className="muted"> · {s.hint}</span>
            </label>
          </div>
        ))}
      </div>
      {error !== '' && (
        <p className="row-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}

/** CardSettings edits how the tournament presents itself on its card and its page's header. */
function CardSettings({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [card, setCard] = useState(t.card);
  const [status, setStatus] = useState('');
  const changed = card.icon !== t.card.icon || card.tagline !== t.card.tagline || card.summary !== t.card.summary;
  const save = async () => {
    setStatus('Saving…');
    const r = await adminRequest(`/api/tournaments/${t.slug}/card`, { method: 'PUT', json: card });
    setStatus(r.ok ? 'Saved. The cards and the tournament page show it at once.' : r.message);
    if (r.ok) await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="card-heading">
      <h2 id="card-heading" className="rating-card-heading">
        Card
      </h2>
      <p className="muted">
        How the tournament shows on the Hall of Fame and Every Tournament, and at the top of its own page. Leave a box empty
        to leave it out; with no icon it shows 📜.
      </p>
      <div className="card-fields">
        <label>
          Icon
          <input id="card-icon" value={card.icon} maxLength={8} placeholder="📜" onChange={(e) => setCard({ ...card, icon: e.target.value })} />
        </label>
        <label className="card-tagline">
          Short description
          <input
            id="card-tagline"
            value={card.tagline}
            maxLength={60}
            placeholder="e.g. Annual World Championship"
            onChange={(e) => setCard({ ...card, tagline: e.target.value })}
          />
        </label>
        <label className="card-summary">
          Information box
          <textarea
            id="card-summary"
            value={card.summary}
            maxLength={1000}
            rows={4}
            onChange={(e) => setCard({ ...card, summary: e.target.value })}
          />
        </label>
      </div>
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={!changed}>
          Save card
        </button>
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </section>
  );
}

/** RegulationsCard edits the format and regulations the site shows, starting from CGE's text when none is written. */
function RegulationsCard({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [text, setText] = useState(t.regulations === '' ? t.cge_regulations : t.regulations);
  const [status, setStatus] = useState('');
  const save = async (value: string) => {
    setStatus('Saving…');
    const r = await adminRequest(`/api/tournaments/${t.slug}/regulations`, { method: 'PUT', json: { text: value } });
    if (!r.ok) {
      setStatus(r.message);
      return;
    }
    setStatus(value.trim() === '' ? "Removed. The site shows CGE's text again." : 'Saved. The site shows it at once.');
    if (value.trim() === '') setText(t.cge_regulations);
    await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="regulations-heading">
      <h2 id="regulations-heading" className="rating-card-heading">
        Format &amp; Regulations
      </h2>
      <p className="muted">
        {t.regulations !== ''
          ? 'Written here, and shown on the site instead of CGE\'s description.'
          : t.cge_regulations !== ''
            ? "The site shows the latest edition's description on CGE, below. Edit it and save to show your own."
            : 'Neither written here nor on CGE: the site shows no box. Write it below.'}
      </p>
      <textarea
        id="regulations-text"
        className="regulations-text"
        aria-labelledby="regulations-heading"
        value={text}
        maxLength={10000}
        rows={12}
        onChange={(e) => setText(e.target.value)}
      />
      <div className="note-actions">
        <button
          type="button"
          className="btn-antique"
          onClick={() => void save(text)}
          disabled={text.trim() === '' || text.trim() === (t.regulations === '' ? t.cge_regulations : t.regulations).trim()}
        >
          Save
        </button>
        {t.regulations !== '' && (
          <button type="button" className="btn-antique" onClick={() => void save('')}>
            {t.cge_regulations !== '' ? "Use CGE's text again" : 'Remove'}
          </button>
        )}
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </section>
  );
}

/** SeasonsCard lists a tournament's seasons with their notes and podiums, and adds or removes a season without games. */
function SeasonsCard({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [error, setError] = useState('');
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
  return (
    <section className="rating-card admin-card" aria-labelledby="seasons-heading">
      <h2 id="seasons-heading" className="rating-card-heading">
        Seasons
      </h2>
      <form className="season-add" onSubmit={(e) => void addSeason(e)}>
        <label>
          Add a season with no games yet (its label as the site shows it, without "Season")
          <input value={newSeason} maxLength={64} placeholder="e.g. 1 or 2023" onChange={(e) => setNewSeason(e.target.value)} />
        </label>
        <button type="submit" className="btn-antique" disabled={newSeason.trim() === ''}>
          Add season
        </button>
      </form>
      {error !== '' && (
        <p className="row-error" role="alert">
          {error}
        </p>
      )}
      {t.seasons.length === 0 && <p className="muted">No seasons yet.</p>}
      {t.seasons.map((se) => (
        <div key={se.label} className="season-notes">
          <h3>Season {se.label}</h3>
          {!se.has_games && (
            <div className="season-empty">
              <span className="muted">No games recorded.</span>
              <button type="button" className="btn-antique" onClick={() => void removeSeason(se.label)}>
                Remove season
              </button>
            </div>
          )}
          <NoteBox slug={t.slug} season={se.label} stage="" initial={se.note} />
          {se.stages.map((st) => (
            <NoteBox key={st.label} slug={t.slug} season={se.label} stage={st.label} initial={st.note} />
          ))}
          <PodiumBox slug={t.slug} season={se.label} saved={se.podium} savedNote={se.podium_note} onReload={onReload} />
        </div>
      ))}
    </section>
  );
}

/** TournamentSettings loads who is signed in and the tournament the address names, then its cards. */
export default function TournamentSettings() {
  const [me, setMe] = useState<Load<AdminMe>>({ kind: 'loading' });
  const [t, setT] = useState<Load<AdminTournament>>({ kind: 'loading' });
  const load = async () => {
    const slug = slugParam(new URLSearchParams(window.location.search));
    if (slug === null) {
      setT({ kind: 'error', message: 'This address names no tournament. Go back to the list.' });
      return;
    }
    const r = await adminRequest<AdminTournament[]>('/api/tournaments', {});
    if (!r.ok) {
      setT({ kind: 'error', message: r.message });
      return;
    }
    const found = r.data.find((x) => x.slug === slug);
    setT(found === undefined ? { kind: 'error', message: `No tournament is called ${slug}.` } : { kind: 'ready', data: found });
  };
  useEffect(() => {
    void adminRequest<AdminMe>('/api/me', {}).then((r) =>
      setMe(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
    void load();
  }, []);

  if (me.kind === 'error') return <p className="row-error" role="alert">{me.message}</p>;
  return (
    <>
      <p className="signed-in">
        <a href="/">← All tournaments</a>
        {me.kind === 'ready' && (
          <span className="muted">
            {' '}
            · Signed in as <strong>{me.data.name}</strong> ({me.data.role})
          </span>
        )}
      </p>
      {t.kind === 'loading' && <p className="muted">Loading…</p>}
      {t.kind === 'error' && <p className="row-error" role="alert">{t.message}</p>}
      {t.kind === 'ready' && (
        <>
          <h2 className="tournament-title">{t.data.name}</h2>
          <SettingsCard t={t.data} onChange={(next) => setT({ kind: 'ready', data: next })} />
          <CardSettings t={t.data} onReload={load} />
          <RegulationsCard t={t.data} onReload={load} />
          <SeasonsCard t={t.data} onReload={load} />
        </>
      )}
    </>
  );
}
