/** The community leaderboard page, as upstream's: an edition's standings, best-8 highlights, and tournament or event columns. */
import { useEffect, useState } from 'react';
import { communityLeaderboard, currentRun, type CommunityEdition, type CommunityRow } from '@ttac/shared/ratings';
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
import { EDITIONS, type Edition } from '../lib/community-editions';
import { titleName } from '../lib/leaderboard-display';
import './CommunityPage.css';
import Segmented from '@ttac/shared/Segmented';
import SearchBox, { type SearchPlayer } from './SearchBox';
import PlayerLink from './PlayerLink';
import Waiting from './Waiting';
import Pager from './Pager';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; rows: CommunityRow[] };

const PILLS: { edition: CommunityEdition; text: string; tip: string }[] = [
  { edition: '2026', text: '⭐ 2026 (Rolling Q2)', tip: 'Active rolling 4-season championship circuit' },
  { edition: '2025', text: '🏆 2025 Final Archive', tip: 'Official final standings for Season 2025' },
  { edition: '2024', text: '📜 2024 Final Archive', tip: 'Official final standings for Season 2024' },
  { edition: 'race26', text: '🏁 2026 Race Tracker', tip: 'Live seasonal points race for calendar year 2026' },
];

/** href is the page's address for a view, as upstream's links carry edition, view, search and page. */
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

function Hero({ edition, view, rows }: { edition: Edition; view: CommunityView; rows: CommunityRow[] }) {
  const top = rows[0];
  const events = edition.categories.reduce((n, c) => n + c.columns.length, 0);
  return (
    <div className="comm-hero">
      <h1 className="comm-title">Community Standard Leaderboard</h1>
      <div className="comm-subtitle">{edition.info.title}</div>
      <div className="maintainer-card">
        <div className="maintainer-badge">🛡️ Circuit Maintainer</div>
        <div className="maintainer-desc">
          This official leaderboard is curated and maintained by{' '}
          <strong>
            <PlayerLink name="wolvs">Wolvs</PlayerLink>
          </strong>{' '}
          • Tracking tournament finishes under the official Best-8 system.
        </div>
      </div>
      <div className="edition-selector-bar">
        <span className="edition-label">📅 Leaderboard Edition:</span>
        <div className="edition-pills">
          {PILLS.map((p) => (
            <a
              key={p.edition}
              href={href(view, { edition: p.edition })}
              className={`edition-pill${view.edition === p.edition ? ' active' : ''}`}
              title={p.tip}
            >
              {p.text}
            </a>
          ))}
        </div>
      </div>
      <div className="comm-stats-grid">
        <div className="stat-tile">
          <div className="stat-tile-num">{rows.length.toLocaleString('en-US')}</div>
          <div className="stat-tile-label">Ranked Competitors</div>
        </div>
        <div className="stat-tile">
          <div className="stat-tile-num">{events}</div>
          <div className="stat-tile-label">Tracked Events ({editionFacts(edition).tiers} Tiers)</div>
        </div>
        <div className="stat-tile">
          <div className="stat-tile-num">
            <PlayerLink name={top?.name ?? 'wolvs'} className="comm-top-link" />
          </div>
          <div className="stat-tile-label">#1 Rank ({whole(top?.points ?? 6320)} pts)</div>
        </div>
        <div className="stat-tile">
          <div className="stat-tile-num">
            <PlayerLink name={edition.info.wc} className="comm-wc-link">
              <span className="badge badge-wc comm-wc-badge">WC</span> {edition.info.wc}
            </PlayerLink>
          </div>
          <div className="stat-tile-label">Reigning World Champion</div>
        </div>
      </div>
    </div>
  );
}

function Explainers({ edition }: { edition: Edition }) {
  const facts = editionFacts(edition);
  return (
    <div className="explainer-cards">
      <div className="explainer-card">
        <h3>
          <span>📜</span> The Best-8 Rolling Point System
        </h3>
        <p>
          The Community Standard Leaderboard ranks players by cumulative performance across the global circuit. To reward
          peak mastery while preventing burnout, rankings operate on a strict <strong>Best-8 Rolling Rule</strong>:
        </p>
        <ul>
          <li>
            <strong>Top 8 Scores Count</strong>: A player&apos;s total is the sum of their 8 highest-scoring tournament finishes
            across all {facts.slots} active event slots.
          </li>
          <li>
            <strong>Participation Protected</strong>: If a competitor has played in 8 or fewer events, all scores sum normally.
            Any scores beyond the 8th are dropped—ensuring players can freely compete without risking their placement.
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

function Table({ edition, view, rows }: { edition: Edition; view: CommunityView; rows: CommunityRow[] }) {
  const columns = edition.categories.flatMap((c) => c.columns);
  const standard = view.view === 'standard';
  return (
    <div className={`table-wrapper${standard ? '' : ' expanded-view'}`}>
      <table className="comm-table">
        <thead>
          <tr className="super-header">
            <th colSpan={5} className="comm-super-left">
              Leaderboard Standings
            </th>
            {standard ? (
              <th colSpan={edition.summary.length}>Tournament Categories (Sum of Events)</th>
            ) : (
              edition.categories.map((c) => (
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
              ? edition.summary.map((t) => (
                  <th key={t.id} title={`${t.name} (${t.desc})`}>
                    {t.short}
                  </th>
                ))
              : columns.map((c) => (
                  <th key={c.id} title={`${c.name} (Max: ${c.maxPts} pts)`}>
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
              const counting = countingColumns(r.scores, edition.sheetOrder);
              const sums = tournamentSums(r.scores, edition.summary, counting);
              const mark = titleMark(r, edition);
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
                    ? edition.summary.map((t) => {
                        const s = sums[t.id];
                        const tip = s
                          ? `${t.name} Sum: ${whole(s.sum)} pts (${s.counting ? 'contributes to Top-8 total' : 'dropped beyond Top-8'})`
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
                              ? `Counts toward Top-8 total: ${whole(v)} pts in ${c.name}`
                              : `Dropped (beyond Top-8): ${whole(v)} pts in ${c.name}`;
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
    currentRun(apiUrl)
      .then((run) => communityLeaderboard(apiUrl, run.run_id, view.edition))
      .then((got) => !stale && setState({ kind: 'ready', rows: got.rows }))
      .catch((e: unknown) => !stale && setState({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
    return () => {
      stale = true;
    };
  }, [apiUrl, view.edition]);

  const edition = EDITIONS[view.edition];
  if (state.kind !== 'ready') return <Waiting load={state} what="the community leaderboard" />;

  const found = view.search ? state.rows.filter((r) => matchesSearch(r.name, view.search)) : state.rows;
  const total = Math.max(1, Math.ceil(found.length / view.perPage));
  const page = Math.min(view.page, total);
  const shown = found.slice((page - 1) * view.perPage, page * view.perPage);
  const onPage = { ...view, page };

  return (
    <div className="community-page">
      <Hero edition={edition} view={onPage} rows={state.rows} />
      <Explainers edition={edition} />
      <Controls view={onPage} slots={editionFacts(edition).slots} names={state.rows.map((r) => ({ name: r.name, country: r.country }))} />
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
              ? `Tournament Sums (${editionFacts(edition).tournaments} Tournaments)`
              : `All ${editionFacts(edition).slots} Period Columns`}
          </strong>
        </div>
      </div>
      <div className="scroll-hint comm-scroll-hint">
        <span>Swipe table horizontally for full community ranking ➔</span>
      </div>
      <Table edition={edition} view={onPage} rows={shown} />
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
