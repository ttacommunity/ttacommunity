/** The leaderboard for the current run, with upstream's options, filters, search, sort and paging. */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { formatValues } from '@ttac/shared/api';
import {
  currentRun,
  history,
  leaderboard,
  yearLeaderboard,
  years as fetchYears,
  type Leaderboard as Board,
  type YearEntry,
  type YearLeaderboard,
} from '@ttac/shared/ratings';
import Segmented from './Segmented';
import SearchBox from './SearchBox';
import OptionsDrawer, { type LinkTo } from './OptionsDrawer';
import YearBanner from './YearBanner';
import { Flag, TitleBadge } from './PlayerMarks';
import * as show from '../lib/leaderboard-display';
import {
  DEFAULT_VIEW,
  deltaDates,
  inactiveCutoff,
  csvUrl,
  defaultMinOpps,
  deltas,
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
  yearDeltas,
  yearRows,
  type Delta,
  type RankedRow,
  type SortKey,
  type View,
} from '../lib/leaderboard-view';
import { playerHref } from '../lib/player-view';
import { recall, remember } from '../lib/remembered';
import './Leaderboard.css';

type State =
  | { kind: 'loading' }
  | { kind: 'error'; message: string }
  | { kind: 'ready'; board: Board; year: YearLeaderboard | null; lastGame: string | null };

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
      .then(async (run): Promise<State> => {
        if (view.year === null)
          return { kind: 'ready', board: await leaderboard(apiUrl, run.run_id, model, view.format), year: null, lastGame: run.last_game };
        const year = await yearLeaderboard(apiUrl, run.run_id, view.year, model, view.format);
        return { kind: 'ready', board: { run_id: year.run_id, model, format: view.format, players: [] }, year, lastGame: run.last_game };
      })
      .then((next) => !stale && setState(next))
      .catch((e: unknown) => !stale && setState({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
    return () => {
      stale = true;
    };
  }, [apiUrl, model, view.format, view.year]);

  const [archive, setArchive] = useState<YearEntry[] | null>(null);
  useEffect(() => {
    let stale = false;
    currentRun(apiUrl)
      .then((run) => fetchYears(apiUrl, run.run_id))
      .then((y) => !stale && setArchive(y.years))
      .catch(() => !stale && setArchive([]));
    return () => {
      stale = true;
    };
  }, [apiUrl]);

  const toggleOptions = (open: boolean) => {
    setOptionsOpen(open);
    try {
      window.localStorage.setItem(OPTIONS_OPEN, String(open));
    } catch {
      // The drawer simply starts closed next time.
    }
    document.body.style.overflow = open && narrow() ? 'hidden' : '';
  };

  const yearBoard = state.kind === 'ready' ? state.year : null;
  const fromYear = useMemo(() => (yearBoard === null ? null : yearRows(yearBoard, view.stats)), [yearBoard, view.stats]);
  const players = useMemo(() => fromYear?.rows ?? (state.kind === 'ready' ? state.board.players : []), [fromYear, state]);
  const [changes, setChanges] = useState<Map<string, Delta> | null>(null);

  useEffect(() => {
    setChanges(null);
    if (state.kind !== 'ready' || view.delta === 'none') return;
    if (state.year !== null) {
      setChanges(yearDeltas(state.year));
      return;
    }
    let stale = false;
    const { board } = state;
    history(apiUrl, board.run_id, board.model, board.format, deltaDates(state.lastGame)[view.delta])
      .then((past) => !stale && setChanges(deltas(board.players, past.players)))
      .catch(() => !stale && setChanges(new Map()));
    return () => {
      stale = true;
    };
  }, [apiUrl, state, view.delta]);
  const cutoff = inactiveCutoff(state.kind === 'ready' ? state.lastGame : null);
  const pool = useMemo(() => rankPool(players, view, fromYear?.played, cutoff), [players, view, fromYear, cutoff]);
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

        {yearBoard !== null && (
          <YearBanner board={yearBoard} latest={archive?.[0]?.year === yearBoard.year} ranked={found.rows.length} view={view} linkTo={linkTo} />
        )}

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
            {view.delta !== DEFAULT_VIEW.delta && (
              <span className="filter-pill">
                &Delta;: {deltaNames[view.delta]}{' '}
                <a {...linkTo({ ...view, delta: DEFAULT_VIEW.delta, page: 1 })} aria-label="Turn off deltas">
                  ✕
                </a>
              </span>
            )}
            {view.minOpps !== defaultMinOpps(view.year) && (
              <span className="filter-pill">
                Min Matches: &ge;{view.minOpps}{' '}
                <a {...linkTo({ ...view, minOpps: defaultMinOpps(view.year), page: 1 })} aria-label="Clear minimum matches">
                  ✕
                </a>
              </span>
            )}
            {hasFilters(view) && (
              <a
                {...linkTo({
                  ...DEFAULT_VIEW,
                  engine: view.engine,
                  reset: view.reset,
                  retro: view.retro,
                  format: view.format,
                  year: view.year,
                  stats: view.stats,
                  minOpps: defaultMinOpps(view.year),
                })}
                className="btn-reset"
              >
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
              changes={changes}
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
          <OptionsDrawer
            view={view}
            titles={titleOptions(players)}
            years={archive}
            downloads={
              state.kind === 'ready'
                ? {
                    shown: csvUrl(apiUrl, state.board.run_id, view, false),
                    everyone: csvUrl(apiUrl, state.board.run_id, view, true),
                    count: found.rows.length,
                  }
                : null
            }
            linkTo={linkTo}
            go={go}
            onClose={() => toggleOptions(false)}
          />
        </Sidebar>
      )}
    </div>
  );
}

const deltaNames: Record<View['delta'], string> = {
  none: 'None',
  last_update: 'Last Update',
  month: 'Month',
  quarter: 'Quarter',
  year: 'Year',
};

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

/** wldName is the W-L-D heading: in a year's archive it names the year, or the career. */
function wldName(view: View): string {
  if (view.year === null) return 'W-L-D';
  return view.stats === 'career' ? 'W-L-D (Career)' : `W-L-D (${view.year})`;
}

function wldTip(view: View): string {
  if (view.year === null) return 'Total Wins, Losses, and Draws accumulated across competitive tournament matches.';
  const how = '(Glicko-2 & WHR ratings are calculated based on the chosen reset mode).';
  return view.stats === 'career'
    ? `Cumulative career match record through Season ${view.year}. ${how}`
    : `Match record for Season ${view.year} only. ${how}`;
}

interface TableProps {
  view: View;
  rows: RankedRow[];
  jumpTo: string | null;
  changes: Map<string, Delta> | null;
  linkTo: LinkTo;
  caption: string;
}

/** Change shows one rating change beside its value, as upstream's delta badge. */
function Change({ value, what }: { value: number | undefined; what: string }) {
  if (value === undefined) return null;
  const d = show.delta(value);
  return (
    <span className={`delta-badge ${d.kind}`} data-tooltip={`${what} change in selected timeframe`}>
      {d.text}
    </span>
  );
}

/** The table of one page of players, with sortable headers and the searched player highlighted. */
function Table({ view, rows, jumpTo, changes, linkTo, caption }: TableProps) {
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
                  <span data-tooltip={name === 'W-L-D' ? wldTip(view) : tip}>{name === 'W-L-D' ? wldName(view) : name}</span>
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
                <a className="player-link" href={playerHref(p.name)}>
                  {p.name}
                </a>
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
                <Change value={changes?.get(p.name)?.c_rating} what="C-Rating" />
              </td>
              <td>
                <span className="rating-primary">{show.rating(p.rating)}</span>{' '}
                <span className="rating-rd">{show.rd(p.rd)}</span>
                <Change value={changes?.get(p.name)?.rating} what="Rating" />
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
