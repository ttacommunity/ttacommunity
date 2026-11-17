/** Admin Movies panel: manage YouTube creator channels and see their subscriptions, and override a video's
 * verdict. */
import { useEffect, useState, type FormEvent } from 'react';
import { useDialog } from '@ttac/shared/Dialog';
import type { Load } from '@ttac/shared/load';
import { clockOrUtc, formatDate, formatInstant, todayIn, viewerZone } from '@ttac/shared/time';
import { adminRequest } from '../lib/admin-api';
import {
  buildNewChannelBody,
  channelStatus,
  DATE_PRESETS,
  moviesRunStarted,
  NO_VIDEO_FILTERS,
  REASONS,
  SHOWN_BY,
  STATUSES,
  UPLOADS,
  overrideChoice,
  overrideOf,
  videoPage,
  videosMatching,
  videoStatus,
  type AdminMoviesState,
  type AdminVideo,
  type AdminVideoChannel,
  type OverrideChoice,
  type VideoFilters,
} from '../lib/admin-movies';
import { outcomeOf, refused, shownAt, succeeded, type Outcome } from '../lib/outcome';
import { playerChoices } from '../lib/person-form';
import Notice, { LoadNotice } from './Notice';
import AdminCard from './AdminCard';
import AdminTable from './AdminTable';
import NewerOlder from '@ttac/shared/NewerOlder';
import MultiPick from './MultiPick';
import RowSelect from './RowSelect';
import PeoplePick from './PeoplePick';

const POLL_MS = 5000;

export default function MoviesPanel() {
  const [state, setState] = useState<Load<AdminMoviesState>>({ kind: 'loading' });
  const [channel, setChannel] = useState('');
  const [name, setName] = useState('');
  const [player, setPlayer] = useState('');
  const [includeAll, setIncludeAll] = useState(false);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [busy, setBusy] = useState(false);
  const [starting, setStarting] = useState(false);
  const [filters, setFilters] = useState<VideoFilters>(NO_VIDEO_FILTERS);
  const [page, setPage] = useState(1);
  const filter = (f: Partial<VideoFilters>) => {
    setFilters({ ...filters, ...f });
    setPage(1);
  };
  const dialog = useDialog();

  const reload = async () => {
    const r = await adminRequest<AdminMoviesState>('/api/movies/channels', {});
    setState(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };

  useEffect(() => {
    void reload();
  }, []);

  const running = state.kind === 'ready' && state.data.job_running;
  useEffect(() => {
    if (!running) return;
    const timer = window.setInterval(() => void reload(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [running]);

  /** startRun starts the Movies job for c, or every creator when null, and says so under key. */
  const startRun = async (c: AdminVideoChannel | null, key: string, before = '') => {
    setStarting(true);
    const r = await adminRequest<unknown>('/api/movies/run', { method: 'POST', json: c === null ? {} : { channel: c.channel_id } });
    setStarting(false);
    if (!r.ok) {
      setOutcome(refused(`${before}The Movies job did not start: ${r.message}`, key));
    } else {
      setOutcome(succeeded(before + moviesRunStarted(c === null ? null : c.name), key));
    }
    await reload();
  };

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
    await startRun(r.data, 'movies-add', `Added ${r.data.name} (${r.data.channel_id}). `);
  };

  /** saveIncludeAll saves whether every upload of c counts, then starts the job for c so its verdicts follow. */
  const saveIncludeAll = async (c: AdminVideoChannel, includeAll: boolean) => {
    setOutcome(null);
    setStarting(true);
    const r = await adminRequest<unknown>(`/api/movies/channels/${encodeURIComponent(c.channel_id)}`, {
      method: 'PATCH',
      json: { include_all: includeAll },
    });
    if (!r.ok) {
      setStarting(false);
      setOutcome(outcomeOf(r, '', `channel-${c.channel_id}`));
      return;
    }
    await startRun(c, `channel-${c.channel_id}`, `${c.name} now ${includeAll ? 'includes all uploads' : 'uses the TTA filter'}. `);
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

  const overrideVideo = async (v: AdminVideo, choice: OverrideChoice) => {
    setOutcome(null);
    const r = await adminRequest<unknown>(`/api/movies/videos/${encodeURIComponent(v.video_id)}`, {
      method: 'PATCH',
      json: { override: overrideOf(choice) },
    });
    if (!r.ok) {
      setOutcome(outcomeOf(r, '', `video-${v.video_id}`));
      return;
    }
    const done = { verdict: 'left to its verdict', show: 'shown whatever its verdict', hide: 'kept off whatever its verdict' }[choice];
    setOutcome(succeeded(`“${v.title}” is ${done}.`, `video-${v.video_id}`));
    await reload();
  };

  const zone = viewerZone();
  const at = (iso: string) => formatInstant(iso, zone);
  const shownVideos =
    state.kind === 'ready' ? videosMatching(state.data.videos, filters, (iso) => clockOrUtc(zone, iso).day, todayIn(zone, new Date(), zone)) : [];
  const creators = state.kind === 'ready' ? state.data.channels.map((c) => ({ value: c.channel_id, label: c.name })) : [];
  const shownPage = videoPage(shownVideos, page);

  return (
    <AdminCard id="movies-admin-heading" title="Movies">
      {dialog.element}
      <p className="muted">
        YouTube creators whose Through the Ages videos appear on the Movies page. YouTube sends each new video as it is
        published. The Movies job reads every creator's videos, renews their subscriptions and drops videos YouTube
        no longer shows: once a day, for a new creator when added, and when started here, for every creator or
        one.
      </p>

      <div className="movies-run">
        <button type="button" className="btn-antique" disabled={running || starting} onClick={() => void startRun(null, 'movies-run')}>
          {running ? 'The Movies job is running…' : 'Run now'}
        </button>
        {running && <span className="muted">This page refreshes until it ends.</span>}
      </div>

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
          <PeoplePick label="Linked TTA player (optional)" value={player} onChange={setPlayer} choices={playerChoices} />
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
              <AdminTable caption="The YouTube creators indexed, each with its linked player, filter and subscription" className="movies-table">
                <thead>
                  <tr>
                    <th>Creator</th>
                    <th>Channel ID</th>
                    <th>Linked player</th>
                    <th>Filter</th>
                    <th>Videos</th>
                    <th>Subscription</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {state.data.channels.map((c) => {
                    const status = channelStatus(c, new Date(), at);
                    return (
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
                        <RowSelect
                          label={`Which of ${c.name}'s uploads count`}
                          title={running ? 'Wait for the Movies job to end' : 'Which uploads count: those the TTA check takes, or all'}
                          value={c.include_all ? 'all' : 'tta'}
                          options={UPLOADS}
                          disabled={running || starting}
                          onChange={(v) => void saveIncludeAll(c, v === 'all')}
                        />
                      </td>
                      <td>
                        {c.videos_found} found · {c.video_count} shown
                      </td>
                      <td>
                        <span className={`movies-status movies-status-${{ ok: 'visible', waiting: 'hidden', failing: 'failing' }[status.kind]}`}>
                          {status.text}
                        </span>
                      </td>
                      <td className="movies-actions">
                        <button
                          type="button"
                          className="btn-settings"
                          disabled={running || starting}
                          onClick={() => void startRun(c, `channel-${c.channel_id}`)}
                          title="Start the Movies job for this creator alone"
                        >
                          Fetch now
                        </button>
                        <button
                          type="button"
                          className="btn-settings"
                          onClick={(e) => void deleteChannel(c, e.currentTarget)}
                        >
                          Remove
                        </button>
                      </td>
                    </tr>
                    );
                  })}
                </tbody>
              </AdminTable>
            )}
          </div>

          <div>
            <h3 className="movies-subheading">
              Videos ({shownVideos.length === state.data.videos.length ? state.data.videos.length : `${shownVideos.length} of ${state.data.videos.length}`})
            </h3>
            {state.data.videos.length > 0 && (
              <div className="movies-filter-bar">
                <label>
                  Title
                  <input className="input-antique" type="search" placeholder="Search titles…" value={filters.title} onChange={(e) => filter({ title: e.target.value })} />
                </label>
                <MultiPick label="Creator" options={creators} ticked={filters.creators} onChange={(creators) => filter({ creators })} all="All creators" noun="creators" />
                <label>
                  Published
                  <select className="input-antique" value={filters.date} onChange={(e) => filter({ date: e.target.value as VideoFilters['date'] })}>
                    {DATE_PRESETS.map((p) => (
                      <option key={p.value} value={p.value}>
                        {p.label}
                      </option>
                    ))}
                  </select>
                </label>
                {filters.date === 'custom' && (
                  <>
                    <label>
                      From
                      <input className="input-antique" type="date" value={filters.from} onChange={(e) => filter({ from: e.target.value })} />
                    </label>
                    <label>
                      To
                      <input className="input-antique" type="date" value={filters.to} onChange={(e) => filter({ to: e.target.value })} />
                    </label>
                  </>
                )}
                <MultiPick label="Status" options={STATUSES} ticked={filters.statuses} onChange={(statuses) => filter({ statuses })} all="Any status" noun="statuses" />
                <MultiPick label="Reason" options={REASONS} ticked={filters.reasons} onChange={(reasons) => filter({ reasons })} all="Any reason" noun="reasons" />
                <MultiPick label="Shown by" options={SHOWN_BY} ticked={filters.shownBy} onChange={(shownBy) => filter({ shownBy })} all="Anyone" noun="choices" />
                <button type="button" className="btn-antique" onClick={() => filter(NO_VIDEO_FILTERS)}>
                  Clear filters
                </button>
              </div>
            )}
            {state.data.videos.length === 0 ? (
              <p className="muted">No videos yet. The Movies job fills them in when a creator is added, or on Run now.</p>
            ) : (
              <AdminTable caption="The videos found, with each one's creator, date, whether the Movies page shows it and why" className="movies-table">
                <thead>
                  <tr>
                    <th>Video</th>
                    <th>Creator</th>
                    <th>Published</th>
                    <th>Status</th>
                    <th>Shown by</th>
                  </tr>
                </thead>
                <tbody>
                  {shownVideos.length === 0 && (
                    <tr>
                      <td colSpan={5} className="muted">
                        No video matches these filters.
                      </td>
                    </tr>
                  )}
                  {shownPage.rows.map((v) => {
                    const status = videoStatus(v);
                    return (
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
                      <td className="movies-date">{formatDate(v.published_at, zone)}</td>
                      <td>
                        <span className={status.shown ? 'movies-status movies-status-visible' : 'movies-status movies-status-hidden'}>
                          {status.shown ? 'Shown' : 'Not shown'}
                        </span>
                        <span className="muted movies-why">{status.why}</span>
                      </td>
                      <td>
                        <RowSelect
                          label={`Who decides whether “${v.title}” is shown`}
                          value={overrideChoice(v)}
                          options={SHOWN_BY}
                          onChange={(choice) => void overrideVideo(v, choice)}
                        />
                      </td>
                    </tr>
                    );
                  })}
                </tbody>
              </AdminTable>
            )}
            {shownVideos.length > 0 && <NewerOlder page={shownPage.page} pages={shownPage.pages} onPage={setPage} />}
          </div>
        </div>
      )}
    </AdminCard>
  );
}
