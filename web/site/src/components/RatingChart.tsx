/** Upstream's rating trajectory chart: models, continuous or season reset, expected or conservative. */
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
import { chartData, runYears, type ChartCurve, type ChartData, type ChartMode } from '../lib/player-view';
import Segmented from '@ttac/shared/Segmented';

Chart.register(LineController, LineElement, PointElement, LinearScale, CategoryScale, Legend, Tooltip);

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
    <Segmented
      label={label}
      look="raised"
      groupClass="chart-toggle"
      prefix={<span className={muted ? 'chart-toggle-label muted' : 'chart-toggle-label'}>{label}:</span>}
      options={options}
      value={value}
      onChange={onChange}
    />
  );
}

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

/** RatingChart draws a player's history, with instant in-place mode and curve switches and all-selections support. */
export default function RatingChart({ history }: { history: PlayerHistory | null }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const chart = useRef<Chart<'line'> | null>(null);
  const [mode, setMode] = useState<ChartMode>('continuous');
  const [curve, setCurve] = useState<ChartCurve>('expected');
  const data = useMemo(() => (history === null ? null : chartData(history, mode, curve)), [history, mode, curve]);
  const empty = data !== null && data.labels.length === 0;
  const years = data === null || empty ? null : runYears(data.labels[0] ?? null, data.labels.at(-1) ?? null);

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
        animation: false,
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
  }, [history, empty]);

  useEffect(() => {
    const c = chart.current;
    if (c === null || data === null) return;
    c.data.labels = data.labels;
    c.data.datasets = data.lines.map((line, i) => ({
      label: line.label,
      data: line.data,
      borderColor: line.color,
      backgroundColor: line.fill ?? 'transparent',
      borderWidth: line.borderDash ? 2 : 2.5,
      borderDash: line.borderDash,
      pointRadius: 1.5,
      tension: 0.1,
      spanGaps: true,
      hidden: c.data.datasets[i] !== undefined && !c.isDatasetVisible(i),
    }));
    c.update('none');
  }, [data]);

  return (
    <div className="chart-container">
      <div className="chart-head">
        <div>
          <h3>Historical Rating Trajectory{years ? ` (${years[0]} - ${years[1]})` : ''}</h3>
          <span className="chart-hint">Click legend items to toggle individual models</span>
        </div>
        <div className="chart-toggles">
          <Toggle
            label="Mode"
            options={[
              { value: 'continuous', label: 'Continuous' },
              { value: 'softer', label: 'Season Reset' },
              { value: 'all', label: 'All Modes' },
            ]}
            value={mode}
            onChange={setMode}
          />
          <Toggle
            label="Curve"
            muted
            options={[
              { value: 'expected', label: 'Expected Mean (E = R)' },
              { value: 'conservative', label: 'Conservative (C = μ − 3σ)' },
              { value: 'all', label: 'All Curves' },
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
