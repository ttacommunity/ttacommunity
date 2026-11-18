/** Top-level player profile options drawer: track, reset mode, engine, and trajectory chart controls. */
import { OptionsDivider, OptionsPanel, OptionsSection } from './SideDrawer';
import Segmented, { type Option } from '@ttac/shared/Segmented';
import type { Engine, ResetMode, Track } from '../lib/leaderboard-view';
import type { Link } from '../lib/navigate';
import type { ChartCurve, ChartMode, ChartOptionsState, ChartScale, ChartTiers, PlayerView } from '../lib/player-view';

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
  const track = view.ranking ?? 'community';

  return (
    <OptionsPanel label="Profile options and filters" onClose={onClose} reset={onReset}>
      <OptionsSection title="Track" tip="Choose which tournament rating pipeline to display across the profile">
        <Segmented label="Track" options={TRACK_OPTIONS} value={track} link={(ranking) => linkTo({ ...view, ranking, matchPage: 1 })} />
      </OptionsSection>

      <OptionsSection title="Reset Mode" tip="Continuous career ratings or gentle annual season resets">
        <Segmented label="Reset mode" options={RESET_OPTIONS} value={view.reset} link={(reset) => linkTo({ ...view, reset, matchPage: 1 })} />
      </OptionsSection>

      <OptionsSection title="Rating Engine" tip="Model used for career summary, format cards, and game deltas">
        <Segmented label="Engine" options={ENGINE_OPTIONS} value={view.engine} link={(engine) => linkTo({ ...view, engine, matchPage: 1 })} />
      </OptionsSection>

      <OptionsDivider>Trajectory Chart</OptionsDivider>

      <OptionsSection title="Chart Models" tip="Toggle individual models on the chart. GlickoD is the multiplayer-weighted baseline; Standard is unselected by default.">
        <div className="format-bar-group" role="group" aria-label="Chart models">
          <button
            type="button"
            className={chart.engines.has('glicko2_daneo') ? 'format-btn active' : 'format-btn'}
            aria-pressed={chart.engines.has('glicko2_daneo')}
            title="DANeo's multiplayer-weighted Glicko-2 baseline"
            onClick={() => toggleEngine('glicko2_daneo')}
          >
            👑 GlickoD
          </button>
          <button
            type="button"
            className={chart.engines.has('glicko2_std') ? 'format-btn active' : 'format-btn'}
            aria-pressed={chart.engines.has('glicko2_std')}
            title="Standard Glicko-2 model (unselected by default)"
            onClick={() => toggleEngine('glicko2_std')}
          >
            Glicko-2 Standard
          </button>
        </div>
      </OptionsSection>

      <OptionsSection title="Time Scale" tip="Perspective scale expands recent history with power-law recency acceleration, while Linear shows uniform spacing.">
        <Segmented label="Scale" options={SCALE_OPTIONS} value={chart.scale} onChange={setScale} />
      </OptionsSection>

      <OptionsSection title="Competitive Rating Tiers" tip="Ambient horizontal bands and dashed threshold markers indicating competitive rating tiers (Bronze to Supreme Grandmaster).">
        <Segmented label="Tiers" options={TIERS_OPTIONS} value={chart.tiers} onChange={setTiers} />
      </OptionsSection>

      <OptionsSection title="Chart Reset Mode" tip="Continuous career trajectory across all seasons, annual season reset rating recalculation, or all modes.">
        <Segmented label="Mode" options={MODE_OPTIONS} value={chart.mode} onChange={setMode} />
      </OptionsSection>

      <OptionsSection title="Rating Estimate Curves" tip="Expected rating mean (E = R), conservative 99.7% lower bound (C = μ − 3σ), or all curves.">
        <Segmented label="Curve" options={CURVE_OPTIONS} value={chart.curve} onChange={setCurve} />
      </OptionsSection>
    </OptionsPanel>
  );
}
