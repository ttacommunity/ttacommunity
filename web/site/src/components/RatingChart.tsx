/** Upstream's rating trajectory chart: models, continuous or season reset, expected or conservative. */
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
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
    tierBands?: { enabled: boolean };
  }
}
import type { PlayerHistory } from '@ttac/shared/ratings';
import {
  chartData,
  chartTicks,
  chartYBounds,
  runYears,
  RATING_TIERS,
  type ChartCurve,
  type ChartData,
  type ChartMode,
  type ChartScale,
  type ChartTick,
  type ChartTiers,
} from '../lib/player-view';
import ChartOptionsModal from './ChartOptionsModal';

const tierBandsPlugin: Plugin<'line'> = {
  id: 'tierBands',
  beforeDraw(chart, _args, options: { enabled?: boolean }) {
    if (!options?.enabled) return;
    const { ctx, chartArea, scales } = chart;
    if (!chartArea || !scales.y) return;
    const yScale = scales.y;
    ctx.save();
    ctx.beginPath();
    ctx.rect(chartArea.left, chartArea.top, chartArea.width, chartArea.height);
    ctx.clip();

    for (const tier of RATING_TIERS) {
      const yTop = yScale.getPixelForValue(Math.min(tier.max, yScale.max));
      const yBottom = yScale.getPixelForValue(Math.max(tier.min, yScale.min));
      if (yBottom <= chartArea.top || yTop >= chartArea.bottom) continue;

      const actualTop = Math.max(chartArea.top, yTop);
      const actualBottom = Math.min(chartArea.bottom, yBottom);
      const height = actualBottom - actualTop;

      if (height > 0) {
        ctx.fillStyle = tier.color;
        ctx.fillRect(chartArea.left, actualTop, chartArea.width, height);
      }

      if (tier.min > yScale.min && tier.min < yScale.max) {
        const lineY = yScale.getPixelForValue(tier.min);
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
export default function RatingChart({ history }: { history: PlayerHistory | null }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const chart = useRef<Chart<'line'> | null>(null);
  const hiddenLabels = useRef<Set<string>>(new Set());
  const initializedDefaults = useRef(false);

  const [optionsOpen, setOptionsOpen] = useState(false);
  const [optionsOpener, setOptionsOpener] = useState<HTMLElement | null>(null);

  const [scale, setScale] = useState<ChartScale>('perspective');
  const [tiers, setTiers] = useState<ChartTiers>('on');
  const [mode, setMode] = useState<ChartMode>('continuous');
  const [curve, setCurve] = useState<ChartCurve>('expected');
  const [engines, setEngines] = useState<Set<string>>(() => new Set(['glicko2_daneo']));

  const data = useMemo(() => (history === null ? null : chartData(history, mode, curve, scale)), [history, mode, curve, scale]);

  // Standard starts with Glicko Standard unselected by default.
  if (!initializedDefaults.current && data) {
    for (const line of data.lines) {
      if (line.key.startsWith('glicko2_std')) {
        hiddenLabels.current.add(line.label);
      }
    }
    initializedDefaults.current = true;
  }

  const empty = data !== null && data.labels.length === 0;
  const years = data === null || empty ? null : runYears(data.labels[0] ?? null, data.labels.at(-1) ?? null);
  const yBounds = useMemo(() => chartYBounds(history, scale, mode, curve, engines), [history, scale, mode, curve, engines]);
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

  const toggleEngine = (eng: string) => {
    setEngines((prev) => {
      const next = new Set(prev);
      if (next.has(eng)) {
        if (next.size > 1) {
          next.delete(eng);
          data?.lines.filter((l) => l.key.startsWith(eng)).forEach((l) => hiddenLabels.current.add(l.label));
        }
      } else {
        next.add(eng);
        data?.lines.filter((l) => l.key.startsWith(eng)).forEach((l) => hiddenLabels.current.delete(l.label));
      }
      return next;
    });
  };

  const resetOptions = () => {
    setScale('perspective');
    setTiers('on');
    setMode('continuous');
    setCurve('expected');
    setEngines(new Set(['glicko2_daneo']));
    hiddenLabels.current.clear();
    if (data) {
      for (const line of data.lines) {
        if (line.key.startsWith('glicko2_std')) {
          hiddenLabels.current.add(line.label);
        }
      }
    }
  };

  const hasActiveOptions =
    scale !== 'perspective' ||
    tiers !== 'on' ||
    mode !== 'continuous' ||
    curve !== 'expected' ||
    engines.size !== 1 ||
    !engines.has('glicko2_daneo');

  // A layout effect, declared before the fill below, so the chart exists when its lines are filled in.
  useIsomorphicLayoutEffect(() => {
    if (canvas.current === null || data === null || empty) return;
    const font = getComputedStyle(document.documentElement).getPropertyValue('--font-heading').trim() || 'serif';
    const grid = {
      color: 'rgba(243, 229, 200, 0.08)',
      lineWidth: 1,
    };
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
                const raw = item.raw as { y?: number | null } | undefined;
                const val = raw?.y ?? item.parsed.y;
                return ` ${item.dataset.label}: ${val}`;
              },
            },
          },
          tierBands: {
            enabled: tiers === 'on',
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
              color: '#a89880',
              callback: (value) => {
                const rounded = Math.round(Number(value) * 10) / 10;
                const tick = tickMapRef.current.get(rounded);
                return tick?.label ?? '';
              },
            },
          },
          y: {
            grid,
            border,
            suggestedMin: yBounds?.min,
            suggestedMax: yBounds?.max,
            min: yBounds?.min,
            max: yBounds?.max,
            ticks: { color: '#a89880' },
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
      c.options.scales.y.min = yBounds?.min;
      c.options.scales.y.max = yBounds?.max;
    }
    if (c.options.plugins) {
      (c.options.plugins as Record<string, unknown>).tierBands = { enabled: tiers === 'on' };
    }
    c.data.datasets = data.lines.map((line) => ({
      label: line.label,
      data: (line.points ?? []).filter((p) => p.y !== null) as unknown as number[],
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
  }, [data, scale, yBounds, tiers, ticks, engines]);

  return (
    <div className="chart-container">
      <div className="chart-head">
        <div>
          <h3>Historical Rating Trajectory{years ? ` (${years[0]} - ${years[1]})` : ''}</h3>
          <span className="chart-hint">Click legend items or open options to customize display</span>
        </div>
        <button
          type="button"
          className={['btn-options-toggle', hasActiveOptions && 'active', optionsOpen && 'open'].filter(Boolean).join(' ')}
          title="Trajectory chart options and curves"
          aria-expanded={optionsOpen}
          onClick={(e) => {
            setOptionsOpener(e.currentTarget);
            setOptionsOpen(true);
          }}
        >
          <span>⚙️ Options</span>
          {hasActiveOptions && <span className="options-active-badge">Active</span>}
          <span className="btn-options-toggle-arrow">{optionsOpen ? '✕' : '▶'}</span>
        </button>
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

      {optionsOpen && (
        <ChartOptionsModal
          opener={optionsOpener}
          scale={scale}
          setScale={setScale}
          tiers={tiers}
          setTiers={setTiers}
          mode={mode}
          setMode={setMode}
          curve={curve}
          setCurve={setCurve}
          engines={engines}
          toggleEngine={toggleEngine}
          onReset={resetOptions}
          onClose={() => {
            setOptionsOpen(false);
            setOptionsOpener(null);
          }}
        />
      )}
    </div>
  );
}
