/** Upstream's Options & Filters drawer: engine, reset mode, prior, and the status, title and opponent filters. */
import type { TitleCode, YearEntry } from '@ttac/shared/ratings';
import { RESET_MODES, engineName, resetName } from '../lib/leaderboard-display';
import { DEFAULT_VIEW, ENGINES, defaultMinOpps, resetApplies, withRanking, type DeltaWindow, type Engine, type Status, type View } from '../lib/leaderboard-view';
import Segmented from '@ttac/shared/Segmented';
import MinOppsField from './MinOppsField';
import TitleSelect from './TitleSelect';
import { OptionsPanel, OptionsSection } from './SideDrawer';
import type { Link } from '../lib/navigate';
import { TRACKS } from '@ttac/shared/tracks';

/** A link to a view: a real address, followed in place. */
export type LinkTo = (view: View) => Link;

const engineTips: Record<Engine, string> = {
  glicko2_daneo: '👑 GlickoD: φ^(N-2) multiplayer decomposition and 15-game emergent prior re-calibration.',
  glicko2_std: 'Standard naive 1v1 pairwise baseline without multiplayer calibration.',
};

/** The CSV downloads of the table as shown, and of every player, with how many rows the first has. */
export interface Downloads {
  shown: string;
  everyone: string;
  count: number;
}

interface Props {
  view: View;
  titles: TitleCode[];
  years: YearEntry[] | null;
  downloads: Downloads | null;
  linkTo: LinkTo;
  go: (view: View) => void;
  onClose: () => void;
}

/** archiveView is where upstream's archive pills go: the year, keeping the model and format, other filters reset. */
function archiveView(view: View, year: number | null): View {
  return { ...DEFAULT_VIEW, ranking: view.ranking, engine: view.engine, reset: view.reset, format: view.format, year, minOpps: defaultMinOpps(year, view.ranking) };
}

/** OptionsDrawer shows every option; choosing one goes to that view at page 1. */
export default function OptionsDrawer({ view, titles, years, downloads, linkTo, go, onClose }: Props) {
  const at = (change: Partial<View>) => ({ ...view, ...change, page: 1 });
  const shownReset = resetName(view.reset);

  return (
    <OptionsPanel
      label="Options and filters"
      onClose={onClose}
      reset={linkTo({ ...DEFAULT_VIEW, format: view.format })}
      summary={
        <>
          Active: <strong>{engineName(view.engine)}</strong> &bull; <strong>{shownReset}</strong>
        </>
      }
    >
      <OptionsSection icon="📅" title="Season Archive">
        <div className="archive-pills">
          <a
            {...linkTo(view.year === null ? { ...view, page: 1 } : archiveView(view, null))}
            className={view.year === null ? 'year-pill archive-live active' : 'year-pill archive-live'}
            title="Current live ratings across all competitive tournament history"
          >
            Live / All-Time
          </a>
          {(years ?? []).map((y, i) => (
            <a
              key={y.year}
              {...linkTo(archiveView(view, y.year))}
              className={view.year === y.year ? 'year-pill archive-year active' : 'year-pill archive-year'}
              title={`Historical standings at end of Season ${y.year}`}
            >
              {y.year}
              {i === 0 ? (
                <span className="year-pill-wc year-pill-current">{String(y.year).slice(-2)}</span>
              ) : (
                y.champion && <span className="year-pill-wc">WC</span>
              )}
            </a>
          ))}
        </div>
      </OptionsSection>

      <OptionsSection icon="🎯" title="Rating Track">
        <Segmented
          label="Rating track"
          stacked
          options={TRACKS.map((t) => ({
            value: t.value,
            label: (
              <>
                <span>{t.name}</span>
                {t.value === DEFAULT_VIEW.ranking && <span className="status">Default</span>}
              </>
            ),
            tooltip: defaultMinOpps(null, t.value) === 0 ? `${t.desc} (sets min opponents to 0)` : t.desc,
          }))}
          value={view.ranking}
          link={(ranking) => linkTo(withRanking(view, ranking))}
        />
      </OptionsSection>

      <OptionsSection icon="⚡" title="Rating Engine">
        <Segmented
          label="Rating engine"
          stacked
          options={ENGINES.map((e) => ({
            value: e,
            label: (
              <>
                <span>{engineName(e)}</span>
                {e === DEFAULT_VIEW.engine && <span className="status">Default</span>}
              </>
            ),
            tooltip: engineTips[e],
          }))}
          value={view.engine}
          link={(engine) => linkTo(at({ engine }))}
        />
      </OptionsSection>

      {resetApplies(view.engine) && (
        <OptionsSection icon="⚙️" title="Season Reset Mode">
          <Segmented
            label="Season reset mode"
            options={RESET_MODES.map((m) => ({ value: m.value, label: m.name, tooltip: m.desc }))}
            value={view.reset}
            link={(reset) => linkTo(at({ reset }))}
          />
        </OptionsSection>
      )}

      <OptionsSection icon="👤" title="Activity Status" htmlFor="options-status">
        <select
          id="options-status"
          className="input-antique"
          value={view.status}
          onChange={(e) => go(at({ status: e.target.value as Status }))}
        >
          <option value="active">Active Only (Standard)</option>
          <option value="all">Include Inactive (All)</option>
          <option value="inactive">Inactive Only (Retired)</option>
        </select>
      </OptionsSection>

      <OptionsSection icon="🏅" title="Titles Filter" htmlFor="options-title">
        <TitleSelect id="options-title" titles={titles} value={view.title} onChange={(title) => go(at({ title }))} />
      </OptionsSection>

      <OptionsSection icon="⏱️" title="Rating Deltas" htmlFor="options-delta">
        <select
          id="options-delta"
          className="input-antique"
          value={view.delta}
          onChange={(e) => go(at({ delta: e.target.value as DeltaWindow }))}
        >
          <option value="none">Deltas: Off</option>
          <option value="last_update">&Delta; Last Webmaster Update</option>
          <option value="month">&Delta; Last Month (30d)</option>
          <option value="quarter">&Delta; Last Quarter (90d)</option>
          <option value="year">&Delta; Last Year (365d)</option>
        </select>
      </OptionsSection>

      <OptionsSection icon="⚔️" title="Minimum Opponents" htmlFor="options-min">
        <MinOppsField id="options-min" value={view.minOpps} onApply={(minOpps) => go(at({ minOpps }))} />
      </OptionsSection>

      {downloads && (
        <OptionsSection icon="📥" title="Tournament Data Export" aside="CSV FILE">
          <a href={downloads.shown} className="btn-antique download" download title="Download current filtered rating table as a CSV file">
            <span>📥 Download currently selected ratings</span>
          </a>
          <div className="options-export-note">
            <span>Includes active filters ({downloads.count} players)</span>
            {view.status !== 'all' && (
              <a href={downloads.everyone} download title="Download full rating database including inactive players">
                Download all (incl. inactive) &raquo;
              </a>
            )}
          </div>
        </OptionsSection>
      )}
    </OptionsPanel>
  );
}
