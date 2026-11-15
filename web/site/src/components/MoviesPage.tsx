/** The public Movies page: chronological YouTube thumbnail cards, creator filter pills, and a nocookie modal player. */
import { useEffect, useRef, useState } from 'react';
import { browserStorage } from '@ttac/shared/storage';
import { seemsSignedIn } from '../lib/account';
import {
  creatorVideoCounts,
  filterVideosByCreators,
  formatMovieDate,
  youtubeEmbedUrl,
  youtubeWatchUrl,
  type MoviesFeed,
  type MovieVideo,
} from '../lib/movies';
import './MoviesPage.css';
import PageHeader from '@ttac/shared/PageHeader';

type FeedState =
  | { kind: 'loading' }
  | { kind: 'error'; message: string }
  | { kind: 'ready'; data: MoviesFeed };

function MoviesHero() {
  return (
    <PageHeader title="Movies" subtitle="Recent Through the Ages videos from community creators, newest first." />
  );
}

export default function MoviesPage({ apiUrl }: { apiUrl: string }) {
  const [state, setState] = useState<FeedState>({ kind: 'loading' });
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [active, setActive] = useState<MovieVideo | null>(null);
  const dialogRef = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    let cancelled = false;
    fetch(`${apiUrl}/v1/movies`)
      .then(async (r) => {
        if (!r.ok) throw new Error(`Failed to load videos (${r.status})`);
        return (await r.json()) as MoviesFeed;
      })
      .then((data) => {
        if (!cancelled) setState({ kind: 'ready', data });
      })
      .catch((err: unknown) => {
        if (!cancelled) setState({ kind: 'error', message: err instanceof Error ? err.message : 'Could not load videos' });
      });

    if (seemsSignedIn(browserStorage())) {
      fetch(`${apiUrl}/v1/me/movies/seen`, { method: 'POST', credentials: 'include' })
        .then((r) => {
          if (r.ok) window.dispatchEvent(new Event('ttac:movies-seen'));
        })
        .catch(() => undefined);
    }

    return () => {
      cancelled = true;
    };
  }, [apiUrl]);

  useEffect(() => {
    const dlg = dialogRef.current;
    if (!dlg) return;
    if (active && !dlg.open) dlg.showModal();
    if (!active && dlg.open) dlg.close();
  }, [active]);

  const toggleCreator = (channelId: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(channelId)) next.delete(channelId);
      else next.add(channelId);
      return next;
    });
  };

  if (state.kind === 'loading') {
    return (
      <div className="movies-page">
        <MoviesHero />
        <p className="movies-status rating-card">Loading videos…</p>
      </div>
    );
  }
  if (state.kind === 'error') {
    return (
      <div className="movies-page">
        <MoviesHero />
        <p className="movies-status movies-error rating-card">{state.message}</p>
      </div>
    );
  }

  const { creators, videos } = state.data;
  const counts = creatorVideoCounts(videos);
  const visible = filterVideosByCreators(videos, selected);

  return (
    <div className="movies-page">
      <MoviesHero />

      {creators.length > 0 && (
        <div className="movies-filters rating-card" role="group" aria-label="Filter by creator">
          <button
            type="button"
            className={`movies-filter-btn${selected.size === 0 ? ' active' : ''}`}
            aria-pressed={selected.size === 0}
            onClick={() => setSelected(new Set())}
          >
            All creators ({videos.length})
          </button>
          {creators.map((c) => {
            const on = selected.has(c.channel_id);
            const count = counts.get(c.channel_id) ?? 0;
            return (
              <button
                key={c.channel_id}
                type="button"
                className={`movies-filter-btn${on ? ' active' : ''}`}
                aria-pressed={on}
                onClick={() => toggleCreator(c.channel_id)}
              >
                {c.name} ({count})
              </button>
            );
          })}
        </div>
      )}

      {visible.length === 0 ? (
        <p className="movies-status rating-card">No videos match the selected creators yet.</p>
      ) : (
        <div className="movies-grid">
          {visible.map((v) => (
            <article key={v.video_id} className="movie-card rating-card">
              <button
                type="button"
                className="movie-thumb-btn"
                onClick={() => setActive(v)}
                aria-label={`Play ${v.title} by ${v.creator_name}`}
              >
                <img
                  src={v.thumbnail_url}
                  alt=""
                  loading="lazy"
                  width="480"
                  height="360"
                  className="movie-thumb-img"
                />
                <span className="movie-play-badge" aria-hidden="true">▶</span>
              </button>
              <div className="movie-card-body">
                <h2 className="movie-title">
                  <button type="button" className="movie-title-btn" onClick={() => setActive(v)}>
                    {v.title}
                  </button>
                </h2>
                <div className="movie-meta">
                  <span className="movie-creator">
                    {v.creator_player ? (
                      <a href={`/ratings/player/?name=${encodeURIComponent(v.creator_player)}`}>
                        {v.creator_name}
                      </a>
                    ) : (
                      v.creator_name
                    )}
                  </span>
                  <time dateTime={v.published_at} className="movie-date">
                    {formatMovieDate(v.published_at)}
                  </time>
                </div>
              </div>
            </article>
          ))}
        </div>
      )}

      <dialog ref={dialogRef} className="movie-modal" onClose={() => setActive(null)}>
        {active && (
          <div className="movie-modal-inner">
            <div className="movie-modal-header">
              <div>
                <h2 className="movie-modal-title">{active.title}</h2>
                <p className="movie-modal-meta">
                  {active.creator_name} · {formatMovieDate(active.published_at)}
                </p>
              </div>
              <button
                type="button"
                className="movie-modal-close"
                onClick={() => setActive(null)}
                aria-label="Close video player"
              >
                ✕
              </button>
            </div>
            <div className="movie-modal-frame-wrap">
              <iframe
                src={youtubeEmbedUrl(active.video_id)}
                title={active.title}
                allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                allowFullScreen
                className="movie-modal-frame"
              />
            </div>
            <div className="movie-modal-footer">
              <a
                href={youtubeWatchUrl(active.video_id)}
                target="_blank"
                rel="noopener noreferrer"
                className="movie-yt-link"
              >
                Watch on YouTube
              </a>
            </div>
          </div>
        )}
      </dialog>
    </div>
  );
}
