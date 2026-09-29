/** Upstream's calibration diagram, ranked model table and analysis selector. */
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { CategoryScale, Chart, Legend, LineController, LineElement, LinearScale, PointElement, Tooltip } from 'chart.js';
import { calibration, currentRun, type Calibration, type Format } from '@ttac/shared/ratings';
import {
  FORMAT_NAMES,
  MODELS,
  MODE_NAMES,
  activeModels,
  eceClass,
  parseAnalysisParams,
  rankModels,
  shownInTable,
  type EvalMode,
  type Family,
  type Prior,
  type Reset,
} from '../lib/analysis';
import { recall } from '../lib/remembered';

Chart.register(LineController, LineElement, PointElement, LinearScale, CategoryScale, Legend, Tooltip);

const ALL_ENGINES: Family[] = ['glicko2_daneo', 'glicko2_mp', 'glicko2_adapt', 'glicko2_std', 'whr'];
const ALL_RESETS: Reset[] = ['continuous', 'softer'];
const BIN_LABELS = ['50-55%', '55-60%', '60-65%', '65-70%', '70-75%', '75-80%', '80-85%', '85-90%', '90-95%', '95-100%'];
const FORMATS: { value: Format; label: string }[] = [
  { value: 0, label: 'All Formats' },
  { value: 2, label: '2-Player' },
  { value: 3, label: '3-Player' },
  { value: 4, label: '4-Player' },
];
const FAMILY_BUTTONS: { family: Family; label: string; className: string }[] = [
  { family: 'glicko2_daneo', label: '👑 GlickoD', className: 'family-daneo' },
  { family: 'glicko2_mp', label: 'G2 MP', className: 'family-mp' },
  { family: 'glicko2_adapt', label: 'G2 Adapt', className: 'family-adapt' },
  { family: 'glicko2_std', label: 'G2 Std', className: 'family-std' },
  { family: 'whr', label: 'WHR', className: 'family-whr' },
];

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: Calibration };

function Toggle({ active, onClick, className, tip, children }: { active: boolean; onClick: () => void; className?: string; tip?: string; children: ReactNode }) {
  return (
    <button
      type="button"
      className={`opt-toggle-btn${className ? ` ${className}` : ''}${active ? ' active' : ''}`}
      aria-pressed={active}
      data-tooltip={tip}
      onClick={onClick}
    >
      {children}
    </button>
  );
}

/** toggled adds or removes one value; "all" selects every value, or none when all are already selected. */
function toggled<T>(set: ReadonlySet<T>, value: T | 'all', every: readonly T[]): Set<T> {
  if (value === 'all') return set.size === every.length ? new Set() : new Set(every);
  const next = new Set(set);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}

/** CalibrationPanel loads the current run's calibration and lets the reader choose which models to compare. */
export default function CalibrationPanel({ apiUrl }: { apiUrl: string }) {
  const [initial] = useState(() => parseAnalysisParams(new URLSearchParams(window.location.search), recall(window.localStorage)));
  const [mode, setMode] = useState<EvalMode>(initial.mode);
  const [format, setFormat] = useState<Format>(initial.format);
  const [engines, setEngines] = useState<ReadonlySet<Family>>(new Set(ALL_ENGINES));
  const [resets, setResets] = useState<ReadonlySet<Reset>>(new Set<Reset>(['continuous']));
  const [prior, setPrior] = useState<Prior>('standard');
  const [load, setLoad] = useState<Load>({ kind: 'loading' });
  const canvas = useRef<HTMLCanvasElement>(null);
  const chart = useRef<Chart<'line'> | null>(null);

  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => calibration(apiUrl, r.run_id))
      .then((data) => setLoad({ kind: 'ready', data }))
      .catch((e: unknown) => setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
  }, [apiUrl]);

  const filters = useMemo(() => ({ engines, resets, prior }), [engines, resets, prior]);
  const active = useMemo(() => new Set(activeModels(filters)), [filters]);
  const rows = load.kind === 'ready' ? load.data.rows : [];
  const ranked = useMemo(() => rankModels(rows, mode, format), [rows, mode, format]);
  const bins = rows.find((r) => r.mode === mode && r.format === format)?.bins;
  const ready = load.kind === 'ready';

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    params.set('eval_mode', mode);
    params.set('format', String(format));
    window.history.replaceState(null, '', `?${params}`);
  }, [mode, format]);

  useEffect(() => {
    if (canvas.current === null || !ready) return;
    const heading = getComputedStyle(document.documentElement).getPropertyValue('--font-heading').trim() || 'serif';
    const axisTitle = { color: '#ffd700', font: { family: heading, weight: 600 } };
    const ticks = { color: '#e0d5c1', font: { family: 'Share Tech Mono' } };
    const grid = { color: 'rgba(74, 53, 37, 0.3)' };
    chart.current = new Chart(canvas.current, {
      type: 'line',
      data: { labels: BIN_LABELS, datasets: [] },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
          x: { title: { display: true, text: 'Model Forecasted Probability Bin', ...axisTitle }, grid, ticks },
          y: {
            min: 0.45,
            max: 1.0,
            title: { display: true, text: 'Empirical Match Win Rate', ...axisTitle },
            grid,
            ticks: { ...ticks, callback: (v) => `${(Number(v) * 100).toFixed(0)}%` },
          },
        },
        plugins: {
          legend: {
            position: 'top',
            labels: { color: '#f5eedb', font: { family: 'Share Tech Mono', size: 11 }, usePointStyle: true, padding: 12 },
          },
          tooltip: {
            callbacks: { label: (c) => `${c.dataset.label ?? ''}: ${(Number(c.raw) * 100).toFixed(1)}% observed win rate` },
          },
        },
      },
    });
    return () => {
      chart.current?.destroy();
      chart.current = null;
    };
  }, [ready]);

  useEffect(() => {
    const c = chart.current;
    if (c === null) return;
    const ideal = bins?.ideal ?? [];
    c.data.labels = bins?.labels ?? BIN_LABELS;
    c.data.datasets = [
      {
        label: 'Ideal Benchmark (Perfect Calibration)',
        data: ideal,
        borderColor: 'rgba(255, 255, 255, 0.4)',
        borderDash: [6, 6],
        pointRadius: 0,
        fill: false,
        borderWidth: 2,
        order: 99,
      },
      ...MODELS.filter((m) => active.has(m.key)).map((m) => {
        const actuals = ranked.find((r) => r.key === m.key)?.actuals ?? [];
        return {
          label: m.name,
          data: actuals.length > 0 ? actuals : ideal,
          borderColor: m.color,
          backgroundColor: m.color,
          pointRadius: m.reset === 'continuous' ? 6 : 5,
          pointHoverRadius: 8,
          borderWidth: m.reset === 'continuous' ? 3 : 2.2,
          borderDash: m.retro ? [2, 2] : m.reset === 'softer' ? [4, 2] : [],
          tension: 0.2,
          order: 1,
        };
      }),
    ];
    c.update();
  }, [bins, ranked, active, ready]);

  const resetAll = () => {
    setEngines(new Set(ALL_ENGINES));
    setResets(new Set<Reset>(['continuous']));
    setMode('standard');
    setFormat(0);
  };

  const curveSummary = MODELS.filter((m) => active.has(m.key))
    .map((m) => m.name)
    .join(', ');

  return (
    <div className="analysis-layout-grid">
      <div className="analysis-main-column">
        <div className="graph-main-card">
          <div className="card-head">
            <div>
              <h3>Reliability Calibration Diagram</h3>
              <span className="card-sub">Predicted Win Probability vs. Observed Win Rate across 10 Probability Bins</span>
            </div>
            <div className="card-badges">
              <span className="mode-badge">{MODE_NAMES[mode]}</span>
              <span className="format-badge">{FORMAT_NAMES[format]}</span>
            </div>
          </div>
          <div className="hero-chart-container">
            {load.kind === 'error' ? (
              <p className="analysis-message">{load.message}</p>
            ) : (
              <canvas ref={canvas} aria-label={`Calibration of each model against the ideal diagonal. Curves: ${curveSummary}. The table below gives the figures.`} />
            )}
          </div>
        </div>

        <div className="eval-table-card">
          <div className="card-head">
            <div>
              <h3>Overall Predictive Model Rankings (4-Metric Evaluation)</h3>
              <div className="card-sub">
                Models ranked from best to worst by Composite Predictive Score (
                <span className="math">
                  <i>S</i> ∈ [0, 100]
                </span>
                ) under active evaluation mode
              </div>
            </div>
          </div>
          <div className="scroll-hint">
            <span>Swipe table horizontally for all 4 metrics ➔</span>
          </div>
          <div className="table-responsive">
            <table className="ranked-table">
              <thead>
                <tr>
                  <th scope="col">Rank</th>
                  <th scope="col">Rating Model &amp; Season Mode</th>
                  <th scope="col" data-tooltip="Expected Calibration Error: Mean percentage gap between predicted win chance & actual outcome. Lower is better.">
                    ECE ↓
                  </th>
                  <th scope="col" data-tooltip="Brier Score (Mean Squared Error): Overall probabilistic forecast loss. Lower is better (0.0 = perfect, 0.25 = random).">
                    Brier ↓
                  </th>
                  <th scope="col" data-tooltip="Log Loss (Cross-Entropy): Information-theoretic penalty for confident mispredictions. Lower is better.">
                    Log Loss ↓
                  </th>
                  <th scope="col" data-tooltip="Accuracy & Ist vs. Soll: Vergleicht die tatsächliche Siegquote des Favoriten (Ist) mit der im Schnitt vorhergesagten Siegchance (Soll).">
                    Favorite Win% (Ist vs. Soll) ↑
                  </th>
                  <th scope="col" data-tooltip="Composite Predictive Score (0–100): Gewichteter Index für Vorhersagegüte = Brier Score MSE (35%) + Expected Calibration Error ECE (35%) + Log Loss Cross-Entropy (15%) + Favoriten-Treffsicherheit Accuracy (15%). Höher = Besserer Prädiktor.">
                    Score ⓘ
                  </th>
                  <th scope="col">Evaluation Verdict &amp; Profile</th>
                </tr>
              </thead>
              <tbody>
                {ready &&
                  ranked
                    .filter((rm) => shownInTable(rm, filters))
                    .map((rm) => (
                      <tr key={rm.key} className={`ranked-eval-row model-${rm.key}${active.has(rm.key) ? ' active-highlight' : ''}`}>
                        <td className="rank-cell">#{rm.rank}</td>
                        <td>
                          <span className="model-dot" />
                          <strong className="model-name">{rm.name}</strong>
                        </td>
                        <td className={`ece-${eceClass(rm.ecePct)}`}>{rm.ecePct.toFixed(2)}%</td>
                        <td className="brier-cell">{rm.brier.toFixed(4)}</td>
                        <td className="logloss-cell">{rm.logLoss.toFixed(4)}</td>
                        <td className="ist-cell">
                          <strong>{rm.pIstPct.toFixed(1)}%</strong>{' '}
                          <span className={rm.pDeltaPct >= 0 ? 'soll-up' : 'soll-down'}>(Soll: {rm.pSollPct.toFixed(1)}%)</span>
                        </td>
                        <td>
                          <span className="score-badge">{rm.composite.toFixed(1)}</span>
                        </td>
                        <td>{rm.verdict}</td>
                      </tr>
                    ))}
              </tbody>
            </table>
            {load.kind === 'loading' && <p className="analysis-message">Loading calibration…</p>}
          </div>
        </div>
      </div>

      <div className="analysis-sidebar-column">
        <div className="options-sticky-panel">
          <div className="options-head">
            <span>⚙ Analysis Selector</span>
            <button type="button" className="options-reset" onClick={resetAll}>
              Reset All
            </button>
          </div>

          <div role="group" aria-label="Evaluation Mode">
            <div className="opt-section-title">🔮 Evaluation Mode</div>
            <div className="opt-btn-grid">
              <Toggle active={mode === 'standard'} onClick={() => setMode('standard')} tip="Standard match-time in-sample evaluation">
                Standard
              </Toggle>
              <Toggle active={mode === 'walk_forward'} onClick={() => setMode('walk_forward')} tip="Strict out-of-sample monthly freeze testing">
                Walk-Forward
              </Toggle>
            </div>
          </div>

          <div role="group" aria-label="Player Format">
            <div className="opt-section-title">🎮 Player Format</div>
            <div className="opt-btn-grid">
              {FORMATS.map((f) => (
                <Toggle key={f.value} active={format === f.value} onClick={() => setFormat(f.value)}>
                  {f.label}
                </Toggle>
              ))}
            </div>
          </div>

          <div role="group" aria-label="Rating Engine Family">
            <div className="opt-section-title">🏅 Rating Engine Family</div>
            <div className="opt-btn-grid">
              <Toggle active={engines.size === ALL_ENGINES.length} onClick={() => setEngines(toggled(engines, 'all', ALL_ENGINES))}>
                All Engines
              </Toggle>
              {FAMILY_BUTTONS.map((b) => (
                <Toggle key={b.family} className={b.className} active={engines.has(b.family)} onClick={() => setEngines(toggled(engines, b.family, ALL_ENGINES))}>
                  {b.label}
                </Toggle>
              ))}
            </div>
          </div>

          <div role="group" aria-label="Season Reset Mode">
            <div className="opt-section-title">↻ Season Reset Mode</div>
            <div className="opt-btn-grid">
              <Toggle active={resets.size === ALL_RESETS.length} onClick={() => setResets(toggled(resets, 'all', ALL_RESETS))}>
                All Resets
              </Toggle>
              <Toggle active={resets.has('continuous')} onClick={() => setResets(toggled(resets, 'continuous', ALL_RESETS))}>
                Continuous
              </Toggle>
              <Toggle active={resets.has('softer')} onClick={() => setResets(toggled(resets, 'softer', ALL_RESETS))}>
                Season Reset
              </Toggle>
            </div>
          </div>

          <div role="group" aria-label="Prior Calibration Lever">
            <div className="opt-section-title">⚡ Prior Calibration Lever</div>
            <div className="opt-btn-grid">
              <Toggle active={prior === 'all'} onClick={() => setPrior('all')} tip="Show both standard and retro-calibrated priors">
                All Priors
              </Toggle>
              <Toggle active={prior === 'standard'} onClick={() => setPrior('standard')} tip="Standard initial priors (1500 / 350 RD)">
                Standard (1500)
              </Toggle>
              <Toggle active={prior === 'retro'} onClick={() => setPrior('retro')} tip="Two-pass 15-game emergent calibration with initial freeze (Option A)">
                15G Retro (★)
              </Toggle>
            </div>
          </div>

          <div className="options-foot">
            <span>
              Active Curves: <strong>{active.size}</strong>
            </span>
            <span className="options-live">Live Reactivity ✔</span>
          </div>
        </div>
      </div>
    </div>
  );
}
