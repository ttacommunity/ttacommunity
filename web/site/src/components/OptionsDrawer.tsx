/** Upstream's Options & Filters drawer: engine, reset mode, prior, and the status, title and opponent filters. */
import { useEffect, useState } from 'react';
import type { TitleCode, YearEntry } from '@ttac/shared/ratings';
import { engineName } from '../lib/leaderboard-display';
import { DEFAULT_VIEW, ENGINES, defaultMinOpps, resetApplies, type DeltaWindow, type Engine, type ResetMode, type Status, type View } from '../lib/leaderboard-view';
import Segmented from '@ttac/shared/Segmented';

/** A link to a view: a real address, followed in place. */
export type LinkTo = (view: View) => { href: string; onClick: (e: React.MouseEvent) => void };

const engineTips: Record<Engine, string> = {
  glicko2_daneo: '👑 GlickoD: φ^(N-2) multiplayer decomposition and 15-game emergent prior re-calibration.',
  glicko2_std: 'Standard naive 1v1 pairwise baseline without multiplayer calibration.',
};

const resetModes: [ResetMode, string, string][] = [
  ['continuous', 'Continuous (Career)', 'Standard uninterrupted career rating trajectory across all tournament history'],
  ['softer', 'Season Reset', 'Gentle annual reset balancing career achievement with current season form'],
];

const titleLabels: Partial<Record<TitleCode, string>> = { GM: 'Grandmaster (GM)', M: 'Master (M)', P: 'Platinum (P)', G: 'Gold (G)' };

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
  return { ...DEFAULT_VIEW, engine: view.engine, reset: view.reset, format: view.format, year, minOpps: defaultMinOpps(year) };
}

/** OptionsDrawer shows every option; choosing one goes to that view at page 1. */
export default function OptionsDrawer({ view, titles, years, downloads, linkTo, go, onClose }: Props) {
  const [minOpps, setMinOpps] = useState(String(view.minOpps));
  useEffect(() => setMinOpps(String(view.minOpps)), [view.minOpps]);
  const at = (change: Partial<View>) => ({ ...view, ...change, page: 1 });
  const resetName = resetModes.find(([id]) => id === view.reset)?.[1];

  return (
    <div className="leaderboard-options-panel" role="dialog" aria-label="Options and filters">
      <div className="options-header">
        <span className="options-header-title">⚙️ Options &amp; Filters</span>
        <button type="button" className="btn-options-close" title="Close options panel" onClick={onClose}>
          ✕ Close
        </button>
      </div>

      <div>
        <div className="options-group-title">
          <span>📅 Season Archive</span>
        </div>
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
      </div>

      <div>
        <div className="options-group-title">
          <span>⚡ Rating Engine</span>
        </div>
        <Segmented
          label="Rating engine"
          look="banner"
          groupClass="reset-btn-group stacked"
          options={ENGINES.map((e) => ({
            value: e,
            label: (
              <>
                <span>{engineName(e)}</span>
                {e === DEFAULT_VIEW.engine && <span className="default-tag">DEFAULT</span>}
              </>
            ),
            tooltip: engineTips[e],
          }))}
          value={view.engine}
          link={(engine) => linkTo(at({ engine }))}
        />
      </div>

      {resetApplies(view.engine) && (
        <div>
          <div className="options-group-title">
            <span>⚙️ Season Reset Mode</span>
          </div>
          <Segmented
            label="Season reset mode"
            look="banner"
            options={resetModes.map(([id, name, tip]) => ({ value: id, label: name, tooltip: tip }))}
            value={view.reset}
            link={(reset) => linkTo(at({ reset }))}
          />
        </div>
      )}

      <form
        onSubmit={(e) => {
          e.preventDefault();
          const n = Number(minOpps);
          if (Number.isInteger(n) && n >= 0 && n <= 1000) go(at({ minOpps: n }));
        }}
      >
        <div className="options-field">
          <label className="options-group-title" htmlFor="options-status">
            <span>👤 Activity Status</span>
          </label>
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
        </div>

        <div className="options-field">
          <label className="options-group-title" htmlFor="options-title">
            <span>🏅 Titles Filter</span>
          </label>
          <select
            id="options-title"
            className="input-antique"
            value={view.title ?? 'ALL'}
            onChange={(e) => go(at({ title: e.target.value === 'ALL' ? null : (e.target.value as TitleCode) }))}
          >
            <option value="ALL">All Titles</option>
            {titles.map((t) => (
              <option key={t} value={t}>
                {titleLabels[t] ?? t}
              </option>
            ))}
          </select>
        </div>

        <div className="options-field">
          <label className="options-group-title" htmlFor="options-delta">
            <span>⏱️ Rating Deltas</span>
          </label>
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
        </div>

        <div className="options-field">
          <label className="options-group-title" htmlFor="options-min">
            <span>⚔️ Minimum Opponents</span>
          </label>
          <div className="options-min-row">
            <input
              id="options-min"
              type="number"
              min={0}
              max={1000}
              className="input-antique"
              placeholder="Min opponents"
              value={minOpps}
              onChange={(e) => setMinOpps(e.target.value)}
            />
            <button type="submit" className="btn-antique">
              Apply
            </button>
          </div>
        </div>
      </form>

      {downloads && (
        <div className="options-export">
          <div className="options-group-title options-export-title">
            <span>📥 Tournament Data Export</span>
            <span className="options-export-kind">CSV FILE</span>
          </div>
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
        </div>
      )}

      <div className="options-footer">
        <div className="options-footer-active">
          Active: <strong>{engineName(view.engine)}</strong> &bull; <strong>{resetName}</strong>
        </div>
        <div className="options-footer-actions">
          <a {...linkTo({ ...DEFAULT_VIEW, format: view.format })} className="btn-reset reset-all">
            Reset All
          </a>
          <button type="button" className="btn-options-close" title="Close options panel" onClick={onClose}>
            ✕ Close
          </button>
        </div>
      </div>
    </div>
  );
}
