/** The community leaderboard page: standings, dynamic best-N highlights, and tournament or event columns. */
import { useEffect, useState } from 'react';
import {
  communityEditions,
  communityLeaderboard,
  currentRun,
  type CommunityEditionItem,
  type CommunityLeaderboard as LeaderboardData,
  type CommunityRow,
} from '@ttac/shared/ratings';
import { Flag } from './PlayerMarks';
import {
  countingColumns,
  editionFacts,
  matchesSearch,
  parseCommunityView,
  titleMark,
  tournamentSums,
  whole,
  type CommunityView,
} from '../lib/community';
import { titleName } from '../lib/leaderboard-display';
import './CommunityPage.css';
import Segmented from '@ttac/shared/Segmented';
import SearchBox, { type SearchPlayer } from './SearchBox';
import PlayerLink from './PlayerLink';
import Waiting from './Waiting';
import Pager from './Pager';

type Load =
  | { kind: 'loading' }
  | { kind: 'error'; message: string }
  | { kind: 'ready'; data: LeaderboardData; editions: CommunityEditionItem[] };

/** href is the page's address for a view, carrying edition, view, search and page. */
function href(view: CommunityView, change: Partial<CommunityView>): string {
  const v = { ...view, ...change };
  const q = new URLSearchParams({ edition: v.edition, view: v.view });
  if (v.search) q.set('q', v.search);
  if (change.page !== undefined) q.set('page', String(v.page));
  return `/ratings/community/?${q}`;
}

function Rank({ rank }: { rank: number }) {
  if (rank <= 3) return <span className={`rank-pill rank-${rank}`}>{rank}</span>;
  return <span className="rank-default">{rank}</span>;
}

function Score({ value, counting, tip }: { value: number | undefined | null; counting: boolean; tip: string }) {
  if (value === undefined || value === null || value <= 0) return <span className="score-empty">—</span>;
  return (
    <span className={counting ? 'score-counting' : 'score-dropped'} title={tip}>
      {whole(value)}
    </span>
  );
}

function Hero({
  data,
  editions,
  view,
}: {
  data: LeaderboardData;
  editions: CommunityEditionItem[];
  view: CommunityView;
}) {
  const top = data.rows[0];
  const events = data.categories.reduce((n, c) => n + c.columns.length, 0);
  const info = data.info;

  return (
    <div className="comm-hero">
      <h1 className="comm-title">Community Standard Leaderboard</h1>
      <div className="comm-subtitle">{info.title || info.name}</div>
      <div className="maintainer-card">
        <div className="maintainer-badge">{info.badge || '🛡️ Circuit Maintainer'}</div>
        <div className="maintainer-desc">
          This official leaderboard is curated and maintained by{' '}
          <strong>
            <PlayerLink name={info.maintainer}>{info.maintainer}</PlayerLink>
          </strong>{' '}
          • {info.maintainer_note}
        </div>
      </div>
      <div className="edition-selector-bar">
        <span className="edition-label">📅 Leaderboard Edition:</span>
        <div className="edition-pills">
          {editions.map((e) => (
            <a
              key={e.slug}
              href={href(view, { edition: e.slug })}
              className={`edition-pill${view.edition === e.slug ? ' active' : ''}`}
              title={e.title}
            >
              {e.badge ? `${e.badge} ` : ''}
              {e.short_name || e.name}
            </a>
          ))}
        </div>
      </div>
      <div className="comm-stats-grid">
        <div className="stat-tile">
          <div className="stat-tile-num">{data.rows.length.toLocaleString('en-US')}</div>
          <div className="stat-tile-label">Ranked Competitors</div>
        </div>
        <div className="stat-tile">
          <div className="stat-tile-num">{events}</div>
          <div className="stat-tile-label">Tracked Events ({data.categories.length} Tiers)</div>
        </div>
        <div className="stat-tile">
          <div className="stat-tile-num">
            <PlayerLink name={top?.name ?? '—'} className="comm-top-link" />
          </div>
          <div className="stat-tile-label">#1 Rank ({whole(top?.points ?? 0)} pts)</div>
        </div>
        <div className="stat-tile">
          <div className="stat-tile-num">
            {info.wc ? (
              <PlayerLink name={info.wc} className="comm-wc-link">
                <span className="badge badge-wc comm-wc-badge">WC</span> {info.wc}
              </PlayerLink>
            ) : (
              <span className="comm-muted">—</span>
            )}
          </div>
          <div className="stat-tile-label">{info.wc_desc || 'Reigning World Champion'}</div>
        </div>
      </div>
    </div>
  );
}

function Explainers({ data }: { data: LeaderboardData }) {
  const facts = editionFacts(data);
  const count = facts.counting;
  return (
    <div className="explainer-cards">
      <div className="explainer-card">
        <h3>
          <span>📜</span> The Best-{count} Point System
        </h3>
        <p>
          The Community Leaderboard ranks players by cumulative performance across the circuit. To reward
          peak mastery while preventing burnout, rankings operate on a strict <strong>Best-{count} Rule</strong>:
        </p>
        <ul>
          <li>
            <strong>Top {count} Scores Count</strong>: A player&apos;s total is the sum of their {count} highest-scoring tournament finishes
            across all {facts.slots} active event slots.
          </li>
          <li>
            <strong>Participation Protected</strong>: If a competitor has played in {count} or fewer events, all scores sum normally.
            Any scores beyond the {count}th are dropped—ensuring players can freely compete without risking their placement.
          </li>
          <li>
            <strong>Visual Cues</strong>: In the table, counting scores are highlighted in{' '}
            <span className="score-counting">gold boxes</span>, while dropped scores are faded.
          </li>
        </ul>
      </div>
      <div className="explainer-card">
        <h3>
          <span>🏆</span> Tracked Event Slots &amp; Point Tiers
        </h3>
        <p>This edition covers {facts.slots} event slots in {facts.tiers} tiers:</p>
        <ul>
          {facts.lines.map((t) => (
            <li key={t.name}>
              <strong>
                {t.name} (<code>{t.codes}</code>)
              </strong>
              : {t.maxPts} pts max{t.each ? ' each' : ''}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function Controls({ view, slots, names }: { view: CommunityView; slots: number; names: SearchPlayer[] }) {
  return (
    <div className="comm-controls">
      <form method="get" action="/ratings/community/" className="comm-search-form">
        <input type="hidden" name="view" value={view.view} />
        <input type="hidden" name="edition" value={view.edition} />
        <SearchBox players={names} value={view.search} name="q" inputClassName="comm-search-input" placeholder="Search competitor by name..." />
        <button type="submit" className="btn-search-gold">
          Search
        </button>
        {view.search && (
          <a href={href(view, { search: '' })} className="btn-clear-muted">
            Clear
          </a>
        )}
      </form>
      <div
        className="cmb-hint-box"
        title="Hold down your Center Mouse Button / Scroll-wheel and drag horizontally, or hold Shift while scrolling"
      >
        <span>🖱️</span> Scroll table with <strong>CMB</strong>
      </div>
      <Segmented
        label="View"
        look="view"
        options={[
          { value: 'standard', label: '📊 Standard View (Tournament Summary)', title: 'Standard View: Shows the sum of each tournament for optimal visibility' },
          { value: 'expanded', label: '📑 Expanded View (All Periods)', title: `Expanded View: Expands to the sides showing all ${slots} individual periods` },
        ]}
        value={view.view}
        link={(v) => ({ href: href(view, { view: v }) })}
      />
    </div>
  );
}

function Table({ data, view, rows }: { data: LeaderboardData; view: CommunityView; rows: CommunityRow[] }) {
  const columns = data.categories.flatMap((c) => c.columns);
  const sheetOrder = columns.map((c) => c.id);
  const standard = view.view === 'standard';
  const count = data.counting_events ?? 8;
  const wcName = data.info.wc;

  return (
    <div className={`table-wrapper${standard ? '' : ' expanded-view'}`}>
      <table className="comm-table">
        <thead>
          <tr className="super-header">
            <th colSpan={5} className="comm-super-left">
              Leaderboard Standings
            </th>
            {standard ? (
              <th colSpan={data.summary.length}>Tournament Categories (Sum of Events)</th>
            ) : (
              data.categories.map((c) => (
                <th key={c.category} colSpan={c.columns.length}>
                  {c.category}
                </th>
              ))
            )}
          </tr>
          <tr className="col-header">
            <th className="sticky-col-rank">#</th>
            <th className="sticky-col-player text-left">Competitor</th>
            <th>Title</th>
            <th>Total Pts</th>
            <th>Tourneys</th>
            {standard
              ? data.summary.map((t) => (
                  <th key={t.id} title={`${t.name} (${t.desc})`}>
                    {t.short}
                  </th>
                ))
              : columns.map((c) => (
                  <th key={c.id} title={`${c.name} (Max: ${c.max_pts} pts)`}>
                    {c.short}
                  </th>
                ))}
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr>
              <td colSpan={25} className="comm-empty">
                No competitors found matching &quot;<strong>{view.search}</strong>&quot;.
              </td>
            </tr>
          ) : (
            rows.map((r) => {
              const counting = countingColumns(r.scores, sheetOrder, count);
              const sums = tournamentSums(r.scores, data.summary, counting);
              const mark = titleMark(r, wcName);
              return (
                <tr key={r.name}>
                  <td className="sticky-col-rank">
                    <Rank rank={r.rank} />
                  </td>
                  <td className="sticky-col-player text-left">
                    <Flag country={r.country} />{' '}
                    <PlayerLink name={r.name} className="player-link comm-player" />
                  </td>
                  <td>
                    {mark === null ? (
                      <span className="comm-dash">-</span>
                    ) : (
                      <span className={`badge badge-${mark.toLowerCase()}`} title={titleName(mark)}>
                        {mark}
                      </span>
                    )}
                  </td>
                  <td>
                    <span className="points-total">{whole(r.points)}</span>
                  </td>
                  <td className="comm-muted">{r.events}</td>
                  {standard
                    ? data.summary.map((t) => {
                        const s = sums[t.id];
                        const tip = s
                          ? `${t.name} Sum: ${whole(s.sum)} pts (${s.counting ? `contributes to Top-${count} total` : `dropped beyond Top-${count}`})`
                          : '';
                        return (
                          <td key={t.id}>
                            <Score value={s?.sum} counting={s?.counting ?? false} tip={tip} />
                          </td>
                        );
                      })
                    : columns.map((c) => {
                        const v = r.scores[c.id];
                        const on = counting.includes(c.id);
                        const tip =
                          v === undefined
                            ? ''
                            : on
                              ? `Counts toward Top-${count} total: ${whole(v)} pts in ${c.name}`
                              : `Dropped (beyond Top-${count}): ${whole(v)} pts in ${c.name}`;
                        return (
                          <td key={c.id}>
                            <Score value={v} counting={on} tip={tip} />
                          </td>
                        );
                      })}
                </tr>
              );
            })
          )}
        </tbody>
      </table>
    </div>
  );
}

/** CommunityPage renders the edition, view, search and page named in the address, from the current run. */
export default function CommunityPage({ apiUrl }: { apiUrl: string }) {
  const [view] = useState(() => parseCommunityView(new URLSearchParams(window.location.search)));
  const [state, setState] = useState<Load>({ kind: 'loading' });

  useEffect(() => {
    let stale = false;
    Promise.all([
      communityEditions(apiUrl),
      currentRun(apiUrl).then((run) => communityLeaderboard(apiUrl, run.run_id, view.edition)),
    ])
      .then(([editions, data]) => {
        if (!stale) {
          setState({ kind: 'ready', data, editions });
        }
      })
      .catch((e: unknown) => {
        if (!stale) {
          setState({ kind: 'error', message: e instanceof Error ? e.message : String(e) });
        }
      });
    return () => {
      stale = true;
    };
  }, [apiUrl, view.edition]);

  if (state.kind !== 'ready') return <Waiting load={state} what="the community leaderboard" />;

  const data = state.data;
  const found = view.search ? data.rows.filter((r) => matchesSearch(r.name, view.search)) : data.rows;
  const total = Math.max(1, Math.ceil(found.length / view.perPage));
  const page = Math.min(view.page, total);
  const shown = found.slice((page - 1) * view.perPage, page * view.perPage);
  const onPage = { ...view, page };

  return (
    <div className="community-page">
      <Hero data={data} editions={state.editions} view={onPage} />
      <Explainers data={data} />
      <Controls
        view={onPage}
        slots={editionFacts(data).slots}
        names={data.rows.map((r) => ({ name: r.name, country: r.country }))}
      />
      <div className="results-counter-card">
        <div>
          Showing <strong className="comm-count-shown">{shown.length}</strong> of{' '}
          <strong className="comm-count-total">{found.length}</strong> competitors
          {view.search && <span className="comm-count-filter"> (filtered by &quot;{view.search}&quot;)</span>}
        </div>
        <div>
          Mode:{' '}
          <strong className="comm-mode">
            {view.view === 'standard'
              ? `Tournament Sums (${editionFacts(data).tournaments} Tournaments)`
              : `All ${editionFacts(data).slots} Period Columns`}
          </strong>
        </div>
      </div>
      <div className="scroll-hint comm-scroll-hint">
        <span>Swipe table horizontally for full community ranking ➔</span>
      </div>
      <Table data={data} view={onPage} rows={shown} />
      {total > 1 && (
        <Pager
          page={page}
          pages={total}
          total={{ count: found.length, noun: 'competitors' }}
          link={(p) => ({ href: href(onPage, { page: p }) })}
          go={(p) => window.location.assign(href(onPage, { page: p }))}
        />
      )}
    </div>
  );
}
