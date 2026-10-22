/** Upstream's rating distribution chart and engine agreement cards, for the format chosen on the page. */
import { useEffect, useRef, useState } from 'react';
import { BarController, BarElement, CategoryScale, Chart, Legend, LinearScale, Tooltip } from 'chart.js';
import { agreement, currentRun, distribution, type Agreement, type Distribution, type Model, type ModelPair } from '@ttac/shared/ratings';
import { FORMAT_NAMES, agreementName, onFormat, parseAnalysisParams } from '../lib/analysis';
import { recall } from '../lib/remembered';
import Tex from './Tex';
import Waiting from './Waiting';

Chart.register(BarController, BarElement, CategoryScale, LinearScale, Legend, Tooltip);

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; dist: Distribution; agree: Agreement };

const BARS: { model: Model; label: string; badge: string; short: string; className: string; fill: string; line: string }[] = [
  { model: 'glicko2_daneo', label: '👑 GlickoD', badge: '👑 GlickoD', short: 'GlickoD', className: 'mean-daneo', fill: 'rgba(229, 169, 60, 0.7)', line: '#e5a93c' },
  { model: 'glicko2_std', label: 'Glicko-2 Standard', badge: 'G2 Std', short: 'Std', className: 'mean-std', fill: 'rgba(255, 202, 40, 0.5)', line: '#ffca28' },
];

/** means returns each bar's all-player and active-player mean, 1500 for an engine the answer lacks. */
function means(d: Distribution) {
  return BARS.map((b) => {
    const e = d.engines.find((x) => x.model === b.model);
    return { ...b, mean: e?.mean ?? 1500, active: e?.active_mean ?? 1500, counts: e?.counts ?? [] };
  });
}

function PairCard({ title, className, pairs }: { title: string; className: string; pairs: ModelPair[] }) {
  return (
    <div className="corr-card">
      <div className={`corr-card-title ${className}`}>{title}</div>
      {pairs.map((p) => (
        <div key={`${p.a}|${p.b}`} className="corr-pair-row">
          <div>
            <strong>{agreementName(p.a)}</strong>
            <span className="pair-sep"> ↔ </span>
            <strong>{agreementName(p.b)}</strong>
          </div>
          <div className={`corr-value ${className}`}>r = {p.spearman.toFixed(4)}</div>
        </div>
      ))}
    </div>
  );
}

/** EngineSpread loads the current run's distribution and agreement for the format in the address, or the remembered one. */
export default function EngineSpread({ apiUrl }: { apiUrl: string }) {
  const [format, setFormat] = useState(() => parseAnalysisParams(new URLSearchParams(window.location.search), recall(window.localStorage)).format);
  const [load, setLoad] = useState<Load>({ kind: 'loading' });
  const canvas = useRef<HTMLCanvasElement>(null);

  useEffect(() => onFormat(window, setFormat), []);

  useEffect(() => {
    // A slower answer for a format the reader has since left must not replace the new one.
    let current = true;
    setLoad({ kind: 'loading' });
    currentRun(apiUrl)
      .then((r) => Promise.all([distribution(apiUrl, r.run_id, format), agreement(apiUrl, r.run_id, format)]))
      .then(([dist, agree]) => current && setLoad({ kind: 'ready', dist, agree }))
      .catch((e: unknown) => current && setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
    return () => {
      current = false;
    };
  }, [apiUrl, format]);

  const dist = load.kind === 'ready' ? load.dist : null;

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
  const sample = load.kind === 'ready' ? load.agree.sample : null;

  return (
    <>
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

      <div className="panel-card">
        <div className="card-head">
          <div>
            <h3>Engine Agreement &amp; Ranking Correlation Insights</h3>
            <div className="card-sub">
              Spearman Rank (<Tex tex="r_s" />) &amp; Pearson (<Tex tex="r" />) Correlation Benchmark across All 10 Model Configurations (
              <Tex tex={`N = ${sample ?? '\\dots'}`} /> active players)
            </div>
          </div>
        </div>

        {load.kind === 'ready' && (
          <div className="corr-grid">
            <PairCard title="✔ Strongest Agreement Pairs" className="corr-strong" pairs={load.agree.top} />
            <PairCard title="⚠ Divergent & Seasonal Mobility Pairs" className="corr-weak" pairs={load.agree.weakest} />
          </div>
        )}

        <div className="corr-note">
          <strong>💡 What do these correlation metrics tell us?</strong>
          <br />
          High Spearman rank correlation (<Tex tex="r_s > 0.95" />) demonstrates that different engines independently converge on the same relative skill hierarchy at the top of the leaderboard. Lower
          correlation values between <em>Continuous (Career)</em> and <em>Hard Reset</em> models reflect the deliberate annual mobility of seasonal
          ladders, where active title contenders can ascend past inactive veterans.
        </div>
      </div>
    </>
  );
}
