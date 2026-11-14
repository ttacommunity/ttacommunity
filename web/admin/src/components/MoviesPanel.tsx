/** Admin Movies panel: manage YouTube creator channels, trigger manual RSS syncs, and hide/show individual videos. */
import { useEffect, useState, type FormEvent } from 'react';
import type { Load } from '@ttac/shared/load';
import { adminRequest } from '../lib/admin-api';
import { buildNewChannelBody, type AdminMoviesState, type AdminVideo, type AdminVideoChannel, type MoviesSyncResult } from '../lib/admin-movies';
import { LoadNotice } from './Notice';

export default function MoviesPanel() {
  const [state, setState] = useState<Load<AdminMoviesState>>({ kind: 'loading' });
  const [channel, setChannel] = useState('');
  const [name, setName] = useState('');
  const [player, setPlayer] = useState('');
  const [includeAll, setIncludeAll] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const reload = async () => {
    const r = await adminRequest<AdminMoviesState>('/api/movies/channels', {});
    setState(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };

  useEffect(() => {
    void reload();
  }, []);

  const addChannel = async (e: FormEvent) => {
    e.preventDefault();
    setNotice(null);
    const parsed = buildNewChannelBody(channel, name, player, includeAll);
    if (!parsed.ok) {
      setNotice(parsed.message);
      return;
    }
    setBusy(true);
    const r = await adminRequest<AdminVideoChannel>('/api/movies/channels', {
      method: 'POST',
      json: parsed.body,
    });
    setBusy(false);
    if (!r.ok) {
      setNotice(r.message);
      return;
    }
    setChannel('');
    setName('');
    setPlayer('');
    setIncludeAll(false);
    setNotice(`Added ${r.data.name}.`);
    await reload();
  };

  const syncNow = async () => {
    setBusy(true);
    setNotice(null);
    const r = await adminRequest<MoviesSyncResult>('/api/movies/sync', { method: 'POST' });
    setBusy(false);
    if (!r.ok) {
      setNotice(r.message);
      return;
    }
    setNotice(`Checked ${r.data.checked} channel(s), added ${r.data.added} new video(s).`);
    await reload();
  };

  const toggleIncludeAll = async (c: AdminVideoChannel) => {
    setNotice(null);
    const r = await adminRequest<unknown>(`/api/movies/channels/${encodeURIComponent(c.channel_id)}`, {
      method: 'PATCH',
      json: { include_all: !c.include_all },
    });
    if (!r.ok) {
      setNotice(r.message);
      return;
    }
    await reload();
  };

  const deleteChannel = async (c: AdminVideoChannel) => {
    setNotice(null);
    const r = await adminRequest<unknown>(`/api/movies/channels/${encodeURIComponent(c.channel_id)}`, {
      method: 'DELETE',
    });
    if (!r.ok) {
      setNotice(r.message);
      return;
    }
    setNotice(`Removed ${c.name}.`);
    await reload();
  };

  const toggleVideoHidden = async (v: AdminVideo) => {
    setNotice(null);
    const r = await adminRequest<unknown>(`/api/movies/videos/${encodeURIComponent(v.video_id)}`, {
      method: 'PATCH',
      json: { hidden: !v.hidden },
    });
    if (!r.ok) {
      setNotice(r.message);
      return;
    }
    await reload();
  };

  return (
    <section className="rating-card admin-card" aria-labelledby="movies-admin-heading">
      <div className="format-bar">
        <div>
          <h2 id="movies-admin-heading" className="rating-card-heading">
            Movies
          </h2>
          <p className="muted">
            YouTube creators whose Through the Ages videos appear on the Movies page. Checked automatically every 30 minutes.
          </p>
        </div>
        <div className="format-bar-actions">
          <button type="button" className="btn-antique" disabled={busy} onClick={syncNow}>
            {busy ? 'Checking…' : 'Check for new videos'}
          </button>
        </div>
      </div>

      {notice && <p className="muted">{notice}</p>}

      <form onSubmit={addChannel} className="admin-form">
        <label>
          YouTube channel URL, @handle, or UC... ID
          <input value={channel} onChange={(e) => setChannel(e.target.value)} placeholder="https://www.youtube.com/@handle" />
        </label>
        <label>
          Creator display name
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Creator name" />
        </label>
        <label>
          Linked TTA player (optional)
          <input value={player} onChange={(e) => setPlayer(e.target.value)} placeholder="Exact player name" />
        </label>
        <label>
          <input type="checkbox" checked={includeAll} onChange={(e) => setIncludeAll(e.target.checked)} /> Include all uploads (skip TTA filter)
        </label>
        <button type="submit" className="btn-antique" disabled={busy}>
          Add creator
        </button>
      </form>

      {state.kind === 'loading' && <p className="muted">Loading creators…</p>}
      {state.kind === 'error' && <LoadNotice message={state.message} />}
      {state.kind === 'ready' && (
        <>
          <h3>Creators ({state.data.channels.length})</h3>
          <table className="admin-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Channel ID</th>
                <th>Player</th>
                <th>Filter</th>
                <th>Videos</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {state.data.channels.map((c) => (
                <tr key={c.channel_id}>
                  <td>{c.name}</td>
                  <td><code>{c.channel_id}</code></td>
                  <td>{c.player ?? '—'}</td>
                  <td>
                    <button type="button" className="btn-antique" onClick={() => void toggleIncludeAll(c)}>
                      {c.include_all ? 'All uploads' : 'TTA-filtered'}
                    </button>
                  </td>
                  <td>{c.video_count}</td>
                  <td>
                    <button type="button" className="btn-antique" onClick={() => void deleteChannel(c)}>
                      Remove
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          <h3>Recent videos ({state.data.videos.length})</h3>
          <table className="admin-table">
            <thead>
              <tr>
                <th>Title</th>
                <th>Creator</th>
                <th>Published</th>
                <th>Status</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              {state.data.videos.map((v) => (
                <tr key={v.video_id}>
                  <td>
                    <a href={`https://www.youtube.com/watch?v=${encodeURIComponent(v.video_id)}`} target="_blank" rel="noopener noreferrer">
                      {v.title}
                    </a>
                  </td>
                  <td>{v.creator_name}</td>
                  <td>{new Date(v.published_at).toLocaleDateString()}</td>
                  <td>{v.hidden ? 'Hidden' : 'Visible'}</td>
                  <td>
                    <button type="button" className="btn-antique" onClick={() => void toggleVideoHidden(v)}>
                      {v.hidden ? 'Show' : 'Hide'}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </section>
  );
}
