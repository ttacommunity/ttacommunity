/** The public Movies page: chronological YouTube thumbnail cards, a creator filter, and a nocookie player in a sheet. */
import { useEffect, useRef, useState } from 'react';
import { browserStorage } from '@ttac/shared/storage';
import { seemsSignedIn } from '../lib/account';
import {
  creatorVideoCounts,
  creatorsOfPlayer,
  filterVideosByCreators,
  formatMovieDate,
  youtubeEmbedUrl,
  youtubeWatchUrl,
  type MoviesFeed,
  type MovieVideo,
} from '../lib/movies';
import './MoviesPage.css';
import PageHeader from '@ttac/shared/PageHeader';
import Sheet from '@ttac/shared/Sheet';
import Message from './Message';
import Tile, { TileLink } from './Tile';
import ViewBar from './ViewBar';

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
  const [active, setActive] = useState<{ video: MovieVideo; opener: HTMLElement } | null>(null);

  useEffect(() => {
    let cancelled = false;
    fetch(`${apiUrl}/v1/movies`)
      .then(async (r) => {
        if (!r.ok) throw new Error(`Failed to load videos (${r.status})`);
        return (await r.json()) as MoviesFeed;
      })
      .then((data) => {
        if (cancelled) return;
        setSelected(creatorsOfPlayer(data.creators, window.location.search));
        setState({ kind: 'ready', data });
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
        <Message role="status">Loading videos…</Message>
      </div>
    );
  }
  if (state.kind === 'error') {
    return (
      <div className="movies-page">
        <MoviesHero />
        <Message role="alert">{state.message}</Message>
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
        <ViewBar>
          <div className="view-group" role="group" aria-label="Filter by creator">
            <span className="view-bar-label">Creator:</span>
            <button
              type="button"
              className={selected.size === 0 ? 'format-btn active' : 'format-btn'}
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
                  className={on ? 'format-btn active' : 'format-btn'}
                  aria-pressed={on}
                  onClick={() => toggleCreator(c.channel_id)}
                >
                  {c.name} ({count})
                </button>
              );
            })}
          </div>
        </ViewBar>
      )}

      {visible.length === 0 ? (
        <Message>No videos match the selected creators yet.</Message>
      ) : (
        <div className="movies-grid">
          {visible.map((v) => (
            <Tile
              key={v.video_id}
              picture={
                <div className="movie-thumb">
                  <img src={v.thumbnail_url} alt="" loading="lazy" width="480" height="360" className="movie-thumb-img" />
                  <span className="movie-play-badge" aria-hidden="true">
                    ▶
                  </span>
                </div>
              }
            >
              <div className="movie-card-body">
                <h2 className="movie-title">
                  <TileLink onClick={(opener) => setActive({ video: v, opener })} label={`Play ${v.title} by ${v.creator_name}`}>
                    {v.title}
                  </TileLink>
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
            </Tile>
          ))}
        </div>
      )}

      {active !== null && <MoviePlayer video={active.video} opener={active.opener} onClose={() => setActive(null)} />}
    </div>
  );
}

/** MoviePlayer plays one video in a wide sheet: its creator and date, the video, and a link to watch it on YouTube. */
function MoviePlayer({ video, opener, onClose }: { video: MovieVideo; opener: HTMLElement; onClose: () => void }) {
  const box = useRef<HTMLDialogElement>(null);
  return (
    <Sheet dialog={box} id="movie-player" title={video.title} opener={opener} onClose={onClose} holds="video">
      <div className="sheet-body">
        <p className="movie-player-meta">
          {video.creator_name} · {formatMovieDate(video.published_at)}
        </p>
        <div className="movie-player-frame">
          <iframe
            src={youtubeEmbedUrl(video.video_id)}
            title={video.title}
            allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
            allowFullScreen
          />
        </div>
        <p className="movie-player-link">
          <a href={youtubeWatchUrl(video.video_id)} target="_blank" rel="noopener noreferrer">
            Watch on YouTube
          </a>
        </p>
      </div>
    </Sheet>
  );
}
