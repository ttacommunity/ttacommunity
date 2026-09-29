/** Upstream's rating trajectory chart: five engines, continuous or season reset, expected or conservative. */
import { useEffect, useMemo, useRef, useState } from 'react';
import {
  CategoryScale,
  Chart,
  Legend,
  LineController,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip,
} from 'chart.js';
import type { PlayerHistory } from '@ttac/shared/ratings';
import { ENGINES, type Engine, type ResetMode } from '../lib/leaderboard-view';
import { chartData, type ChartData } from '../lib/player-view';

Chart.register(LineController, LineElement, PointElement, LinearScale, CategoryScale, Legend, Tooltip);

type Curve = 'expected' | 'conservative';

const LINES: Record<Engine, { name: string; color: string; fill: string; width: number; point: number; tension: number }> = {
  glicko2_daneo: { name: '👑 GlickoD', color: '#c084fc', fill: 'rgba(168, 85, 247, 0.15)', width: 2.5, point: 1.5, tension: 0.1 },
  glicko2_std: { name: 'Glicko-2 Standard', color: '#e5a93c', fill: 'rgba(229, 169, 60, 0.1)', width: 2, point: 1, tension: 0.1 },
  glicko2_mp: { name: 'Glicko-2 MP-Weighted', color: '#8e1e1d', fill: 'rgba(142, 30, 29, 0.1)', width: 2, point: 1, tension: 0.1 },
  glicko2_adapt: { name: 'Glicko-2 Adaptive-T', color: '#22c55e', fill: 'rgba(34, 197, 94, 0.1)', width: 2, point: 1, tension: 0.1 },
  whr: { name: 'Whole-History Rating', color: '#4fc3f7', fill: 'rgba(79, 195, 247, 0.1)', width: 2, point: 1, tension: 0.2 },
};

function Toggle<T extends string>({
  label,
  muted,
  options,
  value,
  onChange,
}: {
  label: string;
  muted?: boolean;
  options: { value: T; label: string }[];
  value: T;
  onChange: (v: T) => void;
}) {
  return (
    <div className="chart-toggle" role="group" aria-label={label}>
      <span className={muted ? 'chart-toggle-label muted' : 'chart-toggle-label'}>{label}:</span>
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          className={o.value === value ? 'format-btn active' : 'format-btn'}
          aria-pressed={o.value === value}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

/** summary describes each line's first and latest value, the chart's text alternative. */
function summary(data: ChartData): string {
  const lines = ENGINES.flatMap((engine) => {
    const at = data.series[engine].flatMap((v, i) => (v === null ? [] : [{ v, d: data.labels[i] }]));
    const first = at[0];
    const last = at[at.length - 1];
    return first && last ? [`${LINES[engine].name}: ${first.v} on ${first.d}, ${last.v} on ${last.d}`] : [];
  });
  return `Rating history. ${lines.join('. ')}.`;
}

/** RatingChart draws a player's history, with upstream's mode and curve switches. */
export default function RatingChart({ history }: { history: PlayerHistory | null }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const chart = useRef<Chart<'line'> | null>(null);
  const [mode, setMode] = useState<ResetMode>('continuous');
  const [curve, setCurve] = useState<Curve>('expected');
  const data = useMemo(() => (history === null ? null : chartData(history, mode, curve)), [history, mode, curve]);
  const empty = data !== null && data.labels.length === 0;

  useEffect(() => {
    if (canvas.current === null || data === null || empty) return;
    const font = getComputedStyle(document.documentElement).getPropertyValue('--font-heading').trim() || 'serif';
    const grid = { color: 'rgba(74, 53, 37, 0.25)' };
    chart.current = new Chart(canvas.current, {
      type: 'line',
      data: { labels: [], datasets: [] },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: { labels: { color: '#f3e5c8', font: { family: font, size: 12 } } },
          tooltip: {
            backgroundColor: 'rgba(28, 19, 13, 0.95)',
            titleColor: '#e5a93c',
            bodyColor: '#f3e5c8',
            borderColor: '#4a3525',
            borderWidth: 1,
          },
        },
        scales: {
          x: { grid, ticks: { color: '#a89880', maxTicksLimit: 12 } },
          y: { grid, ticks: { color: '#a89880' } },
        },
      },
    });
    return () => {
      chart.current?.destroy();
      chart.current = null;
    };
    // Made once per history: the switches only change its data, below, keeping the legend's choices.
  }, [history, empty]);

  useEffect(() => {
    const c = chart.current;
    if (c === null || data === null) return;
    const suffix = `${mode === 'softer' ? '[Reset] ' : ''}${curve === 'conservative' ? '(C)' : '(E)'}`;
    c.data.labels = data.labels;
    c.data.datasets = ENGINES.map((engine, i) => {
      const line = LINES[engine];
      return {
        label: `${line.name} ${suffix}`,
        data: data.series[engine],
        borderColor: line.color,
        backgroundColor: line.fill,
        borderWidth: line.width,
        pointRadius: line.point,
        tension: line.tension,
        spanGaps: true,
        hidden: c.data.datasets[i] !== undefined && !c.isDatasetVisible(i),
      };
    });
    c.update();
  }, [data, mode, curve]);

  return (
    <div className="chart-container">
      <div className="chart-head">
        <div>
          <h3>Historical Rating Trajectory (2017 - 2026)</h3>
          <span className="chart-hint">Click legend items to toggle individual models</span>
        </div>
        <div className="chart-toggles">
          <Toggle
            label="Mode"
            options={[
              { value: 'continuous', label: 'Continuous' },
              { value: 'softer', label: 'Season Reset' },
            ]}
            value={mode}
            onChange={setMode}
          />
          <Toggle
            label="Curve"
            muted
            options={[
              { value: 'expected', label: 'Expected Mean (E = R)' },
              { value: 'conservative', label: 'Conservative (C = R − 2σ)' },
            ]}
            value={curve}
            onChange={setCurve}
          />
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
