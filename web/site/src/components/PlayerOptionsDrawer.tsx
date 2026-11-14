/** Top-level player profile options drawer: track, reset mode, engine, and trajectory chart controls. */
import { useEffect, useRef } from 'react';
import Segmented, { type Option } from '@ttac/shared/Segmented';
import type { Engine, ResetMode, Track } from '../lib/leaderboard-view';
import type { Link } from '../lib/navigate';
import type { ChartCurve, ChartMode, ChartOptionsState, ChartScale, ChartTiers, PlayerView } from '../lib/player-view';

const narrow = () => window.innerWidth <= 768;

export interface PlayerOptionsDrawerProps {
  view: PlayerView;
  linkTo: (next: PlayerView) => Link;
  chart: ChartOptionsState;
  setScale: (s: ChartScale) => void;
  setTiers: (t: ChartTiers) => void;
  setMode: (m: ChartMode) => void;
  setCurve: (c: ChartCurve) => void;
  toggleEngine: (engine: string) => void;
  onReset: () => void;
  onClose: () => void;
}

const TRACK_OPTIONS: Option<Track>[] = [
  { value: 'community', label: 'Standard', tooltip: 'Classical asynchronous community tournaments' },
  { value: 'superblitz', label: 'SuperBlitz', tooltip: 'Exclusive SuperBlitz tournament ratings' },
  { value: 'community_superblitz', label: 'Standard + SB', tooltip: 'Standard tournaments with SuperBlitz included' },
  { value: 'pulse', label: 'Pulse', tooltip: 'TTA Pulse real-time blitz queue ratings' },
];

const RESET_OPTIONS: Option<ResetMode>[] = [
  { value: 'continuous', label: 'Continuous (Career)', tooltip: 'Uninterrupted career rating trajectory across all tournament history' },
  { value: 'softer', label: 'Season Reset', tooltip: 'Gentle annual reset balancing career achievement with current season form' },
];

const ENGINE_OPTIONS: Option<Engine>[] = [
  { value: 'glicko2_daneo', label: 'GlickoD', tooltip: "DANeo's multiplayer-weighted Glicko-2 model" },
  { value: 'glicko2_std', label: 'Glicko-2 Standard', tooltip: 'Standard Glicko-2 model' },
];

const SCALE_OPTIONS: Option<ChartScale>[] = [
  {
    value: 'perspective',
    label: 'Perspective (Φ²)',
    tooltip: 'Recency acceleration (Φ² ≈ 2.618): recent year receives largest share (~40%), earlier years smoothly condensed.',
  },
  {
    value: 'linear',
    label: 'Linear',
    tooltip: 'Uniform chronological time scale: equal horizontal width per month across all years.',
  },
];

const TIERS_OPTIONS: Option<ChartTiers>[] = [
  { value: 'on', label: 'On', tooltip: 'Show ambient rating tier bands and threshold markers' },
  { value: 'off', label: 'Off', tooltip: 'Hide background tier bands' },
];

const MODE_OPTIONS: Option<ChartMode>[] = [
  { value: 'continuous', label: 'Continuous', tooltip: 'Continuous rating development across season boundaries' },
  { value: 'softer', label: 'Season Reset', tooltip: 'Ratings calculated with seasonal rating resets' },
  { value: 'all', label: 'All Modes', tooltip: 'Show both continuous and season-reset curves together' },
];

const CURVE_OPTIONS: Option<ChartCurve>[] = [
  { value: 'expected', label: 'Expected Mean (E = R)', tooltip: 'Display expected rating mean (E = R)' },
  { value: 'conservative', label: 'Conservative (C = μ − 3σ)', tooltip: 'Display conservative rating lower bound (C = μ − 3σ)' },
  { value: 'all', label: 'All Curves', tooltip: 'Show both expected mean and conservative curves together' },
];

/** Renders the scrolling profile options sidebar (bottom sheet on mobile). */
export default function PlayerOptionsDrawer({
  view,
  linkTo,
  chart,
  setScale,
  setTiers,
  setMode,
  setCurve,
  toggleEngine,
  onReset,
  onClose,
}: PlayerOptionsDrawerProps) {
  const ref = useRef<HTMLElement>(null);

  useEffect(() => {
    const place = () => {
      const side = ref.current;
      const layout = side?.parentElement;
      if (!side || !layout) return;
      if (narrow()) {
        side.style.left = '';
        side.style.right = '';
        return;
      }
      const card = layout.getBoundingClientRect();
      const width = document.documentElement.clientWidth;
      const own = side.offsetWidth || 360;
      const ideal = card.right + (width - card.right) / 2 - own / 2;
      const left = Math.max(16, Math.min(ideal, width - own - 16));
      side.style.left = `${left}px`;
      side.style.right = 'auto';
    };
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose();
    place();
    document.body.style.overflow = narrow() ? 'hidden' : '';
    window.addEventListener('resize', place);
    document.addEventListener('keydown', onKey);
    return () => {
      document.body.style.overflow = '';
      window.removeEventListener('resize', place);
      document.removeEventListener('keydown', onKey);
    };
  }, [onClose]);

  const track = view.ranking ?? 'community';

  return (
    <aside ref={ref} className="leaderboard-sidebar player-sidebar">
      <button type="button" className="sidebar-backdrop" aria-label="Close options" tabIndex={-1} onClick={onClose} />
      <div className="leaderboard-options-panel chart-options-panel" role="dialog" aria-label="Profile options and filters">
        <div className="options-header">
          <span className="options-header-title">⚙️ Options &amp; Filters</span>
          <button type="button" className="btn-options-close" title="Close options panel" onClick={onClose}>
            ✕ Close
          </button>
        </div>

        <div className="chart-options-body">
          <div className="options-section">
            <div className="options-group-title">Track</div>
            <div className="options-section-desc">Choose which tournament rating pipeline to display across the profile.</div>
            <Segmented label="Track" look="raised" options={TRACK_OPTIONS} value={track} link={(ranking) => linkTo({ ...view, ranking, matchPage: 1 })} />
          </div>

          <div className="options-section">
            <div className="options-group-title">Reset Mode</div>
            <div className="options-section-desc">Continuous career ratings or gentle annual season resets.</div>
            <Segmented label="Reset mode" look="raised" options={RESET_OPTIONS} value={view.reset} link={(reset) => linkTo({ ...view, reset, matchPage: 1 })} />
          </div>

          <div className="options-section">
            <div className="options-group-title">Rating Engine</div>
            <div className="options-section-desc">Model used for career summary, format cards, and game deltas.</div>
            <Segmented label="Engine" look="raised" options={ENGINE_OPTIONS} value={view.engine} link={(engine) => linkTo({ ...view, engine, matchPage: 1 })} />
          </div>

          <div className="options-section-divider">Trajectory Chart</div>

          <div className="options-section">
            <div className="options-group-title">Chart Models</div>
            <div className="options-section-desc">
              Toggle individual models on the chart. GlickoD is the multiplayer-weighted baseline; Standard is unselected by default.
            </div>
            <div className="options-engine-toggles">
              <button
                type="button"
                className={['opt-engine-btn', chart.engines.has('glicko2_daneo') && 'active'].filter(Boolean).join(' ')}
                onClick={() => toggleEngine('glicko2_daneo')}
              >
                👑 GlickoD {chart.engines.has('glicko2_daneo') ? '✓' : ''}
              </button>
              <button
                type="button"
                className={['opt-engine-btn', chart.engines.has('glicko2_std') && 'active'].filter(Boolean).join(' ')}
                onClick={() => toggleEngine('glicko2_std')}
              >
                Glicko-2 Standard {chart.engines.has('glicko2_std') ? '✓' : ''}
              </button>
            </div>
          </div>

          <div className="options-section">
            <div className="options-group-title">Time Scale</div>
            <div className="options-section-desc">
              Perspective scale expands recent history with power-law recency acceleration, while Linear shows uniform spacing.
            </div>
            <Segmented label="Scale" look="raised" options={SCALE_OPTIONS} value={chart.scale} onChange={setScale} />
          </div>

          <div className="options-section">
            <div className="options-group-title">Competitive Rating Tiers</div>
            <div className="options-section-desc">
              Ambient horizontal bands and dashed threshold markers indicating competitive rating tiers (Bronze to Supreme Grandmaster).
            </div>
            <Segmented label="Tiers" look="raised" options={TIERS_OPTIONS} value={chart.tiers} onChange={setTiers} />
          </div>

          <div className="options-section">
            <div className="options-group-title">Chart Reset Mode</div>
            <div className="options-section-desc">
              Continuous career trajectory across all seasons, annual season reset rating recalculation, or all modes.
            </div>
            <Segmented label="Mode" look="raised" options={MODE_OPTIONS} value={chart.mode} onChange={setMode} />
          </div>

          <div className="options-section">
            <div className="options-group-title">Rating Estimate Curves</div>
            <div className="options-section-desc">
              Expected rating mean (E = R), conservative 99.7% lower bound (C = μ − 3σ), or all curves.
            </div>
            <Segmented label="Curve" look="raised" options={CURVE_OPTIONS} value={chart.curve} onChange={setCurve} />
          </div>
        </div>

        <div className="options-footer-actions">
          <button type="button" className="btn-options-reset" onClick={onReset}>
            Reset to Defaults
          </button>
          <button type="button" className="btn-options-close" onClick={onClose}>
            Done
          </button>
        </div>
      </div>
    </aside>
  );
}
