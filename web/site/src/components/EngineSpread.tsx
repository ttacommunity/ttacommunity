/** Upstream's rating distribution chart for the format chosen on the page. */
import { useEffect, useRef, useState } from 'react';
import { BarController, BarElement, CategoryScale, Chart, Legend, LinearScale, Tooltip } from 'chart.js';
import { currentRun, distribution, type Distribution, type Model } from '@ttac/shared/ratings';
import { FORMAT_NAMES, onFormat, parseAnalysisParams } from '../lib/analysis';
import { recall } from '../lib/remembered';
import { browserStorage } from '../lib/account';
import Waiting from './Waiting';
import { errorText, type Load } from '@ttac/shared/load';

Chart.register(BarController, BarElement, CategoryScale, LinearScale, Legend, Tooltip);


const BARS: { model: Model; label: string; badge: string; short: string; className: string; fill: string; line: string }[] = [
  { model: 'glicko2_daneo', label: '👑 GlickoD', badge: '👑 GlickoD', short: 'GlickoD', className: 'mean-daneo', fill: 'rgba(168, 85, 247, 0.65)', line: '#a855f7' },
  { model: 'glicko2_std', label: 'Glicko-2 Standard', badge: 'G2 Std', short: 'Std', className: 'mean-std', fill: 'rgba(234, 179, 8, 0.55)', line: '#eab308' },
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
  const [load, setLoad] = useState<Load<Distribution>>({ kind: 'loading' });
  const canvas = useRef<HTMLCanvasElement>(null);

  useEffect(() => onFormat(window, setFormat), []);

  useEffect(() => {
    // A slower answer for a format the reader has since left must not replace the new one.
    let current = true;
    setLoad({ kind: 'loading' });
    currentRun(apiUrl)
      .then((r) => distribution(apiUrl, r.run_id, format))
      .then((data) => current && setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => current && setLoad({ kind: 'error', message: errorText(e) }));
    return () => {
      current = false;
    };
  }, [apiUrl, format]);

  const dist = load.kind === 'ready' ? load.data : null;

  useEffect(() => {
    if (canvas.current === null || dist === null) return;
    const heading = getComputedStyle(document.documentElement).getPropertyValue('--font-heading').trim() || 'serif';
    const bars = means(dist);
    const grid = { color: 'rgba(74, 53, 37, 0.3)' };
    const ticks = { color: '#e0d5c1', font: { family: 'Share Tech Mono' } };
    const c = new Chart(canvas.current, {
      type: 'bar',
      data: {
        labels: dist.labels,
        datasets: bars.map((b) => ({
          label: b.label,
          data: b.counts,
          backgroundColor: b.fill,
          borderColor: b.line,
          borderWidth: 1.5,
          minBarLength: 5,
        })),
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        scales: {
          x: { title: { display: true, text: 'Rating Score Bins (100-pt intervals)', color: '#ffd700', font: { family: heading } }, grid, ticks },
          y: { title: { display: true, text: 'Number of Players', color: '#ffd700', font: { family: heading } }, grid, ticks },
        },
        plugins: {
          legend: { position: 'top', labels: { color: '#f5eedb', font: { family: 'Share Tech Mono', size: 12 }, padding: 12 } },
          tooltip: {
            backgroundColor: 'rgba(18, 12, 8, 0.95)',
            titleColor: '#ffd700',
            titleFont: { family: heading, size: 13, weight: 'bold' },
            bodyFont: { family: 'Share Tech Mono', size: 11 },
            borderColor: 'rgba(229, 169, 60, 0.4)',
            borderWidth: 1,
            padding: 10,
            callbacks: {
              title: (items) => (items[0] ? `Rating Bracket: ${dist.labels[items[0].dataIndex] ?? ''}` : ''),
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
              footer: () =>
                `\nExact Means (All Players):\n  ${bars.map((b) => `${b.short}: ${b.mean}`).join(' | ')}\n\nActive Player Means (>10 games in last yr):\n  ${bars.map((b) => `${b.short}: ${b.active}`).join(' | ')}`,
            },
          },
        },
      },
    });
    return () => c.destroy();
  }, [dist]);

  const bars = dist === null ? [] : means(dist);

  return (
    <div className="panel-card">
      <div className="card-head">
        <div>
          <h3>Rating Distribution Density Curves</h3>
          <div className="card-sub">
            Player rating distribution across 100-point skill bins for active format (<strong>{FORMAT_NAMES[format]}</strong>)
          </div>
        </div>
        <div className="density-means">
          {bars.map((b) => (
            <span key={b.model} className={`density-mean ${b.className}`} title={`${b.badge.replace('👑 ', '')} (All Players: ${b.mean} | Active Players >10 games: ${b.active})`}>
              {b.badge}: <strong>{b.mean}</strong> <small>(Active: {b.active})</small>
            </span>
          ))}
        </div>
      </div>
      <div className="density-chart-container">
        {load.kind !== 'ready' ? (
          <Waiting load={load} what="the distribution" form="inline" />
        ) : (
          <canvas
            ref={canvas}
            aria-label={`Number of players in each 100-point rating bin, per engine. ${bars.map((b) => `${b.label}: ${dist?.labels.map((l, i) => `${l} ${b.counts[i] ?? 0}`).join(', ')}`).join('. ')}.`}
          />
        )}
      </div>
    </div>
  );
}
