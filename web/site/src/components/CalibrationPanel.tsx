/** Upstream's calibration diagram and ranked model table, with the format in a ViewBar and the other choices in an options drawer. */
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';

const useIsomorphicLayoutEffect = typeof window !== 'undefined' ? useLayoutEffect : useEffect;
import { CategoryScale, Chart, Legend, LineController, LineElement, LinearScale, PointElement, Tooltip } from 'chart.js';
import { calibration, currentRun, type Format } from '@ttac/shared/ratings';
import {
  MODELS,
  MODE_NAMES,
  activeModels,
  announceFormat,
  eceClass,
  parseAnalysisParams,
  rankModels,
  type EvalMode,
  type Family,
  type Reset,
} from '../lib/analysis';
import { recall } from '../lib/remembered';
import { browserStorage } from '@ttac/shared/storage';
import Tex from './Tex';
import ScrollTable from './ScrollTable';
import Waiting from './Waiting';
import { chartTheme, documentTokens } from '../lib/chart-theme';
import { formatName } from '../lib/leaderboard-display';
import { useLoad } from './useLoad';
import Panel from './Panel';
import Panels from './Panels';
import CalibrationOptionsDrawer from './CalibrationOptionsDrawer';
import { OptionsToggle } from './SideDrawer';
import ViewBar, { FormatPick } from './ViewBar';

Chart.register(LineController, LineElement, PointElement, LinearScale, CategoryScale, Legend, Tooltip);

const ALL_ENGINES: Family[] = ['glicko2_daneo', 'glicko2_std'];
const BIN_LABELS = ['50-55%', '55-60%', '60-65%', '65-70%', '70-75%', '75-80%', '80-85%', '85-90%', '90-95%', '95-100%'];

/** CalibrationPanel loads the current run's calibration and lets the reader choose which models to compare. */
export default function CalibrationPanel({ apiUrl }: { apiUrl: string }) {
  const [initial] = useState(() => parseAnalysisParams(new URLSearchParams(window.location.search), recall(browserStorage())));
  const [mode, setMode] = useState<EvalMode>(initial.mode);
  const [format, setFormat] = useState<Format>(initial.format);
  const [engines, setEngines] = useState<ReadonlySet<Family>>(new Set(ALL_ENGINES));
  const [resets, setResets] = useState<ReadonlySet<Reset>>(new Set<Reset>(['continuous']));
  const [optionsOpen, setOptionsOpen] = useState(false);
  const load = useLoad('calibration', () => currentRun(apiUrl).then((r) => calibration(apiUrl, r.run_id)));
  const canvas = useRef<HTMLCanvasElement>(null);
  const chart = useRef<Chart<'line'> | null>(null);

  const filters = useMemo(() => ({ engines, resets }), [engines, resets]);
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

  useEffect(() => announceFormat(window, format), [format]);

  useEffect(() => {
    if (canvas.current === null || !ready) return;
    const theme = chartTheme(documentTokens);
    const { grid, ticks } = theme;
    chart.current = new Chart(canvas.current, {
      type: 'line',
      data: { labels: BIN_LABELS, datasets: [] },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        scales: {
          x: { title: theme.axisTitle('Model Forecasted Probability Bin'), grid, ticks },
          y: {
            min: 0.45,
            max: 1.0,
            title: theme.axisTitle('Empirical Match Win Rate'),
            grid,
            ticks: { ...ticks, callback: (v) => `${(Number(v) * 100).toFixed(0)}%` },
          },
        },
        plugins: {
          legend: {
            position: 'top',
            labels: { ...theme.legend, usePointStyle: true },
          },
          tooltip: {
            ...theme.tooltip,
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

  useIsomorphicLayoutEffect(() => {
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
          borderDash: m.family === 'glicko2_daneo' ? [2, 2] : [],
          tension: 0.2,
          order: 1,
        };
      }),
    ];
    c.update('none');
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
    <Panels>
      <ViewBar
        actions={
          <OptionsToggle
            title="Evaluation mode, engines, and reset modes"
            active={mode !== 'standard' || engines.size !== ALL_ENGINES.length || resets.size !== 1 || !resets.has('continuous')}
            open={optionsOpen}
            onClick={() => setOptionsOpen(!optionsOpen)}
          />
        }
      >
        <FormatPick value={format} onChange={setFormat} what="calibration" />
      </ViewBar>
      {optionsOpen && (
        <CalibrationOptionsDrawer
          mode={mode}
          engines={engines}
          resets={resets}
          curves={active.size}
          setMode={setMode}
          setEngines={setEngines}
          setResets={setResets}
          onReset={resetAll}
          onClose={() => setOptionsOpen(false)}
        />
      )}
      <Panel
        id="calibration-diagram"
        title="Reliability Calibration Diagram"
        subtitle="Predicted Win Probability vs. Observed Win Rate across 10 Probability Bins"
        className="graph-main-card"
        aside={
          <div className="card-badges">
            <span className="badge badge-format">{formatName(format)}</span>
            <span className="badge badge-mode">{MODE_NAMES[mode]}</span>
          </div>
        }
      >
        <div className="hero-chart-container">
          {load.kind === 'error' ? (
            <Waiting load={load} what="the calibration" form="inline" />
          ) : (
            <canvas ref={canvas} aria-label={`Calibration of each model against the ideal diagonal. Curves: ${curveSummary}. The table below gives the figures.`} />
          )}
        </div>
      </Panel>

      <Panel
        id="calibration-rankings"
        title="Overall Predictive Model Rankings (4-Metric Evaluation)"
        subtitle={
          <>
            Models ranked from best to worst by Composite Predictive Score (<Tex tex="S \in [0, 100]" />) under active evaluation mode
          </>
        }
      >
        <ScrollTable hint="all 4 metrics">
          <table className="leaderboard-table ranked-table">
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
                <th scope="col" data-tooltip="Accuracy & Actual vs. Expected: Compares the favorite's actual win rate (Actual) with their average predicted win probability (Expected).">
                  Favorite Win% (Actual vs. Expected) ↑
                </th>
                <th scope="col" data-tooltip="Composite Predictive Score (0–100): Weighted index of predictive quality = Brier Score MSE (35%) + Expected Calibration Error ECE (35%) + Log Loss Cross-Entropy (15%) + Favorite Accuracy (15%). Higher is better.">
                  Score ⓘ
                </th>
              </tr>
            </thead>
            <tbody>
              {ready &&
                ranked.map((rm) => (
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
                        <span className={rm.pDeltaPct >= 0 ? 'soll-up' : 'soll-down'}>(Expected: {rm.pSollPct.toFixed(1)}%)</span>
                      </td>
                      <td>
                        <span className="score-badge">{rm.composite.toFixed(1)}</span>
                      </td>
                    </tr>
                  ))}
            </tbody>
          </table>
          {load.kind === 'loading' && <Waiting load={load} what="the calibration" form="inline" />}
        </ScrollTable>
      </Panel>
    </Panels>
  );
}
