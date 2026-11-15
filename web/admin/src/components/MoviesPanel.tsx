/** Admin Movies panel: manage YouTube creator channels, trigger manual RSS syncs, and hide/show individual videos. */
import { useEffect, useState, type FormEvent } from 'react';
import { useDialog } from '@ttac/shared/Dialog';
import type { Load } from '@ttac/shared/load';
import { adminRequest } from '../lib/admin-api';
import { buildNewChannelBody, type AdminMoviesState, type AdminVideo, type AdminVideoChannel, type MoviesSyncResult } from '../lib/admin-movies';
import { outcomeOf, refused, shownAt, succeeded, type Outcome } from '../lib/outcome';
import Notice, { LoadNotice } from './Notice';
import AdminCard from './AdminCard';
import AdminTable from './AdminTable';

export default function MoviesPanel() {
  const [state, setState] = useState<Load<AdminMoviesState>>({ kind: 'loading' });
  const [channel, setChannel] = useState('');
  const [name, setName] = useState('');
  const [player, setPlayer] = useState('');
  const [includeAll, setIncludeAll] = useState(false);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [busy, setBusy] = useState(false);
  const dialog = useDialog();

  const reload = async () => {
    const r = await adminRequest<AdminMoviesState>('/api/movies/channels', {});
    setState(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };

  useEffect(() => {
    void reload();
  }, []);

  const addChannel = async (e: FormEvent) => {
    e.preventDefault();
    setOutcome(null);
    const parsed = buildNewChannelBody(channel, name, player, includeAll);
    if (!parsed.ok) {
      setOutcome(refused(parsed.message, 'movies-add'));
      return;
    }
    setBusy(true);
    const r = await adminRequest<AdminVideoChannel>('/api/movies/channels', {
      method: 'POST',
      json: parsed.body,
    });
    setBusy(false);
    if (!r.ok) {
      setOutcome(outcomeOf(r, '', 'movies-add'));
      return;
    }
    setChannel('');
    setName('');
    setPlayer('');
    setIncludeAll(false);
    setOutcome(succeeded(`Added ${r.data.name} (${r.data.channel_id}).`, 'movies-add'));
    await reload();
  };

  const syncNow = async () => {
    setBusy(true);
    setOutcome(null);
    const r = await adminRequest<MoviesSyncResult>('/api/movies/sync', { method: 'POST' });
    setBusy(false);
    if (!r.ok) {
      setOutcome(outcomeOf(r, '', 'movies-sync'));
      return;
    }
    setOutcome(succeeded(`Checked ${r.data.checked} channel(s), added ${r.data.added} new video(s).`, 'movies-sync'));
    await reload();
  };

  const toggleIncludeAll = async (c: AdminVideoChannel) => {
    setOutcome(null);
    const r = await adminRequest<unknown>(`/api/movies/channels/${encodeURIComponent(c.channel_id)}`, {
      method: 'PATCH',
      json: { include_all: !c.include_all },
    });
    if (!r.ok) {
      setOutcome(outcomeOf(r, '', `channel-${c.channel_id}`));
      return;
    }
    setOutcome(succeeded(`${c.name} now ${!c.include_all ? 'includes all uploads' : 'uses the TTA filter'}.`, `channel-${c.channel_id}`));
    await reload();
  };

  const deleteChannel = async (c: AdminVideoChannel, opener: HTMLElement) => {
    const sure = await dialog.confirm(
      {
        heading: `Remove ${c.name}?`,
        text: 'This removes the creator and their fetched videos from the Movies feed.',
        action: 'Remove creator',
        final: true,
      },
      opener,
    );
    if (!sure) return;
    setOutcome(null);
    const r = await adminRequest<unknown>(`/api/movies/channels/${encodeURIComponent(c.channel_id)}`, {
      method: 'DELETE',
    });
    if (!r.ok) {
      setOutcome(outcomeOf(r, '', `channel-${c.channel_id}`));
      return;
    }
    setOutcome(succeeded(`Removed ${c.name}.`, 'movies-list'));
    await reload();
  };

  const toggleVideoHidden = async (v: AdminVideo) => {
    setOutcome(null);
    const r = await adminRequest<unknown>(`/api/movies/videos/${encodeURIComponent(v.video_id)}`, {
      method: 'PATCH',
      json: { hidden: !v.hidden },
    });
    if (!r.ok) {
      setOutcome(outcomeOf(r, '', `video-${v.video_id}`));
      return;
    }
    setOutcome(succeeded(`${!v.hidden ? 'Hid' : 'Shown'} “${v.title}”.`, `video-${v.video_id}`));
    await reload();
  };

  return (
    <AdminCard
      id="movies-admin-heading"
      title="Movies"
      actions={
        <button type="button" className="btn-antique" disabled={busy} onClick={() => void syncNow()}>
          {busy ? 'Checking…' : 'Check for new videos'}
        </button>
      }
    >
      {dialog.element}
      <p className="muted">YouTube creators whose Through the Ages videos appear on the Movies page. Checked automatically every 30 minutes.</p>

      <Notice outcome={shownAt(outcome, outcome?.at ?? '')} onClose={() => setOutcome(null)} />

      <form onSubmit={addChannel} className="movies-add-card">
        <h3>Add creator</h3>
        <div className="admin-add">
          <label className="movies-field-wide">
            YouTube channel URL, @handle, or UC... ID
            <input
              className="input-antique"
              value={channel}
              onChange={(e) => setChannel(e.target.value)}
              placeholder="https://www.youtube.com/@handle or @handle"
            />
          </label>
          <label>
            Creator display name
            <input
              className="input-antique"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Creator name"
            />
          </label>
          <label>
            Linked TTA player (optional)
            <input
              className="input-antique"
              value={player}
              onChange={(e) => setPlayer(e.target.value)}
              placeholder="Exact player name"
            />
          </label>
          <label className="setting movies-include-all">
            <input type="checkbox" checked={includeAll} onChange={(e) => setIncludeAll(e.target.checked)} />
            <span>Include all uploads (skip TTA filter)</span>
          </label>
          <button type="submit" className="btn-antique" disabled={busy}>
            Add creator
          </button>
        </div>
      </form>

      {state.kind === 'loading' && <p className="muted">Loading creators…</p>}
      {state.kind === 'error' && <LoadNotice message={state.message} />}
      {state.kind === 'ready' && (
        <div className="movies-sections">
          <div>
            <h3 className="movies-subheading">Creators ({state.data.channels.length})</h3>
            {state.data.channels.length === 0 ? (
              <p className="muted">No creators added yet. Add a YouTube channel above to start indexing videos.</p>
            ) : (
              <AdminTable caption="The YouTube creators indexed, each with its linked player and filter" className="movies-table">
                <thead>
                  <tr>
                    <th>Creator</th>
                    <th>Channel ID</th>
                    <th>Linked player</th>
                    <th>Filter</th>
                    <th>Videos</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {state.data.channels.map((c) => (
                    <tr key={c.channel_id}>
                      <td className="movies-creator-name">{c.name}</td>
                      <td>
                        <a
                          className="movies-channel-id"
                          href={`https://www.youtube.com/channel/${encodeURIComponent(c.channel_id)}`}
                          target="_blank"
                          rel="noopener noreferrer"
                        >
                          <code>{c.channel_id}</code>
                        </a>
                      </td>
                      <td>{c.player ?? <span className="muted">—</span>}</td>
                      <td>
                        <button
                          type="button"
                          className="btn-settings"
                          onClick={() => void toggleIncludeAll(c)}
                          title="Toggle Through the Ages keyword filter"
                        >
                          {c.include_all ? 'All uploads' : 'TTA-filtered'}
                        </button>
                      </td>
                      <td>{c.video_count}</td>
                      <td>
                        <button
                          type="button"
                          className="btn-settings"
                          onClick={(e) => void deleteChannel(c, e.currentTarget)}
                        >
                          Remove
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </AdminTable>
            )}
          </div>

          <div>
            <h3 className="movies-subheading">Recent videos ({state.data.videos.length})</h3>
            {state.data.videos.length === 0 ? (
              <p className="muted">No videos fetched yet. Click “Check for new videos” after adding a creator.</p>
            ) : (
              <AdminTable caption="The videos found, with each one's creator, date and whether the Movies page shows it" className="movies-table">
                <thead>
                  <tr>
                    <th>Video</th>
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
                        <div className="movies-video-cell">
                          <img
                            className="movies-thumb"
                            src={`https://i.ytimg.com/vi/${encodeURIComponent(v.video_id)}/mqdefault.jpg`}
                            alt=""
                            width={72}
                            height={40}
                            loading="lazy"
                          />
                          <a
                            href={`https://www.youtube.com/watch?v=${encodeURIComponent(v.video_id)}`}
                            target="_blank"
                            rel="noopener noreferrer"
                          >
                            {v.title}
                          </a>
                        </div>
                      </td>
                      <td>{v.creator_name}</td>
                      <td className="movies-date">{new Date(v.published_at).toLocaleDateString()}</td>
                      <td>
                        <span className={v.hidden ? 'movies-status movies-status-hidden' : 'movies-status movies-status-visible'}>
                          {v.hidden ? 'Hidden' : 'Visible'}
                        </span>
                      </td>
                      <td>
                        <button type="button" className="btn-settings" onClick={() => void toggleVideoHidden(v)}>
                          {v.hidden ? 'Show' : 'Hide'}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </AdminTable>
            )}
          </div>
        </div>
      )}
    </AdminCard>
  );
}
