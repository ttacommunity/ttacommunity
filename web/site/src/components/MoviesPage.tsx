/** The public Movies page: YouTube thumbnail cards a page at a time, loading the next as you reach the bottom; a creator filter, a sort, and a nocookie player in a sheet. */
import { useCallback, useEffect, useRef, useState } from 'react';
import { browserStorage } from '@ttac/shared/storage';
import { count } from '@ttac/shared/numbers';
import { seemsSignedIn } from '../lib/account';
import {
  addPage,
  chosenCreators,
  everyVideo,
  formatMovieDate,
  movieSorts,
  moviesParams,
  moviesView,
  withCreator,
  youtubeEmbedUrl,
  youtubeWatchUrl,
  type MovieCreator,
  type MoviesFeed,
  type MoviesView,
  type MovieVideo,
} from '../lib/movies';
import './MoviesPage.css';
import PageHeader from '@ttac/shared/PageHeader';
import Segmented from '@ttac/shared/Segmented';
import Sheet from '@ttac/shared/Sheet';
import Message from './Message';
import Tile, { TileLink } from './Tile';
import ViewBar from './ViewBar';

type ListState =
  | { kind: 'loading' }
  | { kind: 'error'; message: string }
  | { kind: 'ready'; creators: MovieCreator[]; pages: number; total: number; videos: MovieVideo[]; first: number; last: number; more: 'idle' | 'loading' | 'error' };

function MoviesHero() {
  return <PageHeader title="Movies" subtitle="Through the Ages videos from community creators." />;
}

/** address is the page's own address for a view. */
function address(view: MoviesView): string {
  const q = moviesParams(view).toString();
  return q === '' ? window.location.pathname : `?${q}`;
}

/** getMovies fetches one page of the feed for a view. */
async function getMovies(apiUrl: string, view: MoviesView): Promise<MoviesFeed> {
  const q = moviesParams(view).toString();
  const r = await fetch(`${apiUrl}/v1/movies${q === '' ? '' : `?${q}`}`);
  if (r.status === 404) throw new Error('No videos from that creator.');
  if (!r.ok) throw new Error(`Failed to load videos (${r.status})`);
  return (await r.json()) as MoviesFeed;
}

export default function MoviesPage({ apiUrl }: { apiUrl: string }) {
  const [view, setView] = useState<MoviesView>(() => moviesView(window.location.search));
  const [list, setList] = useState<ListState>({ kind: 'loading' });
  const [active, setActive] = useState<{ video: MovieVideo; opener: HTMLElement } | null>(null);
  const request = useRef(0);
  const bottom = useRef<HTMLAnchorElement>(null);

  useEffect(() => {
    const id = ++request.current;
    setList({ kind: 'loading' });
    getMovies(apiUrl, view)
      .then((feed) => {
        if (id !== request.current) return;
        setList({ kind: 'ready', creators: feed.creators, pages: feed.pages, total: feed.total, videos: feed.videos, first: feed.page, last: feed.page, more: 'idle' });
      })
      .catch((err: unknown) => {
        if (id === request.current) setList({ kind: 'error', message: err instanceof Error ? err.message : 'Could not load videos' });
      });
  }, [apiUrl, view]);

  useEffect(() => {
    if (!seemsSignedIn(browserStorage())) return;
    fetch(`${apiUrl}/v1/me/movies/seen`, { method: 'POST', credentials: 'include' })
      .then((r) => {
        if (r.ok) window.dispatchEvent(new Event('ttac:movies-seen'));
      })
      .catch(() => undefined);
  }, [apiUrl]);

  const show = (next: MoviesView) => {
    window.history.replaceState(null, '', address(next));
    setView(next);
  };

  const loadMore = useCallback(() => {
    if (list.kind !== 'ready' || list.more === 'loading' || list.last >= list.pages) return;
    const id = request.current;
    const next = { ...view, page: list.last + 1 };
    setList({ ...list, more: 'loading' });
    getMovies(apiUrl, next)
      .then((feed) => {
        if (id !== request.current) return;
        setList((l) => (l.kind === 'ready' ? { ...l, creators: feed.creators, pages: feed.pages, total: feed.total, videos: addPage(l.videos, feed.videos), last: feed.page, more: 'idle' } : l));
        window.history.replaceState(null, '', address({ ...view, page: feed.page }));
      })
      .catch(() => {
        if (id === request.current) setList((l) => (l.kind === 'ready' ? { ...l, more: 'error' } : l));
      });
  }, [apiUrl, view, list]);

  useEffect(() => {
    const el = bottom.current;
    if (el === null || list.kind !== 'ready' || list.more !== 'idle') return;
    const seen = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) loadMore();
    }, { rootMargin: '600px 0px' });
    seen.observe(el);
    return () => seen.disconnect();
  }, [loadMore, list]);

  if (list.kind === 'loading') {
    return (
      <div className="movies-page">
        <MoviesHero />
        <Message role="status">Loading videos…</Message>
      </div>
    );
  }
  if (list.kind === 'error') {
    return (
      <div className="movies-page">
        <MoviesHero />
        <Message role="alert">{list.message}</Message>
      </div>
    );
  }

  const { creators, videos } = list;
  const chosen = chosenCreators(view, creators);

  return (
    <div className="movies-page">
      <MoviesHero />

      {creators.length > 0 && (
        <ViewBar>
          <div className="format-bar-group" role="group" aria-label="Filter by creator">
            <span className="view-bar-label">Creator:</span>
            <button
              type="button"
              className={chosen.size === 0 ? 'format-btn active' : 'format-btn'}
              aria-pressed={chosen.size === 0}
              onClick={() => show({ ...view, creators: [], player: '', page: 1 })}
            >
              All creators ({count(everyVideo(creators))})
            </button>
            {creators.map((c) => {
              const on = chosen.has(c.channel_id);
              return (
                <button
                  key={c.channel_id}
                  type="button"
                  className={on ? 'format-btn active' : 'format-btn'}
                  aria-pressed={on}
                  onClick={() => show(withCreator(view, creators, c.channel_id))}
                >
                  {c.name} ({count(c.videos)})
                </button>
              );
            })}
          </div>
          <Segmented label="Sort videos" prefix={<span className="view-bar-label">Sort:</span>} options={[...movieSorts]} value={view.sort} onChange={(sort) => show({ ...view, sort, page: 1 })} />
        </ViewBar>
      )}

      {list.first > 1 && (
        <p className="movies-earlier">
          <a href={address({ ...view, page: list.first - 1 })}>‹ Page {count(list.first - 1)}</a>
        </p>
      )}

      {videos.length === 0 ? (
        <Message>No videos from the chosen creators yet.</Message>
      ) : (
        <div className="movies-grid">
          {videos.map((v) => (
            <Tile
              key={v.video_id}
              playable
              picture={
                <div className="movie-thumb">
                  <img src={v.thumbnail_url} alt="" loading="lazy" width="480" height="360" className="movie-thumb-img" />
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

      <p className="movies-more" role="status">
        {list.total > 0 && `Showing ${count(videos.length)} of ${count(list.total)} videos`}
        {list.more === 'loading' && ' · Loading more…'}
      </p>
      {list.last < list.pages && (
        <p className="movies-more">
          <a
            ref={bottom}
            className="btn-quiet"
            href={address({ ...view, page: list.last + 1 })}
            onClick={(e) => {
              e.preventDefault();
              loadMore();
            }}
          >
            {list.more === 'error' ? 'Could not load more videos: try again' : 'More videos'}
          </a>
        </p>
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
