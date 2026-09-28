/** The leaderboard for the current run, with upstream's options, filters, search, sort and paging. */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { formatValues } from '@ttac/shared/api';
import { currentRun, leaderboard, type Leaderboard as Board } from '@ttac/shared/ratings';
import Segmented from './Segmented';
import SearchBox from './SearchBox';
import OptionsDrawer, { type LinkTo } from './OptionsDrawer';
import { Flag, TitleBadge } from './PlayerMarks';
import * as show from '../lib/leaderboard-display';
import {
  DEFAULT_VIEW,
  hasFilters,
  hasOptions,
  modelOf,
  nextSort,
  page,
  parseView,
  rankPool,
  search,
  sortRows,
  titleOptions,
  viewParams,
  type RankedRow,
  type SortKey,
  type View,
} from '../lib/leaderboard-view';
import { recall, remember } from '../lib/remembered';
import './Leaderboard.css';

type State = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; board: Board };

const formatOptions = formatValues.map((f) => ({ value: f, label: show.formatName(f) }));
const OPTIONS_OPEN = 'ttac.leaderboard.options';
const narrow = () => window.innerWidth <= 768;

function readView(): View {
  return parseView(new URLSearchParams(window.location.search), recall(window.localStorage));
}

/** Shows the chosen leaderboard. Every choice is part of the address, so a view can be shared. */
export default function Leaderboard({ apiUrl }: { apiUrl: string }) {
  const [view, setView] = useState<View>(readView);
  const [state, setState] = useState<State>({ kind: 'loading' });
  const [optionsOpen, setOptionsOpen] = useState(false);
  const model = modelOf(view);

  const go = useCallback((next: View) => {
    window.history.pushState(null, '', `?${viewParams(next, true)}`);
    remember(window.localStorage, next);
    setView(next);
  }, []);
  const linkTo: LinkTo = (next) => ({
    href: `?${viewParams(next, true)}`,
    onClick: (e) => {
      if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
      e.preventDefault();
      go(next);
    },
  });

  useEffect(() => {
    const onPop = () => setView(readView());
    window.addEventListener('popstate', onPop);
    remember(window.localStorage, readView());
    try {
      if (!narrow()) setOptionsOpen(window.localStorage.getItem(OPTIONS_OPEN) === 'true');
    } catch {
      // Without storage the drawer starts closed.
    }
    return () => window.removeEventListener('popstate', onPop);
  }, []);

  useEffect(() => {
    let stale = false;
    setState({ kind: 'loading' });
    currentRun(apiUrl)
      .then((run) => leaderboard(apiUrl, run.run_id, model, view.format))
      .then((board) => !stale && setState({ kind: 'ready', board }))
      .catch((e: unknown) => !stale && setState({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
    return () => {
      stale = true;
    };
  }, [apiUrl, model, view.format]);

  const toggleOptions = (open: boolean) => {
    setOptionsOpen(open);
    try {
      window.localStorage.setItem(OPTIONS_OPEN, String(open));
    } catch {
      // The drawer simply starts closed next time.
    }
    document.body.style.overflow = open && narrow() ? 'hidden' : '';
  };

  const players = useMemo(() => (state.kind === 'ready' ? state.board.players : []), [state]);
  const pool = useMemo(() => rankPool(players, view), [players, view]);
  const found = useMemo(() => search(pool, view.search), [pool, view.search]);
  const shown = useMemo(
    () => page(sortRows(found.rows, view.sort, view.order), view.page, found.jumpTo),
    [found, view.sort, view.order, view.page],
  );

  return (
    <div className={optionsOpen ? 'leaderboard-layout options-open' : 'leaderboard-layout'}>
      <div className="rating-card leaderboard">
        <div className="format-bar">
          <Segmented label="Format" options={formatOptions} value={view.format} onChange={(format) => go({ ...view, format, page: 1 })} />
          <div className="format-bar-actions">
            <SearchBox players={players} value={view.search} onSearch={(term) => go({ ...view, search: term, page: 1 })} />
            <button
              type="button"
              className={['btn-options-toggle', hasOptions(view) && 'active', optionsOpen && 'open'].filter(Boolean).join(' ')}
              title="Toggle advanced rating options, season archive, and filters"
              aria-expanded={optionsOpen}
              onClick={() => toggleOptions(!optionsOpen)}
            >
              <span>⚙️ Options</span>
              {hasOptions(view) && <span className="options-active-badge">Active</span>}
              <span className="btn-options-toggle-arrow">{optionsOpen ? '✕' : '▶'}</span>
            </button>
          </div>
        </div>

        <div className="leaderboard-heading">
          <div>
            <h1 className="rating-card-heading">Competitive Leaderboard</h1>
            <p className="leaderboard-engine">
              Engine: <strong>{show.engineName(model)}</strong>
              <span className="badge badge-format">{show.formatName(view.format)}</span>
              <span className="badge badge-mode">{show.resetModeName(model)}</span>
              {show.engineNote(model) !== '' && <span className="leaderboard-engine-note">({show.engineNote(model)})</span>}
            </p>
          </div>
          {state.kind === 'ready' && (
            <p className="leaderboard-count">
              Players: <strong>{found.rows.length.toLocaleString('en')}</strong>{' '}
              <span className={`leaderboard-status status-${view.status}`}>
                {view.status === 'active' ? '(Active Only)' : view.status === 'inactive' ? '(Inactive Only)' : '(All Recorded)'}
              </span>
              {view.minOpps > 0 && (
                <>
                  {' '}
                  <span className="leaderboard-status status-min">(&ge; {view.minOpps} matches)</span>
                </>
              )}
            </p>
          )}
        </div>

        <div className="leaderboard-filters">
          <div className="leaderboard-pills">
            {view.search !== '' && (
              <span className="filter-pill">
                Search: &quot;{view.search}&quot;{' '}
                <a {...linkTo({ ...view, search: '', page: 1 })} aria-label="Clear search">
                  ✕
                </a>
              </span>
            )}
            {view.title !== null && (
              <span className="filter-pill">
                Title: {view.title}{' '}
                <a {...linkTo({ ...view, title: null, page: 1 })} aria-label="Clear title filter">
                  ✕
                </a>
              </span>
            )}
            {view.status !== DEFAULT_VIEW.status && (
              <span className="filter-pill">
                Status: {view.status === 'all' ? 'All' : 'Inactive'}{' '}
                <a {...linkTo({ ...view, status: DEFAULT_VIEW.status, page: 1 })} aria-label="Clear status filter">
                  ✕
                </a>
              </span>
            )}
            {view.minOpps !== DEFAULT_VIEW.minOpps && (
              <span className="filter-pill">
                Min Matches: &ge;{view.minOpps}{' '}
                <a {...linkTo({ ...view, minOpps: DEFAULT_VIEW.minOpps, page: 1 })} aria-label="Clear minimum matches">
                  ✕
                </a>
              </span>
            )}
            {hasFilters(view) && (
              <a {...linkTo({ ...DEFAULT_VIEW, engine: view.engine, reset: view.reset, retro: view.retro, format: view.format })} className="btn-reset">
                Clear Filters
              </a>
            )}
          </div>
          <div>
            Sorted by: <strong>{sortNames[view.sort]}</strong> ({view.order.toUpperCase()})
          </div>
        </div>

        {state.kind === 'loading' && (
          <p role="status" className="leaderboard-message">
            Loading…
          </p>
        )}
        {state.kind === 'error' && (
          <p role="alert" className="leaderboard-message">
            Could not load the leaderboard: {state.message}
          </p>
        )}
        {state.kind === 'ready' && (
          <>
            <div className="scroll-hint">
              <span>Swipe table horizontally for full stats ➔</span>
            </div>
            <Table
              view={view}
              rows={shown.rows}
              jumpTo={found.jumpTo}
              linkTo={linkTo}
              caption={`${show.modelLabel(model)}, ${show.formatName(view.format)}, run ${state.board.run_id}`}
            />
            <Pagination view={view} at={shown} linkTo={linkTo} go={go} />
          </>
        )}

        <section className="rating-card rating-card-note" aria-labelledby="leaderboard-guide">
          <h2 id="leaderboard-guide">Leaderboard Mechanics &amp; Rating Principles</h2>
          <p>
            Rankings are calculated dynamically across all recorded competitive Through the Ages tournaments. The primary
            sorting metric is the{' '}
            <strong>
              Conservative Rating <i>C</i> = <i>μ</i> − 3<i>σ</i>
            </strong>
            , guaranteeing that a player&apos;s standing represents an outcome that they can outperform 99.7% of the time.
          </p>
        </section>
      </div>

      {optionsOpen && (
        <Sidebar onClose={() => toggleOptions(false)}>
          <OptionsDrawer view={view} titles={titleOptions(players)} linkTo={linkTo} go={go} onClose={() => toggleOptions(false)} />
        </Sidebar>
      )}
    </div>
  );
}

const sortNames: Record<SortKey, string> = {
  rank: 'Rank',
  name: 'Name',
  c_rating: 'C Rating',
  rating: 'Rating',
  opps: 'Opps',
  win_rate: 'Win Rate',
  last_played: 'Last Played',
};

/** Sidebar places the drawer in the free space right of the card, as upstream does, or as a sheet on phones. */
function Sidebar({ children, onClose }: { children: React.ReactNode; onClose: () => void }) {
  const ref = useRef<HTMLElement>(null);

  useEffect(() => {
    const place = () => {
      const side = ref.current;
      const layout = side?.parentElement;
      if (!side || !layout) return;
      if (narrow()) {
        side.style.left = '';
        side.style.right = '';
        return;
      }
      const card = layout.getBoundingClientRect();
      const width = document.documentElement.clientWidth;
      const own = side.offsetWidth || 350;
      const ideal = card.right + (width - card.right) / 2 - own / 2;
      const left = Math.max(16, Math.min(ideal, width - own - 16));
      side.style.left = `${left - card.left}px`;
      side.style.right = 'auto';
    };
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    place();
    window.addEventListener('resize', place);
    document.addEventListener('keydown', onKey);
    return () => {
      window.removeEventListener('resize', place);
      document.removeEventListener('keydown', onKey);
    };
  }, [onClose]);

  return (
    <aside ref={ref} className="leaderboard-sidebar">
      {/* On phones a tap on the dimmed backdrop closes the sheet, as upstream's does. */}
      <button type="button" className="sidebar-backdrop" aria-label="Close options" tabIndex={-1} onClick={onClose} />
      {children}
    </aside>
  );
}

const headers: [SortKey | null, string, string][] = [
  ['rank', 'Rank', 'Current competitive ranking based on Conservative Rating (C = μ - 3σ).'],
  [null, 'Flag', "Player's registered national flag."],
  ['name', 'Player', 'Player username.'],
  [null, 'Title', 'Official TTA Titles: WC (World Champion), GM (Grandmaster), M (Master), P (Platinum), G (Gold), S (Silver), B (Bronze), W (Wood).'],
  ['c_rating', 'C-Rating', 'Conservative Rating: C = Rating - 3 × RD (99.7% confidence lower bound). Prevents unearned top spots with high uncertainty.'],
  ['rating', 'Rating ± RD', 'Glicko-2 / WHR Skill Rating (mean μ) and Rating Deviation (RD / σ). Default starting rating is 1500 ± 350.'],
  ['opps', 'Opps', 'Total competitive opponents faced across all recorded tournament games in this format.'],
  [null, 'W-L-D', 'Total Wins, Losses, and Draws accumulated across competitive tournament matches.'],
  ['win_rate', 'Win %', 'Effective win percentage: (Wins + 0.5 × Draws) / Opponents × 100.'],
  ['last_played', 'Last Played', "Date of player's most recent competitive match. Players without games in > 12 months are retired."],
];

interface TableProps {
  view: View;
  rows: RankedRow[];
  jumpTo: string | null;
  linkTo: LinkTo;
  caption: string;
}

/** The table of one page of players, with sortable headers and the searched player highlighted. */
function Table({ view, rows, jumpTo, linkTo, caption }: TableProps) {
  const jumped = useRef<HTMLTableRowElement>(null);
  useEffect(() => {
    const t = setTimeout(() => jumped.current?.scrollIntoView({ behavior: 'smooth', block: 'center' }), 150);
    return () => clearTimeout(t);
  }, [jumpTo]);

  return (
    <div className="table-responsive">
      <table className="leaderboard-table">
        <caption className="visually-hidden">{caption}</caption>
        <thead>
          <tr>
            {headers.map(([key, name, tip], i) => (
              <th key={name} scope="col" className={i === 0 ? 'col-sticky-rank' : undefined}>
                {key === null ? (
                  <span data-tooltip={tip}>{name}</span>
                ) : (
                  <a {...linkTo({ ...view, ...nextSort(view, key), page: 1 })} data-tooltip={tip}>
                    {name}
                  </a>
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((p) => (
            <tr key={p.name} ref={p.name === jumpTo ? jumped : undefined} className={p.name === jumpTo ? 'jump-highlight' : undefined}>
              <td className={show.isTopThree(p.displayRank) ? 'col-sticky-rank rank-col top-3-rank' : 'col-sticky-rank rank-col'}>
                {show.rank(p.displayRank)}
              </td>
              <td>
                <Flag country={p.country} />
              </td>
              <th scope="row">
                <span className="player-name">{p.name}</span>
                {p.provisional && (
                  <span className="calibrating-badge" data-tooltip={`Provisional / Prior Calibration (${p.opponents}/15 matches completed)`}>
                    ⏳
                  </span>
                )}
                {!p.active && view.status === 'all' && (
                  <span className="retired-badge" data-tooltip="Inactive / Retired (no competitive match in > 12 months)">
                    Retired
                  </span>
                )}
              </th>
              <td>
                <TitleBadge title={p.title} />
              </td>
              <td>
                <span className="c-rating-val">{show.cRating(p.c_rating)}</span>
              </td>
              <td>
                <span className="rating-primary">{show.rating(p.rating)}</span>{' '}
                <span className="rating-rd">{show.rd(p.rd)}</span>
              </td>
              <td>{p.opponents}</td>
              <td>
                <span className="wld-win">{p.wins}</span> - <span className="wld-loss">{p.losses}</span> -{' '}
                <span className="wld-draw">{p.draws}</span>
              </td>
              <td>
                <strong>{show.winRate(p.win_rate)}</strong>
              </td>
              <td className="last-played">{p.last_played ?? '-'}</td>
            </tr>
          ))}
          {rows.length === 0 && (
            <tr>
              <td colSpan={headers.length} className="empty">
                No players found matching your criteria.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

interface PaginationProps {
  view: View;
  at: { page: number; pages: number; total: number };
  linkTo: LinkTo;
  go: (view: View) => void;
}

/** Pagination links to the first, previous, next and last pages, and jumps to a typed page number. */
function Pagination({ view, at, linkTo, go }: PaginationProps) {
  const [typed, setTyped] = useState(String(at.page));
  useEffect(() => setTyped(String(at.page)), [at.page]);
  const to = (n: number) => linkTo({ ...view, page: n });

  return (
    <nav className="pagination" aria-label="Pages">
      <div>
        Showing page <strong>{at.page}</strong> of <strong>{at.pages}</strong> ({at.total.toLocaleString('en')} players)
      </div>
      <div className="pagination-links">
        {at.page > 1 && (
          <>
            <a {...to(1)} title="Jump to First Page">
              &laquo; First
            </a>
            <a {...to(at.page - 1)} title="Previous Page">
              &lsaquo; Prev
            </a>
          </>
        )}
        <form
          className="page-jumper"
          onSubmit={(e) => {
            e.preventDefault();
            const n = Number.parseInt(typed, 10);
            go({ ...view, page: Number.isNaN(n) ? 1 : Math.min(Math.max(n, 1), at.pages) });
          }}
        >
          <span>Page</span>
          <input type="number" min={1} max={at.pages} aria-label="Page number" value={typed} onChange={(e) => setTyped(e.target.value)} />
          <span>/ {at.pages}</span>
          <button type="submit" title="Go to page">
            Go
          </button>
        </form>
        {at.page < at.pages && (
          <>
            <a {...to(at.page + 1)} title="Next Page">
              Next &rsaquo;
            </a>
            <a {...to(at.pages)} title="Jump to Last Page">
              Last &raquo;
            </a>
          </>
        )}
      </div>
    </nav>
  );
}
