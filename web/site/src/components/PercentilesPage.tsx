/** The rating percentiles page: how the leaderboard's players spread over the chosen rating, and where a chosen player stands. */
import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { BarController, BarElement, CategoryScale, Chart, LinearScale, Tooltip, type Plugin } from 'chart.js';
import { currentRun, leaderboard, ratingPercentiles, type RatingBasis, type RatingPercentiles, type TitleCode } from '@ttac/shared/ratings';
import Segmented from '@ttac/shared/Segmented';
import { count, counted, ordinal } from '@ttac/shared/numbers';
import * as show from '../lib/leaderboard-display';
import { DEFAULT_VIEW, ENGINES, modelOf, titlesInOrder, type Status } from '../lib/leaderboard-view';
import {
  BIN_WIDTHS,
  chartBins,
  leaderboardHref,
  markLabel,
  parsePercentileView,
  percentileParams,
  percentileQuery,
  ratingLabel,
  tierOf,
  tierSpans,
  topShare,
  type PercentileView,
  type TierSpan,
} from '../lib/percentiles';
import { profileHref } from '../lib/player-view';
import { recall } from '../lib/remembered';
import { browserStorage } from '@ttac/shared/storage';
import Message from './Message';
import MinOppsField from './MinOppsField';
import PanelHeader from './PanelHeader';
import PlayerPicker from './PlayerPicker';
import ScrollTable from './ScrollTable';
import TitleSelect from './TitleSelect';
import ViewSelectors from './ViewSelectors';
import Waiting from './Waiting';
import { useKeptFound, useLoad } from './useLoad';
import { useAddressView, type Address } from './useAddressView';
import './PercentilesPage.css';

Chart.register(BarController, BarElement, CategoryScale, LinearScale, Tooltip);

const ADDRESS: Address<PercentileView> = {
  parse: parsePercentileView,
  params: percentileParams,
  remembers: (v) => ({ ...DEFAULT_VIEW, ...recall(browserStorage()), engine: v.board.engine, reset: v.board.reset, format: v.board.format }),
};

const STATUS_OPTIONS: { value: Status; label: string; tooltip: string }[] = [
  { value: 'active', label: 'Active', tooltip: "Players with a game in the 12 months before the run's last game" },
  { value: 'all', label: 'All', tooltip: 'Every rated player' },
  { value: 'inactive', label: 'Inactive', tooltip: 'Players without a game in those 12 months' },
];

const RATING_OPTIONS: { value: RatingBasis; label: string; tooltip: string }[] = [
  { value: 'c_rating', label: 'Conservative', tooltip: 'C-Rating: rating minus three times its deviation, as the leaderboard ranks' },
  { value: 'rating', label: 'Plain', tooltip: 'The rating itself, however uncertain' },
];

const statusWord: Record<Status, string> = { active: 'active ', all: '', inactive: 'inactive ' };

/** PercentilesPage loads the spread for the view in the address, and the rated players' names once the search box is used. */
export default function PercentilesPage({ apiUrl }: { apiUrl: string }) {
  const { view, go, linkTo } = useAddressView(ADDRESS);
  const run = useLoad('run', () => currentRun(apiUrl).then((r) => r.run_id));
  const runId = run.kind === 'ready' ? run.data : null;
  const q = percentileQuery(view);
  const spread = useKeptFound(`${runId}|${percentileParams(view)}`, runId === null ? null : () => ratingPercentiles(apiUrl, runId, q));

  const [used, setUsed] = useState(false);
  const names = useLoad(
    `${used}|${runId}`,
    used && runId !== null ? () => leaderboard(apiUrl, runId, modelOf(DEFAULT_VIEW), DEFAULT_VIEW.format).then((b) => b.players.map((p) => p.name)) : null,
  );

  const board = view.board;
  const pick = (player: string) => go({ ...view, player });
  const held = spread.kind === 'ready' ? titlesInOrder(spread.data.titles) : [];
  const titles: TitleCode[] = board.title === null || held.includes(board.title) ? held : [...held, board.title];

  return (
    <>
      <ViewSelectors
        reset={board.reset}
        format={board.format}
        linkReset={(reset) => linkTo({ ...view, board: { ...board, reset } })}
        linkFormat={(format) => linkTo({ ...view, board: { ...board, format } })}
      >
        <div className="view-bar view-bar-row">
          <Segmented
            label="Engine"
            groupClass="view-group"
            prefix={<span className="view-bar-label">Engine:</span>}
            options={ENGINES.map((e) => ({ value: e, label: show.engineName(e) }))}
            value={board.engine}
            link={(engine) => linkTo({ ...view, board: { ...board, engine } })}
          />
          <Segmented
            label="Players counted"
            groupClass="view-group"
            prefix={<span className="view-bar-label">Players:</span>}
            options={STATUS_OPTIONS}
            value={board.status}
            link={(status) => linkTo({ ...view, board: { ...board, status } })}
          />
        </div>
        <div className="view-bar view-bar-row">
          <Segmented
            label="Rating counted"
            groupClass="view-group"
            prefix={<span className="view-bar-label">Rating:</span>}
            options={RATING_OPTIONS}
            value={view.rating}
            link={(rating) => linkTo({ ...view, rating })}
          />
          <Segmented
            label="Bin width"
            groupClass="view-group"
            prefix={<span className="view-bar-label">Bins:</span>}
            options={BIN_WIDTHS.map((w) => ({ value: w, label: String(w), tooltip: `${w}-point bins` }))}
            value={view.binWidth}
            link={(binWidth) => linkTo({ ...view, binWidth })}
          />
        </div>
        <div className="view-bar view-bar-row">
          <div className="view-group">
            <label className="view-bar-label" htmlFor="percentiles-title">
              Title:
            </label>
            <TitleSelect
              id="percentiles-title"
              titles={titles}
              value={board.title}
              onChange={(title) => go({ ...view, board: { ...board, title } })}
            />
          </div>
          <div className="view-group">
            <label className="view-bar-label" htmlFor="percentiles-min">
              Min opponents:
            </label>
            <MinOppsField id="percentiles-min" value={board.minOpps} onApply={(minOpps) => go({ ...view, board: { ...board, minOpps } })} />
          </div>
        </div>
      </ViewSelectors>

      <div className="percentiles-picker">
        <PlayerPicker players={names.kind === 'ready' ? names.data : []} value={view.player} onPick={pick} onFocus={() => setUsed(true)} />
      </div>

      {run.kind === 'error' ? (
        <Waiting load={run} what="the ratings" />
      ) : spread.kind === 'missing' ? (
        <Message>
          No rated player is named {view.player}. <a {...linkTo({ ...view, player: '' })}>Show everyone</a>
        </Message>
      ) : spread.kind !== 'ready' ? (
        <Waiting load={spread} what="the rating spread" />
      ) : (
        <Spread data={spread.data} view={view} />
      )}
    </>
  );
}

/** Spread shows the player's standing, the chart and the marked percentiles. */
function Spread({ data, view }: { data: RatingPercentiles; view: PercentileView }) {
  const who = `${statusWord[data.status]}${data.title === null ? '' : `${data.title} `}players with ${data.min_opps}+ opponents`;
  const p = data.player;
  const rating = ratingLabel(data.rating);
  return (
    <>
      {p !== undefined && (
        <div className="stats-grid-4">
          <div className="metric-card">
            <div className="metric-label">
              <a href={profileHref(p.name, view.board)}>{p.name}</a>
            </div>
            <div className="metric-number metric-matches">{ordinal(Math.floor(p.percentile))}</div>
            <div className="metric-note">percentile: rated above {p.percentile.toFixed(1)}% of {who}</div>
          </div>
          <div className="metric-card">
            <div className="metric-label">Standing</div>
            <div className="metric-number">{topShare(p, data.players)}</div>
            <div className="metric-note">
              #{count(p.rank)} of {count(data.players + (p.counted ? 0 : 1))}
              {p.counted ? '' : ` (${p.name} is outside these filters)`}
            </div>
          </div>
          <div className="metric-card">
            <div className="metric-label">{rating}</div>
            <div className="metric-number">{p.value.toFixed(1)}</div>
            <div className="metric-note">
              {show.engineName(percentileQuery(view).model)}, {show.formatName(data.format)}
            </div>
          </div>
        </div>
      )}

      <div className="rating-card panel-card percentiles-chart-card">
        <PanelHeader
          title={data.rating === 'rating' ? 'Rating Distribution' : 'Conservative Rating Distribution'}
          subtitle={
            <>
              {counted(data.players, 'player')} ({who}) in {data.bin_width}-point bins of {rating}
              {data.rating === 'c_rating' ? (
                <>
                  , the rating the <a href={leaderboardHref(view)}>leaderboard</a> ranks by
                </>
              ) : (
                <>
                  ; the <a href={leaderboardHref(view)}>leaderboard</a> ranks by C-Rating
                </>
              )}
            </>
          }
        />
        {data.players === 0 ? <Message form="inline">No players match these filters.</Message> : <SpreadChart data={data} />}
      </div>

      {data.marks.length > 0 && (
        <div className="rating-card panel-card">
          <PanelHeader title="Percentile Marks" subtitle={`The ${rating} a share of the players are rated at or below`} />
          <ScrollTable hint="all columns">
            <table className="leaderboard-table">
              <caption className="visually-hidden">{rating} at each marked percentile</caption>
              <thead>
                <tr>
                  <th scope="col">Percentile</th>
                  <th scope="col">{rating}</th>
                  <th scope="col">Players at or above</th>
                </tr>
              </thead>
              <tbody>
                {data.marks.map((m) => (
                  <tr key={m.percentile}>
                    <th scope="row">{markLabel(m.percentile)}</th>
                    <td>{m.value.toFixed(1)}</td>
                    <td>Top {100 - m.percentile}%</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </ScrollTable>
        </div>
      )}
    </>
  );
}

/** tierBands shades each rating tier behind the bars, as the player's rating chart does, with its name at the top. */
function tierBands(spans: TierSpan[], line: string, text: string, font: string): Plugin<'bar'> {
  return {
    id: 'percentileTiers',
    beforeDatasetsDraw({ ctx, chartArea, scales }) {
      const x = scales.x;
      if (x === undefined) return;
      ctx.save();
      ctx.font = `11px ${font}`;
      ctx.textBaseline = 'top';
      for (const [i, s] of spans.entries()) {
        const left = x.getPixelForValue(s.start);
        const right = x.getPixelForValue(s.end);
        ctx.fillStyle = s.tier.color;
        ctx.fillRect(left, chartArea.top, right - left, chartArea.height);
        if (i > 0) {
          ctx.strokeStyle = s.tier.borderColor ?? line;
          ctx.setLineDash([4, 4]);
          ctx.beginPath();
          ctx.moveTo(left, chartArea.top);
          ctx.lineTo(left, chartArea.bottom);
          ctx.stroke();
        }
        const name = ctx.measureText(s.tier.name).width + 8 <= right - left ? s.tier.name : s.tier.code;
        if (ctx.measureText(name).width + 8 <= right - left) {
          ctx.fillStyle = text;
          ctx.fillText(name, left + 4, chartArea.top + 4);
        }
      }
      ctx.restore();
    },
  };
}

const useIsomorphicLayoutEffect = typeof window !== 'undefined' ? useLayoutEffect : useEffect;

/** SpreadChart draws the bins as bars, the player's bin in the win colour, with the share below each bin on hover. */
function SpreadChart({ data }: { data: RatingPercentiles }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const bins = chartBins(data);
  const rating = ratingLabel(data.rating);
  const spans = tierSpans(data);

  useIsomorphicLayoutEffect(() => {
    if (canvas.current === null) return;
    const token = (name: string) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    const gold = token('--accent-gold');
    const grid = { color: token('--border-faint') };
    const ticks = { color: token('--text-soft'), font: { family: token('--font-stats') } };
    const title = (text: string) => ({ display: true, text, color: gold, font: { family: token('--font-heading') } });
    const highlight = (i: number) => i === bins.playerBin;
    const chart = new Chart(canvas.current, {
      type: 'bar',
      plugins: [tierBands(spans, token('--border-faint'), token('--text-soft'), token('--font-heading'))],
      data: {
        labels: bins.labels,
        datasets: [
          {
            label: 'Players',
            data: bins.counts,
            backgroundColor: bins.counts.map((_, i) => (highlight(i) ? token('--win') : token('--gold-wash-strong'))),
            borderColor: bins.counts.map((_, i) => (highlight(i) ? token('--win-edge') : gold)),
            borderWidth: 1,
            categoryPercentage: 1,
            barPercentage: 0.95,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        scales: { x: { title: title(rating), grid, ticks }, y: { title: title('Players'), grid, ticks, beginAtZero: true } },
        plugins: {
          tooltip: {
            callbacks: {
              title: (items) => {
                const i = items[0]?.dataIndex ?? -1;
                const bin = data.bins[i];
                return bin === undefined ? '' : `${rating} ${bins.labels[i] ?? ''} · ${tierOf(bin.from).name}`;
              },
              label: (ctx) => {
                const n = bins.counts[ctx.dataIndex] ?? 0;
                const below = bins.below[ctx.dataIndex] ?? 0;
                return [` ${counted(n, 'player')} (${((100 * n) / data.players).toFixed(1)}%)`, ` Rated above ${below.toFixed(1)}% of players`];
              },
              footer: (items) => (items[0]?.dataIndex === bins.playerBin && data.player ? `${data.player.name} is here` : ''),
            },
          },
        },
      },
    });
    return () => chart.destroy();
  }, [data]);

  const summary = bins.labels.map((l, i) => `${l}: ${bins.counts[i]}`).join(', ');
  const tiers = spans.map((s) => s.tier.name).join(', ');
  return (
    <div className="percentiles-chart">
      <canvas ref={canvas} aria-label={`Players in each ${data.bin_width}-point ${rating} bin, shaded by rating tier (${tiers}). ${summary}.`} />
    </div>
  );
}
