/** Upstream's rating distribution chart for the format chosen on the page. */
import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { BarController, BarElement, CategoryScale, Chart, Legend, LinearScale, Tooltip } from 'chart.js';
import { currentRun, distribution, type Distribution } from '@ttac/shared/ratings';
import { onFormat, parseAnalysisParams } from '../lib/analysis';
import { formatName } from '../lib/leaderboard-display';
import type { Engine } from '../lib/leaderboard-view';
import { recall } from '../lib/remembered';
import { browserStorage } from '@ttac/shared/storage';
import Waiting from './Waiting';
import MeanBadge, { MeanBadges } from './MeanBadge';
import { chartTheme, documentTokens } from '../lib/chart-theme';
import { useKeptLoad } from './useLoad';
import Panel from './Panel';

Chart.register(BarController, BarElement, CategoryScale, LinearScale, Legend, Tooltip);

const useIsomorphicLayoutEffect = typeof window !== 'undefined' ? useLayoutEffect : useEffect;

const BARS: { model: Engine; label: string; short: string; fill: string; line: string }[] = [
  { model: 'glicko2_daneo', label: '👑 GlickoD', short: 'GlickoD', fill: '--model-daneo-bar', line: '--model-daneo' },
  { model: 'glicko2_std', label: 'Glicko-2 Standard', short: 'Std', fill: '--model-std-bar', line: '--model-std' },
];

/** means returns each bar's all-player and active-player mean, 1500 for an engine the answer lacks. */
function means(d: Distribution) {
  return BARS.map((b) => {
    const e = d.engines.find((x) => x.model === b.model);
    return { ...b, mean: e?.mean ?? 1500, active: e?.active_mean ?? 1500, counts: e?.counts ?? [] };
  });
}

/** EngineSpread loads the current run's distribution for the format in the address, or the remembered one. */
export default function EngineSpread({ apiUrl }: { apiUrl: string }) {
  const [format, setFormat] = useState(() => parseAnalysisParams(new URLSearchParams(window.location.search), recall(browserStorage())).format);
  const load = useKeptLoad(String(format), () => currentRun(apiUrl).then((r) => distribution(apiUrl, r.run_id, format)));
  const distData = load.kind === 'ready' ? load.data : null;
  const canvas = useRef<HTMLCanvasElement>(null);
  const chart = useRef<Chart<'bar'> | null>(null);
  const distRef = useRef<Distribution | null>(null);
  distRef.current = distData;

  useEffect(() => onFormat(window, setFormat), []);
  useEffect(() => () => {
    chart.current?.destroy();
    chart.current = null;
  }, []);

  // Create chart instance once canvas and data are both present, then update datasets on change
  useIsomorphicLayoutEffect(() => {
    if (canvas.current === null || distData === null) return;
    if (chart.current === null) {
    const theme = chartTheme(documentTokens);
    const { grid, ticks } = theme;

    chart.current = new Chart(canvas.current, {
      type: 'bar',
      data: { labels: [], datasets: [] },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        interaction: { mode: 'index', intersect: false },
        scales: {
          x: { title: theme.axisTitle('Rating Score Bins (100-pt intervals)'), grid, ticks },
          y: { title: theme.axisTitle('Number of Players'), grid, ticks },
        },
        plugins: {
          legend: { position: 'top', labels: theme.legend },
          tooltip: {
            ...theme.tooltip,
            callbacks: {
              title: (items) => (items[0] && distRef.current ? `Rating Bracket: ${distRef.current.labels[items[0].dataIndex] ?? ''}` : ''),
              label: (ctx) => {
                const counts = ctx.dataset.data as number[];
                const val = ctx.parsed.y ?? 0;
                const total = counts.reduce((a, b) => a + b, 0);
                const cum = counts.slice(0, ctx.dataIndex + 1).reduce((a, b) => a + b, 0);
                const pct = total > 0 ? ((val / total) * 100).toFixed(1) : '0.0';
                const cumPct = total > 0 ? ((cum / total) * 100).toFixed(1) : '0.0';
                const topPct = total > 0 ? (((total - cum) / total) * 100).toFixed(1) : '0.0';
                return ` ${ctx.dataset.label ?? ''}: ${val} (${pct}%) • Percentile: ${cumPct}% (Top ${topPct}%)`;
              },
              footer: () => {
                if (!distRef.current) return '';
                const currentBars = means(distRef.current);
                return `\nExact Means (All Players):\n  ${currentBars.map((b) => `${b.short}: ${b.mean}`).join(' | ')}\n\nActive Player Means (>10 games in last yr):\n  ${currentBars.map((b) => `${b.short}: ${b.active}`).join(' | ')}`;
              },
            },
          },
        },
      },
    });

    }
    const c = chart.current;
    const currentBars = means(distData);
    c.data.labels = distData.labels;
    c.data.datasets = currentBars.map((b) => ({
      label: b.label,
      data: b.counts,
      backgroundColor: documentTokens(b.fill),
      borderColor: documentTokens(b.line),
      borderWidth: 1.5,
      minBarLength: 5,
    }));
    c.update('none');
  }, [distData]);

  const bars = distData === null ? [] : means(distData);

  return (
    <Panel
      id="engine-spread"
      title="Rating Distribution Density Curves"
      subtitle={
        <>
          Player rating distribution across 100-point skill bins for active format (<strong>{formatName(format)}</strong>)
        </>
      }
      aside={
        <MeanBadges>
          {bars.map((b) => (
            <MeanBadge key={b.model} engine={b.model} mean={b.mean} active={b.active} />
          ))}
        </MeanBadges>
      }
    >
      <div className="density-chart-container">
        {distData === null && load.kind !== 'ready' ? (
          <Waiting load={load} what="the distribution" form="inline" />
        ) : (
          <canvas
            ref={canvas}
            aria-label={`Number of players in each 100-point rating bin, per engine. ${bars.map((b) => `${b.label}: ${distData?.labels.map((l, i) => `${l} ${b.counts[i] ?? 0}`).join(', ')}`).join('. ')}.`}
          />
        )}
      </div>
    </Panel>
  );
}
