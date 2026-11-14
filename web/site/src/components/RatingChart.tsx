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
import type { PlayerHistory } from '@ttac/shared/ratings';
import {
  chartData,
  chartTicks,
  chartYScale,
  runYears,
  RATING_TIERS,
  type ChartData,
  type ChartOptionsState,
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

    for (const tier of RATING_TIERS) {
      const tMin = Math.max(tier.min, axisMin);
      const tMax = Math.min(tier.max, axisMax);
      if (tMin >= tMax) continue;

      const yTop = yScale.getPixelForValue(warpY(tMax));
      const yBottom = yScale.getPixelForValue(warpY(tMin));
      if (yBottom <= chartArea.top || yTop >= chartArea.bottom) continue;

      const actualTop = Math.max(chartArea.top, yTop);
      const actualBottom = Math.min(chartArea.bottom, yBottom);
      const height = actualBottom - actualTop;

      if (height > 0) {
        ctx.fillStyle = tier.color;
        ctx.fillRect(chartArea.left, actualTop, chartArea.width, height);
      }

      if (tier.min > axisMin && tier.min < axisMax) {
        const lineY = yScale.getPixelForValue(warpY(tier.min));
        ctx.strokeStyle = tier.borderColor ?? 'rgba(74, 53, 37, 0.35)';
        ctx.lineWidth = 1;
        ctx.setLineDash([4, 4]);
        ctx.beginPath();
        ctx.moveTo(chartArea.left, lineY);
        ctx.lineTo(chartArea.right, lineY);
        ctx.stroke();

        ctx.fillStyle = 'rgba(243, 229, 200, 0.35)';
        ctx.font = '10px serif';
        ctx.textAlign = 'right';
        ctx.textBaseline = 'bottom';
        ctx.fillText(`${tier.min} · ${tier.code}`, chartArea.right - 6, lineY - 3);
      }
    }

    // Visual compression break markers on the Y-axis border at focal boundaries
    if (hasLowerTail && fMin !== undefined && fMin > axisMin && fMin < axisMax) {
      const breakY = yScale.getPixelForValue(warpY(fMin));
      ctx.strokeStyle = 'rgba(212, 175, 55, 0.65)';
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
      ctx.strokeStyle = 'rgba(212, 175, 55, 0.65)';
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

  const ticks = useMemo(
    () => (data && data.minDate && data.maxDate ? chartTicks(data.minDate, data.maxDate, scale) : []),
    [data, scale],
  );

  const ticksRef = useRef<ChartTick[]>(ticks);
  const tickMapRef = useRef<Map<number, ChartTick>>(new Map());
  ticksRef.current = ticks;
  const tickMap = new Map<number, ChartTick>();
  for (const t of ticks) {
    tickMap.set(Math.round(t.x * 10) / 10, t);
  }
  tickMapRef.current = tickMap;

  // A layout effect, declared before the fill below, so the chart exists when its lines are filled in.
  useIsomorphicLayoutEffect(() => {
    if (canvas.current === null || data === null || empty) return;
    const font = getComputedStyle(document.documentElement).getPropertyValue('--font-heading').trim() || 'serif';
        const border = { color: 'rgba(74, 53, 37, 0.5)' };
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
            labels: {
              color: '#f3e5c8',
              font: { family: font, size: 11 },
              boxWidth: 20,
              padding: 12,
            },
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
            backgroundColor: 'rgba(28, 19, 13, 0.95)',
            titleColor: '#e5a93c',
            bodyColor: '#f3e5c8',
            borderColor: '#4a3525',
            borderWidth: 1,
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
                return tick?.isYear ? 'rgba(243, 229, 200, 0.10)' : 'transparent';
              },
              lineWidth: 1,
            },
            border,
            afterBuildTicks: (axis) => {
              axis.ticks = ticksRef.current.map((t) => ({ value: t.x }));
            },
            ticks: {
              align: 'inner',
              color: '#a89880',
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
                if (tick?.isBreak) return 'rgba(212, 175, 55, 0.25)';
                return tick?.isTier ? 'rgba(243, 229, 200, 0.12)' : 'rgba(243, 229, 200, 0.06)';
              },
              lineWidth: 1,
            },
            border,
            afterBuildTicks: (axis) => {
              axis.ticks = yTicksRef.current.map((t) => ({ value: t.y }));
            },
            ticks: {
              color: '#a89880',
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
      c.options.scales.x.afterBuildTicks = (axis) => {
        axis.ticks = ticks.map((t) => ({ value: t.x }));
      };
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
      borderColor: line.color,
      backgroundColor: line.fill ?? 'transparent',
      borderWidth: line.borderDash ? 2 : 2.5,
      borderDash: line.borderDash,
      pointRadius: 2,
      pointHoverRadius: 5,
      tension: 0.1,
      spanGaps: true,
      hidden: hiddenLabels.current.has(line.label),
    }));
    c.update('none');
  }, [data, scale, yScale, tiers, ticks, engines]);

  return (
    <div className="chart-container">
      <div className="chart-head">
        <div>
          <h3>Historical Rating Trajectory{years ? ` (${years[0]} - ${years[1]})` : ''}</h3>
          <span className="chart-hint">Click legend items or use the top Options panel to customize display</span>
        </div>
      </div>
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

    </div>
  );
}
