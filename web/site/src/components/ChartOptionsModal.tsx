/** Modal dialog for trajectory chart options: scale, rating tiers, reset modes, curves, and engines. */
import { useRef } from 'react';
import Segmented, { type Option } from '@ttac/shared/Segmented';
import { useModal, type Opener } from '@ttac/shared/modal';
import type { ChartCurve, ChartMode, ChartScale, ChartTiers } from '../lib/player-view';

export interface ChartOptionsModalProps {
  opener: Opener | null;
  scale: ChartScale;
  setScale: (s: ChartScale) => void;
  tiers: ChartTiers;
  setTiers: (t: ChartTiers) => void;
  mode: ChartMode;
  setMode: (m: ChartMode) => void;
  curve: ChartCurve;
  setCurve: (c: ChartCurve) => void;
  engines: ReadonlySet<string>;
  toggleEngine: (engine: string) => void;
  onReset: () => void;
  onClose: () => void;
}

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

export default function ChartOptionsModal({
  opener,
  scale,
  setScale,
  tiers,
  setTiers,
  mode,
  setMode,
  curve,
  setCurve,
  engines,
  toggleEngine,
  onReset,
  onClose,
}: ChartOptionsModalProps) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const closed = useModal(dialogRef, opener, onClose, { backdropCloses: true });

  return (
    <dialog ref={dialogRef} className="chart-options-modal site-dialog" aria-labelledby="chart-options-title" onClose={closed}>
      <div className="options-header">
        <span id="chart-options-title" className="options-header-title">
          ⚙️ Trajectory Chart Options
        </span>
        <button type="button" className="btn-options-close" title="Close options" onClick={() => dialogRef.current?.close()}>
          ✕ Close
        </button>
      </div>

      <div className="chart-options-body">
        <div className="options-section">
          <div className="options-group-title">Rating Models &amp; Engines</div>
          <div className="options-section-desc">
            Toggle individual model curves on the chart. GlickoD is the multiplayer-weighted baseline; Standard is unselected by default.
          </div>
          <div className="options-engine-toggles">
            <button
              type="button"
              className={['opt-engine-btn', engines.has('glicko2_daneo') && 'active'].filter(Boolean).join(' ')}
              onClick={() => toggleEngine('glicko2_daneo')}
            >
              👑 GlickoD {engines.has('glicko2_daneo') ? '✓' : ''}
            </button>
            <button
              type="button"
              className={['opt-engine-btn', engines.has('glicko2_std') && 'active'].filter(Boolean).join(' ')}
              onClick={() => toggleEngine('glicko2_std')}
            >
              Glicko-2 Standard {engines.has('glicko2_std') ? '✓' : ''}
            </button>
          </div>
        </div>

        <div className="options-section">
          <div className="options-group-title">Time Scale</div>
          <div className="options-section-desc">
            Perspective scale expands recent history with power-law recency acceleration, while Linear shows uniform spacing.
          </div>
          <Segmented label="Scale" look="raised" options={SCALE_OPTIONS} value={scale} onChange={setScale} />
        </div>

        <div className="options-section">
          <div className="options-group-title">Competitive Rating Tiers</div>
          <div className="options-section-desc">
            Ambient horizontal bands and dashed threshold markers indicating competitive rating tiers (Bronze to Supreme Grandmaster).
          </div>
          <Segmented label="Tiers" look="raised" options={TIERS_OPTIONS} value={tiers} onChange={setTiers} />
        </div>

        <div className="options-section">
          <div className="options-group-title">Season Reset Mode</div>
          <div className="options-section-desc">
            Continuous career trajectory across all seasons, annual season reset rating recalculation, or all modes.
          </div>
          <Segmented label="Mode" look="raised" options={MODE_OPTIONS} value={mode} onChange={setMode} />
        </div>

        <div className="options-section">
          <div className="options-group-title">Rating Estimate Curves</div>
          <div className="options-section-desc">
            Expected rating mean (E = R), conservative 99.7% lower bound (C = μ − 3σ), or all curves.
          </div>
          <Segmented label="Curve" look="raised" options={CURVE_OPTIONS} value={curve} onChange={setCurve} />
        </div>
      </div>

      <div className="chart-options-footer">
        <button type="button" className="btn-options-reset" onClick={onReset}>
          Reset to Defaults
        </button>
        <button type="button" className="btn-options-done" onClick={() => dialogRef.current?.close()}>
          Done
        </button>
      </div>
    </dialog>
  );
}
