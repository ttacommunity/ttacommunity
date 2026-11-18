/** Options & filters drawer for the rating percentiles page. */
import type { RatingBasis, TitleCode } from '@ttac/shared/ratings';
import Segmented from '@ttac/shared/Segmented';
import * as show from '../lib/leaderboard-display';
import { ENGINES, FORMATS, type Status } from '../lib/leaderboard-view';
import type { Link } from '../lib/navigate';
import { BIN_WIDTHS, withDefaults, type PercentileView } from '../lib/percentiles';
import MinOppsField from './MinOppsField';
import { OptionsPanel, OptionsSection } from './SideDrawer';
import TitleSelect from './TitleSelect';

const STATUS_OPTIONS: { value: Status; label: string; tooltip: string }[] = [
  { value: 'active', label: 'Active', tooltip: "Players with a game in the 12 months before the run's last game" },
  { value: 'all', label: 'All', tooltip: 'Every rated player' },
  { value: 'inactive', label: 'Inactive', tooltip: 'Players without a game in those 12 months' },
];

const RATING_OPTIONS: { value: RatingBasis; label: string; tooltip: string }[] = [
  { value: 'c_rating', label: 'Conservative', tooltip: 'C-Rating: rating minus three times its deviation, as the leaderboard ranks' },
  { value: 'rating', label: 'Plain', tooltip: 'The rating itself, however uncertain' },
];

interface Props {
  view: PercentileView;
  titles: TitleCode[];
  linkTo: (next: PercentileView) => Link;
  go: (next: PercentileView) => void;
  onClose: () => void;
}

/** PercentilesOptionsDrawer holds all format, model, population, and binning filters for the percentiles page. */
export default function PercentilesOptionsDrawer({ view, titles, linkTo, go, onClose }: Props) {
  const board = view.board;
  const at = (change: Partial<PercentileView['board']>) => ({ ...view, board: { ...board, ...change } });

  return (
    <OptionsPanel label="Percentiles options and filters" onClose={onClose} reset={linkTo(withDefaults(view))}>
      <OptionsSection icon="🎮" title="Format">
        <Segmented
          label="Format"
          options={FORMATS.map((f) => ({ value: f, label: show.formatName(f), tooltip: `View rating spread for ${show.formatName(f)}` }))}
          value={board.format}
          link={(format) => linkTo(at({ format }))}
        />
      </OptionsSection>

      <OptionsSection icon="⚡" title="Rating Engine">
        <Segmented
          label="Rating engine"
          options={ENGINES.map((e) => ({ value: e, label: show.engineName(e) }))}
          value={board.engine}
          link={(engine) => linkTo(at({ engine }))}
        />
      </OptionsSection>

      <OptionsSection icon="⚙️" title="Season Reset Mode">
        <Segmented
          label="Season reset mode"
          options={show.RESET_MODES.map((m) => ({ value: m.value, label: m.name, tooltip: m.desc }))}
          value={board.reset}
          link={(reset) => linkTo(at({ reset }))}
        />
      </OptionsSection>

      <OptionsSection icon="👤" title="Activity Status">
        <Segmented label="Activity status" options={STATUS_OPTIONS} value={board.status} link={(status) => linkTo(at({ status }))} />
      </OptionsSection>

      <OptionsSection icon="📏" title="Rating Counted">
        <Segmented label="Rating counted" options={RATING_OPTIONS} value={view.rating} link={(rating) => linkTo({ ...view, rating })} />
      </OptionsSection>

      <OptionsSection icon="📊" title="Bin Width">
        <Segmented
          label="Bin width"
          options={BIN_WIDTHS.map((w) => ({ value: w, label: String(w), tooltip: `${w}-point bins` }))}
          value={view.binWidth}
          link={(binWidth) => linkTo({ ...view, binWidth })}
        />
      </OptionsSection>

      <OptionsSection icon="🏅" title="Titles Filter" htmlFor="percentiles-title">
        <TitleSelect id="percentiles-title" titles={titles} value={board.title} onChange={(title) => go(at({ title }))} />
      </OptionsSection>

      <OptionsSection icon="⚔️" title="Minimum Opponents" htmlFor="percentiles-min">
        <MinOppsField id="percentiles-min" value={board.minOpps} onApply={(minOpps) => go(at({ minOpps }))} />
      </OptionsSection>
    </OptionsPanel>
  );
}
