/** The rating percentiles page: how the leaderboard's players spread over the chosen rating, and where a chosen player stands. */
import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { BarController, BarElement, CategoryScale, Chart, LinearScale, Tooltip, type Plugin } from 'chart.js';
import { currentRun, leaderboard, ratingPercentiles, type RatingPercentiles, type TitleCode } from '@ttac/shared/ratings';
import { count, counted, ordinal } from '@ttac/shared/numbers';
import * as show from '../lib/leaderboard-display';
import { DEFAULT_VIEW, modelOf, titlesInOrder, type Status } from '../lib/leaderboard-view';
import {
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
} from '../lib/percentiles';
import { drawTierBands, type TierLook, type TierSpan } from '../lib/tier-bands';
import { profileHref } from '../lib/player-view';
import { chartTheme, documentTokens } from '../lib/chart-theme';
import { recall } from '../lib/remembered';
import { browserStorage } from '@ttac/shared/storage';
import Message from './Message';
import MeanBadge, { MeanBadges } from './MeanBadge';
import MetricCard, { MetricGrid } from './MetricCard';
import { OptionsToggle } from './SideDrawer';
import Panel from './Panel';
import Panels from './Panels';
import PercentilesOptionsDrawer from './PercentilesOptionsDrawer';
import PlayerPicker from './PlayerPicker';
import ViewBar, { FormatPick } from './ViewBar';
import ScrollTable from './ScrollTable';
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

const statusWord: Record<Status, string> = { active: 'active ', all: '', inactive: 'inactive ' };

/**
 * PercentilesPage loads the spread for the view in the address, and the rated players' names once the search box is
 * used. Only once a player is entered does it highlight their percentile bin and standing.
 */
export default function PercentilesPage({ apiUrl }: { apiUrl: string }) {
  const { view, go, linkTo } = useAddressView(ADDRESS);
  const [optionsOpen, setOptionsOpen] = useState(false);
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
  const activeOptions =
    board.status !== DEFAULT_VIEW.status ||
    board.title !== DEFAULT_VIEW.title ||
    board.minOpps !== DEFAULT_VIEW.minOpps ||
    view.rating !== 'c_rating' ||
    view.binWidth !== 50;

  return (
    <>
      <ViewBar
        actions={
          <>
            <PlayerPicker players={names.kind === 'ready' ? names.data : []} value={view.player} onPick={pick} onFocus={() => setUsed(true)} inline />
            <OptionsToggle title="Engine, population, and binning options" active={activeOptions} open={optionsOpen} onClick={() => setOptionsOpen(!optionsOpen)} />
          </>
        }
      >
        <FormatPick value={view.board.format} link={(format) => linkTo({ ...view, board: { ...view.board, format } })} what="the rating spread" />
      </ViewBar>
      {optionsOpen && <PercentilesOptionsDrawer view={view} titles={titles} linkTo={linkTo} go={go} onClose={() => setOptionsOpen(false)} />}

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
        <MetricGrid>
          <MetricCard
            label={<a href={profileHref(p.name, view.board)}>{p.name}</a>}
            value={ordinal(Math.floor(p.percentile))}
            note={`percentile: rated above ${p.percentile.toFixed(1)}% of ${who}`}
            tone="gold"
          />
          <MetricCard
            label="Standing"
            value={topShare(p, data.players)}
            note={`#${count(p.rank)} of ${count(data.players + (p.counted ? 0 : 1))}${p.counted ? '' : ` (${p.name} is outside these filters)`}`}
          />
          <MetricCard label={rating} value={p.value.toFixed(1)} note={`${show.engineName(percentileQuery(view).model)}, ${show.formatName(data.format)}`} />
        </MetricGrid>
      )}

      <Panels>
        <Panel
          id="percentiles-chart"
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
          aside={
            data.players > 0 && (
              <MeanBadges>
                <MeanBadge engine={view.board.engine} mean={data.mean} />
              </MeanBadges>
            )
          }
        >
          {data.players === 0 ? <Message form="inline">No players match these filters.</Message> : <SpreadChart data={data} />}
        </Panel>

        {data.marks.length > 0 && (
          <Panel id="percentiles-marks" title="Percentile Marks" subtitle={`The ${rating} a share of the players are rated at or below`}>
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
          </Panel>
        )}
      </Panels>
    </>
  );
}

/** tierBands shades each rating tier behind the bars, as the player's rating chart does. */
function tierBands(spans: TierSpan[], look: TierLook): Plugin<'bar'> {
  return {
    id: 'percentileTiers',
    beforeDatasetsDraw({ ctx, chartArea, scales }) {
      const x = scales.x;
      if (x !== undefined) drawTierBands(ctx, chartArea, (v) => x.getPixelForValue(v), 'x', spans, look);
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
    const token = documentTokens;
    const theme = chartTheme(token);
    const { gold, grid, ticks } = theme;
    const title = theme.axisTitle;
    const highlight = (i: number) => i === bins.playerBin;
    const chart = new Chart(canvas.current, {
      type: 'bar',
      plugins: [tierBands(spans, { line: token('--border-faint'), text: token('--text-soft'), font: token('--font-heading') })],
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
        scales: { x: { title: title(rating), grid, ticks }, y: { title: title('Players'), grid, ticks, beginAtZero: true, grace: '15%' } },
        plugins: {
          tooltip: {
            ...theme.tooltip,
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
