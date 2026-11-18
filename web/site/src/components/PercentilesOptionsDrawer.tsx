/** Options & filters drawer for the rating percentiles page. */
import type { RatingBasis, TitleCode } from '@ttac/shared/ratings';
import Segmented from '@ttac/shared/Segmented';
import * as show from '../lib/leaderboard-display';
import { ENGINES, FORMATS, type Status } from '../lib/leaderboard-view';
import type { Link } from '../lib/navigate';
import { BIN_WIDTHS, type PercentileView } from '../lib/percentiles';
import MinOppsField from './MinOppsField';
import SideDrawer from './SideDrawer';
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

  return (
    <SideDrawer className="player-sidebar" onClose={onClose}>
      <div className="leaderboard-options-panel chart-options-panel" role="dialog" aria-label="Percentiles options and filters">
        <div className="options-header">
          <span className="options-header-title">⚙️ Options &amp; Filters</span>
          <button type="button" className="btn-options-close" title="Close options panel" onClick={onClose}>
            ✕ Close
          </button>
        </div>

        <div>
          <div className="options-group-title">
            <span>🎮 Format</span>
          </div>
          <Segmented
            label="Format"
            groupClass="view-group view-group-format"
            options={FORMATS.map((f) => ({ value: f, label: show.formatName(f), tooltip: `View rating spread for ${show.formatName(f)}` }))}
            value={board.format}
            link={(format) => linkTo({ ...view, board: { ...board, format } })}
          />
        </div>

        <div>
          <div className="options-group-title">
            <span>🔄 Reset Mode</span>
          </div>
          <Segmented
            label="Reset mode"
            groupClass="view-group view-group-reset"
            options={show.RESET_MODES.map((m) => ({ value: m.value, label: m.name, tooltip: m.desc }))}
            value={board.reset}
            link={(reset) => linkTo({ ...view, board: { ...board, reset } })}
          />
        </div>

        <div>
          <div className="options-group-title">
            <span>🧠 Engine</span>
          </div>
          <Segmented
            label="Engine"
            groupClass="view-group"
            options={ENGINES.map((e) => ({ value: e, label: show.engineName(e) }))}
            value={board.engine}
            link={(engine) => linkTo({ ...view, board: { ...board, engine } })}
          />
        </div>

        <div>
          <div className="options-group-title">
            <span>👥 Players</span>
          </div>
          <Segmented
            label="Players counted"
            groupClass="view-group"
            options={STATUS_OPTIONS}
            value={board.status}
            link={(status) => linkTo({ ...view, board: { ...board, status } })}
          />
        </div>

        <div>
          <div className="options-group-title">
            <span>📏 Rating &amp; Bins</span>
          </div>
          <div className="options-stack">
            <Segmented
              label="Rating counted"
              groupClass="view-group"
              prefix={<span className="view-bar-label">Rating:</span>}
              options={RATING_OPTIONS}
              value={view.rating}
              link={(rating) => linkTo({ ...view, rating })}
            />
            <Segmented
              label="Bin width"
              groupClass="view-group"
              prefix={<span className="view-bar-label">Bins:</span>}
              options={BIN_WIDTHS.map((w) => ({ value: w, label: String(w), tooltip: `${w}-point bins` }))}
              value={view.binWidth}
              link={(binWidth) => linkTo({ ...view, binWidth })}
            />
          </div>
        </div>

        <div>
          <div className="options-group-title">
            <span>🏅 Filter</span>
          </div>
          <div className="options-stack">
            <div className="view-group">
              <label className="view-bar-label" htmlFor="percentiles-title">
                Title:
              </label>
              <TitleSelect
                id="percentiles-title"
                titles={titles}
                value={board.title}
                onChange={(title) => go({ ...view, board: { ...board, title } })}
              />
            </div>
            <div className="view-group">
              <label className="view-bar-label" htmlFor="percentiles-min">
                Min opponents:
              </label>
              <MinOppsField id="percentiles-min" value={board.minOpps} onApply={(minOpps) => go({ ...view, board: { ...board, minOpps } })} />
            </div>
          </div>
        </div>
      </div>
    </SideDrawer>
  );
}
