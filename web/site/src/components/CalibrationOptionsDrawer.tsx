/** Options & filters drawer for the model calibration page: the evaluation mode and which engines' and reset modes' curves are drawn. */
import Segmented, { type Option } from '@ttac/shared/Segmented';
import ToggleGroup from '@ttac/shared/ToggleGroup';
import type { EvalMode, Family, Reset } from '../lib/analysis';
import { OptionsPanel, OptionsSection } from './SideDrawer';

const MODE_OPTIONS: Option<EvalMode>[] = [
  { value: 'standard', label: 'Standard', tooltip: 'Standard match-time in-sample evaluation' },
  { value: 'walk_forward', label: 'Walk-Forward', tooltip: 'Strict out-of-sample monthly freeze testing' },
];

const ENGINE_OPTIONS: Option<Family>[] = [
  { value: 'glicko2_daneo', label: '👑 GlickoD' },
  { value: 'glicko2_std', label: 'G2 Std' },
];

const RESET_OPTIONS: Option<Reset>[] = [
  { value: 'continuous', label: 'Continuous' },
  { value: 'softer', label: 'Season Reset' },
];

interface Props {
  mode: EvalMode;
  engines: ReadonlySet<Family>;
  resets: ReadonlySet<Reset>;
  /** How many curves the choices draw. */
  curves: number;
  setMode: (mode: EvalMode) => void;
  setEngines: (engines: Set<Family>) => void;
  setResets: (resets: Set<Reset>) => void;
  onReset: () => void;
  onClose: () => void;
}

/** CalibrationOptionsDrawer chooses what the calibration chart and table compare. */
export default function CalibrationOptionsDrawer({ mode, engines, resets, curves, setMode, setEngines, setResets, onReset, onClose }: Props) {
  return (
    <OptionsPanel
      label="Calibration options"
      onClose={onClose}
      reset={onReset}
      summary={
        <>
          Active Curves: <strong>{curves}</strong>
        </>
      }
    >
      <OptionsSection icon="🔮" title="Evaluation Mode">
        <Segmented label="Evaluation mode" options={MODE_OPTIONS} value={mode} onChange={setMode} />
      </OptionsSection>

      <OptionsSection icon="🏅" title="Rating Engine Family">
        <ToggleGroup label="Rating engine family" all="All Engines" options={ENGINE_OPTIONS} chosen={engines} onChange={setEngines} />
      </OptionsSection>

      <OptionsSection icon="↻" title="Season Reset Mode">
        <ToggleGroup label="Season reset mode" all="All Resets" options={RESET_OPTIONS} chosen={resets} onChange={setResets} />
      </OptionsSection>
    </OptionsPanel>
  );
}
