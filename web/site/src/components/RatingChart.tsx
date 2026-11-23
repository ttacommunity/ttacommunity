/** Upstream's rating trajectory chart: models, continuous or season reset, expected or conservative. */
import { useEffect, useLayoutEffect, useMemo, useRef, type Dispatch, type SetStateAction } from 'react';
import {
  CategoryScale,
  Chart,
  Legend,
  LineController,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip,
  type ChartType,
  type Plugin,
} from 'chart.js';

declare module 'chart.js' {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  interface PluginOptionsByType<TType extends ChartType> {
    tierBands?: {
      enabled: boolean;
      warpY?: (r: number) => number;
      axisMin?: number;
      axisMax?: number;
      fMin?: number;
      fMax?: number;
      hasLowerTail?: boolean;
      hasUpperTail?: boolean;
    };
  }
}
import { chartTheme, documentTokens } from '../lib/chart-theme';
import { drawTierBands, tierSpans } from '../lib/tier-bands';
import type { PlayerHistory } from '@ttac/shared/ratings';
import Panel from './Panel';
import {
  chartData,
  chartTicks,
  labelGap,
  chartYScale,
  runYears,
  type ChartData,
  type ChartOptionsState,
  type ChartScale,
  type ChartTick,
  type ChartYTick,
} from '../lib/player-view';

const tierBandsPlugin: Plugin<'line'> = {
  id: 'tierBands',
  beforeDraw(chart, _args, options: {
    enabled?: boolean;
    warpY?: (r: number) => number;
    axisMin?: number;
    axisMax?: number;
    fMin?: number;
    fMax?: number;
    hasLowerTail?: boolean;
    hasUpperTail?: boolean;
  }) {
    if (!options?.enabled || !options.warpY) return;
    const { ctx, chartArea, scales } = chart;
    if (!chartArea || !scales.y) return;
    const { warpY, axisMin = 0, axisMax = 3000, fMin, fMax, hasLowerTail, hasUpperTail } = options;
    const yScale = scales.y;
    ctx.save();
    ctx.beginPath();
    ctx.rect(chartArea.left, chartArea.top, chartArea.width, chartArea.height);
    ctx.clip();

    const look = { line: documentTokens('--border-faint'), text: documentTokens('--text-soft'), font: documentTokens('--font-heading'), read: documentTokens };
    drawTierBands(ctx, chartArea, (v) => yScale.getPixelForValue(v), 'y', tierSpans(axisMin, axisMax, warpY), look);

    // Visual compression break markers on the Y-axis border at focal boundaries
    if (hasLowerTail && fMin !== undefined && fMin > axisMin && fMin < axisMax) {
      const breakY = yScale.getPixelForValue(warpY(fMin));
      ctx.strokeStyle = documentTokens('--chart-break');
      ctx.lineWidth = 1.5;
      ctx.setLineDash([]);
      const x = chartArea.left;
      ctx.beginPath();
      ctx.moveTo(x - 5, breakY + 3);
      ctx.lineTo(x + 5, breakY - 3);
      ctx.moveTo(x - 5, breakY + 7);
      ctx.lineTo(x + 5, breakY + 1);
      ctx.stroke();
    }

    if (hasUpperTail && fMax !== undefined && fMax > axisMin && fMax < axisMax) {
      const breakY = yScale.getPixelForValue(warpY(fMax));
      ctx.strokeStyle = documentTokens('--chart-break');
      ctx.lineWidth = 1.5;
      ctx.setLineDash([]);
      const x = chartArea.left;
      ctx.beginPath();
      ctx.moveTo(x - 5, breakY + 3);
      ctx.lineTo(x + 5, breakY - 3);
      ctx.moveTo(x - 5, breakY + 7);
      ctx.lineTo(x + 5, breakY + 1);
      ctx.stroke();
    }

    ctx.restore();
  },
};

Chart.register(LineController, LineElement, PointElement, LinearScale, CategoryScale, Legend, Tooltip, tierBandsPlugin);

const useIsomorphicLayoutEffect = typeof window !== 'undefined' ? useLayoutEffect : useEffect;

/** summary describes each line's first and latest value, the chart's text alternative. */
function summary(data: ChartData): string {
  const lines = data.lines.flatMap((line) => {
    const at = line.data.flatMap((v, i) => (v === null ? [] : [{ v, d: data.labels[i] }]));
    const first = at[0];
    const last = at[at.length - 1];
    return first && last ? [`${line.label}: ${first.v} on ${first.d}, ${last.v} on ${last.d}`] : [];
  });
  return `Rating history. ${lines.join('. ')}.`;
}

/** RatingChart draws a player's history, with perspective/linear time scaling and ambient rating tier bands. */
export default function RatingChart({
  history,
  options,
  setEngines,
}: {
  history: PlayerHistory | null;
  options: ChartOptionsState;
  setEngines: Dispatch<SetStateAction<Set<string>>>;
}) {
  const { scale, tiers, mode, curve, engines } = options;
  const canvas = useRef<HTMLCanvasElement>(null);
  const chart = useRef<Chart<'line'> | null>(null);
  const hiddenLabels = useRef<Set<string>>(new Set());
  const prevEngines = useRef<ReadonlySet<string> | null>(null);

  const data = useMemo(() => (history === null ? null : chartData(history, mode, curve, scale)), [history, mode, curve, scale]);

  // Sync hidden legend labels whenever the top-level engine selection changes (or data first loads).
  if (data && prevEngines.current !== engines) {
    for (const line of data.lines) {
      const enabled = Array.from(engines).some((eng) => line.key.startsWith(eng));
      if (enabled) hiddenLabels.current.delete(line.label);
      else hiddenLabels.current.add(line.label);
    }
    prevEngines.current = engines;
  }

  const empty = data !== null && data.labels.length === 0;
  const years = data === null || empty ? null : runYears(data.labels[0] ?? null, data.labels.at(-1) ?? null);
  const yScale = useMemo(
    () => (history ? chartYScale(history, scale, mode, curve, engines) : null),
    [history, scale, mode, curve, engines],
  );
  const yTicks = useMemo(() => yScale?.ticks ?? [], [yScale]);
  const yTicksRef = useRef<ChartYTick[]>(yTicks);
  const yTickMapRef = useRef<Map<number, ChartYTick>>(new Map());
  yTicksRef.current = yTicks;
  const yTickMap = new Map<number, ChartYTick>();
  for (const t of yTicks) {
    yTickMap.set(Math.round(t.y * 10) / 10, t);
  }
  yTickMapRef.current = yTickMap;

  // The x labels are spaced for the axis's drawn width, so they are placed when Chart.js lays the axis out.
  const spanRef = useRef<{ minDate: string; maxDate: string; scale: ChartScale } | null>(null);
  spanRef.current = data?.minDate && data.maxDate ? { minDate: data.minDate, maxDate: data.maxDate, scale } : null;
  const tickMapRef = useRef<Map<number, ChartTick>>(new Map());
  const placeXTicks = (axis: { width: number; ticks: { value: number }[] }) => {
    const span = spanRef.current;
    const spaced = span === null ? [] : chartTicks(span.minDate, span.maxDate, span.scale, labelGap(axis.width));
    tickMapRef.current = new Map(spaced.map((t) => [Math.round(t.x * 10) / 10, t]));
    axis.ticks = spaced.map((t) => ({ value: t.x }));
  };

  // A layout effect, declared before the fill below, so the chart exists when its lines are filled in.
  useIsomorphicLayoutEffect(() => {
    if (canvas.current === null || data === null || empty) return;
    const theme = chartTheme(documentTokens);
    const border = { color: theme.grid.color };
    chart.current = new Chart(canvas.current, {
      type: 'line',
      data: { datasets: [] },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        interaction: { mode: 'nearest', intersect: false },
        plugins: {
          legend: {
            labels: { ...theme.legend, boxWidth: 20 },
            onClick: (_e, legendItem, legend) => {
              const index = legendItem.datasetIndex;
              if (index === undefined) return;
              const ci = legend.chart;
              const label = ci.data.datasets[index]?.label;
              const line = data?.lines[index];
              const engine = line?.key.split('-')[0];
              if (ci.isDatasetVisible(index)) {
                ci.hide(index);
                legendItem.hidden = true;
                if (label) hiddenLabels.current.add(label);
                if (engine && data?.lines.filter((l) => l.key.startsWith(engine)).every((l) => hiddenLabels.current.has(l.label))) {
                  setEngines((prev) => {
                    const next = new Set(prev);
                    next.delete(engine);
                    return next;
                  });
                }
              } else {
                ci.show(index);
                legendItem.hidden = false;
                if (label) hiddenLabels.current.delete(label);
                if (engine) {
                  setEngines((prev) => new Set([...prev, engine]));
                }
              }
            },
          },
          tooltip: {
            ...theme.tooltip,
            callbacks: {
              title: (items) => {
                const raw = items[0]?.raw as { date?: string } | undefined;
                return raw?.date ?? '';
              },
              label: (item) => {
                const raw = item.raw as { rawY?: number | null; y?: number | null } | undefined;
                const val = raw?.rawY !== undefined && raw?.rawY !== null ? raw.rawY : item.parsed.y;
                return ` ${item.dataset.label}: ${val}`;
              },
            },
          },
          tierBands: {
            enabled: tiers === 'on',
            warpY: yScale?.warpY,
            axisMin: yScale?.axisMin,
            axisMax: yScale?.axisMax,
            fMin: yScale?.fMin,
            fMax: yScale?.fMax,
            hasLowerTail: yScale?.hasLowerTail,
            hasUpperTail: yScale?.hasUpperTail,
          },
        },
        scales: {
          x: {
            type: 'linear',
            min: 0,
            max: 100,
            grid: {
              color: (ctx) => {
                if (!ctx.tick) return 'transparent';
                const rounded = Math.round(Number(ctx.tick.value) * 10) / 10;
                const tick = tickMapRef.current.get(rounded);
                return tick?.isYear ? documentTokens('--chart-grid-year') : 'transparent';
              },
              lineWidth: 1,
            },
            border,
            afterBuildTicks: placeXTicks,
            ticks: {
              ...theme.ticks,
              align: 'inner',
              maxRotation: 0,
              autoSkip: false,
              callback: (value) => {
                const rounded = Math.round(Number(value) * 10) / 10;
                const tick = tickMapRef.current.get(rounded);
                return tick?.label ?? '';
              },
            },
          },
          y: {
            type: 'linear',
            min: 0,
            max: 100,
            grid: {
              color: (ctx) => {
                if (!ctx.tick) return 'transparent';
                const rounded = Math.round(Number(ctx.tick.value) * 10) / 10;
                const tick = yTickMapRef.current.get(rounded);
                if (tick?.isBreak) return documentTokens('--chart-grid-break');
                return documentTokens(tick?.isTier ? '--chart-grid-tier' : '--chart-grid');
              },
              lineWidth: 1,
            },
            border,
            afterBuildTicks: (axis) => {
              axis.ticks = yTicksRef.current.map((t) => ({ value: t.y }));
            },
            ticks: {
              ...theme.ticks,
              callback: (value) => {
                const rounded = Math.round(Number(value) * 10) / 10;
                const tick = yTickMapRef.current.get(rounded);
                return tick?.label ?? '';
              },
            },
          },
        },
      },
    });
    return () => {
      chart.current?.destroy();
      chart.current = null;
    };
  }, [history, empty]);

  useIsomorphicLayoutEffect(() => {
    const c = chart.current;
    if (c === null || data === null) return;
    if (c.options.scales?.x) {
      c.options.scales.x.afterBuildTicks = placeXTicks;
    }
    if (c.options.scales?.y) {
      c.options.scales.y.afterBuildTicks = (axis) => {
        axis.ticks = yTicksRef.current.map((t) => ({ value: t.y }));
      };
    }
    if (c.options.plugins) {
      (c.options.plugins as Record<string, unknown>).tierBands = {
        enabled: tiers === 'on',
        warpY: yScale?.warpY,
        axisMin: yScale?.axisMin,
        axisMax: yScale?.axisMax,
        fMin: yScale?.fMin,
        fMax: yScale?.fMax,
        hasLowerTail: yScale?.hasLowerTail,
        hasUpperTail: yScale?.hasUpperTail,
      };
    }
    c.data.datasets = data.lines.map((line) => ({
      label: line.label,
      data: (line.points ?? [])
        .filter((p) => p.y !== null)
        .map((p) => ({
          x: p.x,
          y: yScale ? yScale.warpY(p.y as number) : (p.y as number),
          rawY: p.y,
          date: p.date,
        })) as unknown as number[],
      borderColor: documentTokens(line.color),
      backgroundColor: line.fill === undefined ? 'transparent' : documentTokens(line.fill),
      borderWidth: line.borderDash ? 2 : 2.5,
      borderDash: line.borderDash,
      pointRadius: 2,
      pointHoverRadius: 5,
      tension: 0.1,
      spanGaps: true,
      hidden: hiddenLabels.current.has(line.label),
    }));
    c.update('none');
  }, [data, scale, yScale, tiers, engines]);

  return (
    <Panel
      id="rating-chart"
      title={`Historical Rating Trajectory${years ? ` (${years[0]} - ${years[1]})` : ''}`}
      subtitle="Click legend items or use the top Options panel to customize display"
    >
      <div className="chart-container-inner" role={empty ? undefined : 'img'} aria-label={empty ? undefined : 'Rating history by model'}>
        {empty ? (
          <div className="chart-empty">
            <span className="chart-empty-icon" aria-hidden="true">
              📈
            </span>
            <span className="chart-empty-title">No Historical Rating Trajectory</span>
            <span className="chart-empty-text">This player does not have recorded match history in the selected format.</span>
          </div>
        ) : (
          <canvas ref={canvas} />
        )}
      </div>
      {data !== null && !empty && <p className="visually-hidden">{summary(data)}</p>}
    </Panel>
  );
}
