/** Upstream's activity and demographics page: players, games per format, titles and the leading countries. */
import { useEffect, useMemo, useState } from 'react';
import { activity, currentRun, type Activity } from '@ttac/shared/ratings';
import { formatCards, sortCountries, titleLabel, type CountryColumn } from '../lib/analysis';
import { runYears } from '../lib/player-view';
import { Flag } from './PlayerMarks';
import ScrollTable from './ScrollTable';
import Waiting from './Waiting';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: Activity; years: [number, number] | null };

const COLUMNS: { col: CountryColumn; label: string; tip?: string }[] = [
  { col: 'rank', label: 'Rank' },
  { col: 'country', label: 'Country' },
  { col: 'players', label: 'Total Players' },
  { col: 'gm', label: 'Avg (GM)', tip: 'Average rating of Grandmaster (GM) players in this country' },
  { col: 'm_plus', label: 'Avg (M+)', tip: 'Average rating of Master or higher (GM + M) players in this country' },
  { col: 'p_plus', label: 'Avg (Platinum+)', tip: 'Average rating of Platinum or higher (GM + M + P) players in this country' },
  { col: 'all', label: 'Avg (All)', tip: 'Average rating across all registered players in this country' },
];

const count = (n: number) => n.toLocaleString('en-US');
const avg = (x: number | null) => (x === null ? '-' : x.toFixed(1));

/** ActivityPage loads the current run's activity and shows it as upstream's page does. */
export default function ActivityPage({ apiUrl }: { apiUrl: string }) {
  const [load, setLoad] = useState<Load>({ kind: 'loading' });
  const [sort, setSort] = useState<{ col: CountryColumn; asc: boolean } | null>(null);
  const [directions, setDirections] = useState<Partial<Record<CountryColumn, boolean>>>({});

  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => activity(apiUrl, r.run_id).then((data) => setLoad({ kind: 'ready', data, years: runYears(r.first_game, r.last_game) })))
      .catch((e: unknown) => setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
  }, [apiUrl]);

  const ranked = useMemo(() => (load.kind === 'ready' ? load.data.countries.map((c, i) => ({ ...c, rank: i + 1 })) : []), [load]);
  const countries = sort === null ? ranked : sortCountries(ranked, sort.col, sort.asc);

  if (load.kind !== 'ready') return <Waiting load={load} what="the activity figures" />;

  const a = load.data;
  const total = a.active + a.inactive;
  const pct = (n: number) => (total > 0 ? ((n / total) * 100).toFixed(1) : '0.0');
  const formats = formatCards(a.formats);
  // Each header flips its own direction, starting ascending, as upstream's script does.
  const sortBy = (col: CountryColumn) => {
    const asc = !directions[col];
    setDirections({ ...directions, [col]: asc });
    setSort({ col, asc });
  };

  return (
    <>
      <div className="stats-grid-4">
        <div className="metric-card">
          <div className="metric-label">Active Players (12m)</div>
          <div className="metric-number metric-active">{count(a.active)}</div>
          <div className="metric-note">{pct(a.active)}% active retention</div>
        </div>
        <div className="metric-card">
          <div className="metric-label">Inactive / Retired</div>
          <div className="metric-number metric-inactive">{count(a.inactive)}</div>
          <div className="metric-note">{pct(a.inactive)}% inactive (&gt;12m)</div>
        </div>
        <div className="metric-card">
          <div className="metric-label">Total Matches Recorded</div>
          <div className="metric-number metric-matches">{count(formats.total)}</div>
          <div className="metric-note">{load.years ? `Across ${load.years[0]}–${load.years[1]} seasons` : 'Across every season'}</div>
        </div>
        <div className="metric-card">
          <div className="metric-label">All Recorded Competitors</div>
          <div className="metric-number metric-players">{count(total)}</div>
          <div className="metric-note">Unique tournament players</div>
        </div>
      </div>

      <div className="activity-columns">
        <div className="rating-card activity-card">
          <h3>Matches by Game Format</h3>
          <p className="activity-sub">Distribution of competitive tournaments across player counts.</p>
          <div className="activity-list">
            {formats.cards.map((f) => (
              <div key={f.format} className="activity-item">
                <div>
                  <strong>{f.name}</strong>
                  <div className="activity-item-note">{f.note}</div>
                </div>
                <div className="activity-item-value">{count(f.games)}</div>
              </div>
            ))}
          </div>
        </div>

        <div className="rating-card activity-card">
          <h3>Official Tournament Titles</h3>
          <p className="activity-sub">Awarded for top finishes in official World Championships &amp; continental championships.</p>
          <div className="activity-list">
            {a.titles.map((t) => (
              <div key={t.code} className="activity-item">
                <div className="activity-title">
                  <span className={`badge badge-${t.code.toLowerCase()} activity-badge`}>{t.code}</span>
                  <strong>{titleLabel(t.code)}</strong>
                </div>
                <div className="activity-item-value activity-title-count">{t.players} players</div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="rating-card activity-card activity-countries">
        <h3>Top 15 Countries by Player Participation &amp; Title Rating Depth</h3>
        <p className="activity-sub">
          Comparative average Glicko-2 ratings by official title status across the top 15 participating nations. Click headers to sort.
        </p>
        <ScrollTable hint="country demographics">
          <table className="activity-table">
            <caption className="visually-hidden">The 15 countries with most rated players, and their average Glicko-2 Standard rating by title</caption>
            <thead>
              <tr>
                {COLUMNS.map((c) => (
                  <th
                    key={c.col}
                    scope="col"
                    className={c.col === 'country' ? 'col-country' : undefined}
                    aria-sort={sort?.col === c.col ? (sort.asc ? 'ascending' : 'descending') : undefined}
                  >
                    <button type="button" className="sort-header" data-tooltip={c.tip} onClick={() => sortBy(c.col)}>
                      {c.label} ⇅
                    </button>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {countries.map((c) => (
                <tr key={c.country}>
                  <td className="country-rank">#{c.rank}</td>
                  <td className="col-country">
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
      </div>
    </>
  );
}
