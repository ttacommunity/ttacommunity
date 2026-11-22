/** Upstream's activity and demographics page: players, games per format, titles and the leading countries. */
import { useMemo } from 'react';
import { activity, currentRun } from '@ttac/shared/ratings';
import SortHeader, { useSort } from '@ttac/shared/SortHeader';
import { formatCards, COUNTRY_COLUMNS, titleLabel, type CountryColumn } from '../lib/analysis';
import { runYears } from '../lib/player-view';
import { Flag, TitleBadge } from './PlayerMarks';
import Panel from './Panel';
import Panels from './Panels';
import Inset from './Inset';
import MetricCard, { MetricGrid } from './MetricCard';
import ScrollTable from './ScrollTable';
import Waiting from './Waiting';
import { count } from '@ttac/shared/numbers';
import { useLoad } from './useLoad';


const COLUMNS: { col: CountryColumn; label: string; tip?: string }[] = [
  { col: 'rank', label: 'Rank' },
  { col: 'country', label: 'Country' },
  { col: 'players', label: 'Total Players' },
  { col: 'gm', label: 'Avg (GM)', tip: 'Average rating of Grandmaster (GM) players in this country' },
  { col: 'm_plus', label: 'Avg (M+)', tip: 'Average rating of Master or higher (GM + M) players in this country' },
  { col: 'p_plus', label: 'Avg (Platinum+)', tip: 'Average rating of Platinum or higher (GM + M + P) players in this country' },
  { col: 'all', label: 'Avg (All)', tip: 'Average rating across all registered players in this country' },
];

const avg = (x: number | null) => (x === null ? '-' : x.toFixed(1));

/** ActivityPage loads the current run's activity and shows it as upstream's page does. */
export default function ActivityPage({ apiUrl }: { apiUrl: string }) {
  const load = useLoad('activity', () => currentRun(apiUrl).then((r) => activity(apiUrl, r.run_id).then((a) => ({ activity: a, years: runYears(r.first_game, r.last_game) }))));
  const ranked = useMemo(() => (load.kind === 'ready' ? load.data.activity.countries.map((c, i) => ({ ...c, rank: i + 1 })) : []), [load]);
  const [countries, sort, sortBy] = useSort(ranked, COUNTRY_COLUMNS);

  if (load.kind !== 'ready') return <Waiting load={load} what="the activity figures" />;

  const a = load.data.activity;
  const years = load.data.years;
  const total = a.active + a.inactive;
  const pct = (n: number) => (total > 0 ? ((n / total) * 100).toFixed(1) : '0.0');
  const formats = formatCards(a.formats);

  return (
    <Panels>
      <MetricGrid>
        <MetricCard label="Active Players (12m)" value={count(a.active)} note={<>{pct(a.active)}% active retention</>} tone="win" />
        <MetricCard label="Inactive / Retired" value={count(a.inactive)} note={<>{pct(a.inactive)}% inactive (&gt;12m)</>} tone="muted" />
        <MetricCard label="Total Matches Recorded" value={count(formats.total)} note={years ? `Across ${years[0]}–${years[1]} seasons` : 'Across every season'} tone="gold" />
        <MetricCard label="All Recorded Competitors" value={count(total)} note="Unique tournament players" tone="sky" />
      </MetricGrid>

      <div className="activity-columns">
        <Panel id="activity-formats" title="Matches by Game Format" subtitle="Distribution of competitive tournaments across player counts.">
          <div className="activity-list">
            {formats.cards.map((f) => (
              <Inset key={f.format} form="row">
                <div className="activity-name">
                  <strong>{f.name}</strong>
                  <div className="activity-item-note">{f.note}</div>
                </div>
                <div className="activity-item-value">{count(f.games)}</div>
              </Inset>
            ))}
          </div>
        </Panel>

        <Panel id="activity-titles" title="Official Tournament Titles" subtitle="Awarded for top finishes in official World Championships & continental championships.">
          <div className="activity-list">
            {a.titles.map((t) => (
              <Inset key={t.code} form="row">
                <div className="activity-title">
                  <TitleBadge title={{ code: t.code }} />
                  <strong>{titleLabel(t.code)}</strong>
                </div>
                <div className="activity-item-value activity-title-count">{t.players} players</div>
              </Inset>
            ))}
          </div>
        </Panel>
      </div>

      <Panel
        id="activity-countries"
        title="Top 15 Countries by Player Participation & Title Rating Depth"
        subtitle="Comparative average Glicko-2 ratings by official title status across the top 15 participating nations. Click headers to sort."
      >
        <ScrollTable label="Countries">
          <table className="leaderboard-table activity-table">
            <caption className="visually-hidden">The 15 countries with most rated players, and their average Glicko-2 Standard rating by title</caption>
            <thead>
              <tr>
                {COLUMNS.map((c) => (
                  <SortHeader key={c.col} col={c.col} sort={sort} onSort={sortBy} align={c.col === 'country' ? 'start' : undefined}>
                    <span data-tooltip={c.tip}>{c.label}</span>
                  </SortHeader>
                ))}
              </tr>
            </thead>
            <tbody>
              {countries.map((c) => (
                <tr key={c.country}>
                  <td className="country-rank">#{c.rank}</td>
                  <td className="col-start activity-country">
                    <span className="country-flag">
                      <Flag country={c.country} />
                    </span>
                  </td>
                  <td className="country-players">{c.players}</td>
                  <td className="avg-gm">{avg(c.gm)}</td>
                  <td className="avg-m">{avg(c.m_plus)}</td>
                  <td className="avg-p">{avg(c.p_plus)}</td>
                  <td className="avg-all">{avg(c.all)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </ScrollTable>
      </Panel>
    </Panels>
  );
}
