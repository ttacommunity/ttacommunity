/** The community leaderboard page: standings, dynamic best-N highlights, and tournament or event columns. */
import { useState } from 'react';
import {
  communityEditions,
  communityLeaderboard,
  currentRun,
  type CommunityEditionItem,
  type CommunityLeaderboard as LeaderboardData,
  type CommunityRow,
} from '@ttac/shared/ratings';
import { Flag, TitleBadge } from './PlayerMarks';
import {
  columnTitle,
  countingColumns,
  editionFacts,
  editionLabel,
  editionToShow,
  matchesSearch,
  parseCommunityView,
  titleMark,
  tournamentSums,
  whole,
  type CommunityView,
} from '../lib/community';
import './CommunityPage.css';
import Segmented from '@ttac/shared/Segmented';
import SearchBox, { type SearchPlayer } from './SearchBox';
import MetricCard, { MetricGrid } from './MetricCard';
import Panel from './Panel';
import PlayerLink from './PlayerLink';
import Waiting from './Waiting';
import Pager from './Pager';
import { count } from '@ttac/shared/numbers';
import { useLoad } from './useLoad';
import PageHeader from '@ttac/shared/PageHeader';
import { isTopThree, rank } from '../lib/leaderboard-display';


/** href is the page's address for a view, carrying edition, view, search and page. */
function href(view: CommunityView, change: Partial<CommunityView>): string {
  const v = { ...view, ...change };
  const q = new URLSearchParams({ edition: v.edition, view: v.view });
  if (v.search) q.set('q', v.search);
  if (change.page !== undefined) q.set('page', String(v.page));
  return `/ratings/community/?${q}`;
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
    <>
      <PageHeader title="Community Standard Leaderboard" subtitle={info.title || info.name} />
      <div className="comm-hero">
        <div className="maintainer-card">
          <div className="tag">{info.badge || '🛡️ Circuit Maintainer'}</div>
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
          <Segmented
            label="Leaderboard edition"
            options={editions.map((e) => ({ value: e.slug, label: editionLabel(e), title: e.title }))}
            value={view.edition}
            link={(edition) => ({ href: href(view, { edition }) })}
          />
        </div>
        <MetricGrid>
          <MetricCard label="Ranked Competitors" value={count(data.rows.length)} tone="gold" />
          <MetricCard label={`Tracked Events (${data.categories.length} Tiers)`} value={events} tone="gold" />
          <MetricCard label={`#1 Rank (${whole(top?.points ?? 0)} pts)`} value={<PlayerLink name={top?.name ?? '—'} className="comm-top-link" />} />
          <MetricCard
            label={info.wc_desc || 'Reigning World Champion'}
            value={
              info.wc ? (
                <PlayerLink name={info.wc} className="comm-wc-link">
                  <TitleBadge title={{ code: 'WC' }} className="comm-wc-badge" /> {info.wc}
                </PlayerLink>
              ) : (
                <span className="comm-muted">—</span>
              )
            }
          />
        </MetricGrid>
      </div>
    </>
  );
}

function Explainers({ data }: { data: LeaderboardData }) {
  const facts = editionFacts(data);
  const count = facts.counting;
  return (
    <div className="explainer-cards">
      <Panel id="best-rule" icon="📜" title={`The Best-${count} Point System`} stripe="gold" className="explainer-card">
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
      </Panel>
      <Panel id="event-slots" icon="🏆" title="Tracked Event Slots & Point Tiers" stripe="gold" className="explainer-card">
        <p>This edition covers {facts.slots} event slots in {facts.tiers} tiers:</p>
        <ul>
          {facts.lines.map((t) => (
            <li key={t.name}>
              <strong>
                {t.name} (<code>{t.codes}</code>)
              </strong>
              {t.maxPts !== null && `: ${t.maxPts} pts max${t.each ? ' each' : ''}`}
            </li>
          ))}
        </ul>
      </Panel>
    </div>
  );
}

function Controls({ view, slots, names }: { view: CommunityView; slots: number; names: SearchPlayer[] }) {
  return (
    <div className="view-bar comm-controls">
      <form method="get" action="/ratings/community/" className="comm-search-form">
        <input type="hidden" name="view" value={view.view} />
        <input type="hidden" name="edition" value={view.edition} />
        <SearchBox players={names} value={view.search} name="q" inputClassName="input-antique comm-search-input" placeholder="Search competitor by name..." wide />
        {view.search && (
          <a href={href(view, { search: '' })} className="btn-quiet">
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
  const [scrolled, setScrolled] = useState(false);

  return (
    <Panel label="Community leaderboard" className={standard ? 'table-wrapper' : 'table-wrapper expanded-view'}>
      <div
        className={scrolled ? 'table-responsive scrolled-x' : 'table-responsive'}
        onScroll={(e) => setScrolled(e.currentTarget.scrollLeft > 0)}
      >
        <table className="leaderboard-table comm-table">
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
              <th className="col-pinned comm-pinned-rank">#</th>
              <th className="col-pinned comm-pinned-name text-left">Competitor</th>
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
                    <th key={c.id} title={columnTitle(c)}>
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
                    <td className={isTopThree(r.rank) ? 'col-pinned comm-pinned-rank rank-col top-3-rank' : 'col-pinned comm-pinned-rank rank-col'}>{rank(r.rank)}</td>
                    <td className="col-pinned comm-pinned-name text-left">
                      <Flag country={r.country} />{' '}
                      <PlayerLink name={r.name} className="player-link comm-player" />
                    </td>
                    <td>
                      {mark === null ? <span className="comm-dash">-</span> : <TitleBadge title={{ code: mark }} />}
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
    </Panel>
  );
}

/** CommunityPage renders the edition, view, search and page named in the address, from the current run. */
export default function CommunityPage({ apiUrl }: { apiUrl: string }) {
  const [view] = useState(() => parseCommunityView(new URLSearchParams(window.location.search)));
  const state = useLoad(String(view.edition), async () => {
    const [editions, run] = await Promise.all([communityEditions(apiUrl), currentRun(apiUrl)]);
    const edition = editionToShow(view.edition, editions);
    if (edition === null) throw new Error('no community edition is shown');
    return { board: await communityLeaderboard(apiUrl, run.run_id, edition), editions };
  });

  if (state.kind !== 'ready') return <Waiting load={state} what="the community leaderboard" />;

  const data = state.data.board;
  const found = view.search ? data.rows.filter((r) => matchesSearch(r.name, view.search)) : data.rows;
  const total = Math.max(1, Math.ceil(found.length / view.perPage));
  const page = Math.min(view.page, total);
  const shown = found.slice((page - 1) * view.perPage, page * view.perPage);
  const onPage = { ...view, page, edition: data.edition };

  return (
    <div className="community-page">
      <Hero data={data} editions={state.data.editions} view={onPage} />
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
