/** Upstream's multi-engine player comparison: a search, and each player's standing in every model. */
import { useEffect, useState } from 'react';
import { comparedPlayers, currentRun, leaderboard, type ComparedPlayer, type Format } from '@ttac/shared/ratings';
import { FORMAT_NAMES, parseMoversParams } from '../lib/analysis';
import { Flag } from './PlayerMarks';
import SearchBox, { type SearchPlayer } from './SearchBox';

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; players: ComparedPlayer[] };

const ROWS: Record<string, { engine: string; reset: string; className: string }> = {
  glicko2_daneo: { engine: '👑 GlickoD (Dynamic)', reset: 'Continuous', className: 'comp-daneo' },
  glicko2_daneo_softer: { engine: '👑 GlickoD (Dynamic)', reset: 'Season Reset', className: 'comp-daneo-reset' },
  glicko2_std: { engine: 'Glicko-2 Standard', reset: 'Continuous', className: '' },
  glicko2_std_softer: { engine: 'Glicko-2 Standard', reset: 'Season Reset', className: 'comp-reset' },
  glicko2_mp: { engine: 'Glicko-2 MP-Weighted', reset: 'Continuous', className: '' },
  glicko2_mp_softer: { engine: 'Glicko-2 MP-Weighted', reset: 'Season Reset', className: 'comp-reset' },
  glicko2_adapt: { engine: 'Glicko-2 Adaptive-T', reset: 'Continuous', className: '' },
  glicko2_adapt_softer: { engine: 'Glicko-2 Adaptive-T', reset: 'Season Reset', className: 'comp-reset' },
  whr: { engine: 'Whole-History Rating (WHR)', reset: 'Continuous', className: '' },
  whr_softer: { engine: 'Whole-History Rating (WHR)', reset: 'Season Reset', className: 'comp-reset' },
};

const FORMATS: Format[] = [0, 2, 3, 4];

function PlayerCard({ player, format }: { player: ComparedPlayer; format: Format }) {
  return (
    <div className="player-comp-card">
      <div className="player-comp-header">
        <div className="player-comp-name">
          {player.country !== null && <Flag country={player.country} />}
          <a href={`/ratings/player/?name=${encodeURIComponent(player.name)}`} title="Open Profile ↗">
            {player.name} ↗
          </a>
          {player.title !== null && <span className={`badge badge-${player.title.toLowerCase()}`}>{player.title}</span>}
        </div>
        <div className="player-comp-format">
          Format: <strong>{FORMAT_NAMES[format]}</strong>
        </div>
      </div>

      <div className="scroll-hint">
        <span>Swipe table horizontally for all engine comparisons ➔</span>
      </div>
      <div className="table-responsive">
        <table className="comp-table">
          <caption className="visually-hidden">{player.name}'s ratings in every engine and reset mode</caption>
          <thead>
            <tr>
              <th scope="col">Rating Engine &amp; Reset Mode</th>
              <th scope="col">Ladder Rank</th>
              <th scope="col">Conservative Rating (C)</th>
              <th scope="col">Expected Rating (E)</th>
              <th scope="col">Uncertainty (RD)</th>
              <th scope="col">Opponents</th>
              <th scope="col">Win Rate</th>
            </tr>
          </thead>
          <tbody>
            {player.ratings.length === 0 && (
              <tr>
                <td colSpan={7} className="comp-empty">
                  {player.name} has no rating in {FORMAT_NAMES[format]}: no rated games in this format.
                </td>
              </tr>
            )}
            {player.ratings.map((r) => {
              const row = ROWS[r.model] ?? { engine: r.model, reset: '', className: '' };
              return (
                <tr key={r.model} className={row.className || undefined}>
                  <td>
                    <span className="comp-engine">{row.engine}</span>
                    <span className="badge badge-rank comp-reset-badge">{row.reset}</span>
                  </td>
                  <td className="comp-rank">#{r.rank ?? 'None'}</td>
                  <td className="comp-c">{r.c_rating.toFixed(1)}</td>
                  <td className="comp-e">{r.rating.toFixed(1)}</td>
                  <td>±{r.rd.toFixed(1)}</td>
                  <td>{r.opponents}</td>
                  <td>{r.win_rate.toFixed(1)}%</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}

/** MoversPage shows the players the address searches for, or upstream's six, in the address's format. */
export default function MoversPage({ apiUrl }: { apiUrl: string }) {
  const [{ search, format }] = useState(() => parseMoversParams(new URLSearchParams(window.location.search)));
  const [load, setLoad] = useState<Load>({ kind: 'loading' });
  const [names, setNames] = useState<SearchPlayer[]>([]);

  useEffect(() => {
    currentRun(apiUrl)
      .then((r) => {
        leaderboard(apiUrl, r.run_id, 'glicko2_daneo', 0)
          .then((b) => setNames(b.players))
          .catch(() => setNames([]));
        return comparedPlayers(apiUrl, r.run_id, search, format);
      })
      .then((d) => setLoad({ kind: 'ready', players: d.players }))
      .catch((e: unknown) => setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
  }, [apiUrl, search, format]);

  return (
    <>
      <div className="movers-search-card">
        <form method="get" action="/ratings/analysis/movers/" className="movers-form">
          <div className="movers-search-field">
            <label htmlFor="movers-search">Search Player Name(s) (comma-separated):</label>
            <SearchBox players={names} value={search} name="search" inputClassName="" placeholder="e.g. Weidenbaum, a440, Martin_Pecheur" wide />
          </div>
          <div>
            <label htmlFor="movers-format">Tournament Format:</label>
            <select id="movers-format" name="format" defaultValue={String(format)}>
              {FORMATS.map((f) => (
                <option key={f} value={f}>
                  {FORMAT_NAMES[f]}
                </option>
              ))}
            </select>
          </div>
          <div className="movers-submit">
            <button type="submit" className="btn-antique movers-compare">
              🔍 Compare Ratings
            </button>
          </div>
        </form>
      </div>

      {load.kind === 'loading' && <p className="analysis-message">Loading players…</p>}
      {load.kind === 'error' && <p className="analysis-message">{load.message}</p>}
      {load.kind === 'ready' &&
        (load.players.length > 0 ? (
          load.players.map((p) => <PlayerCard key={p.name} player={p} format={format} />)
        ) : (
          <div className="movers-empty">No matching players found. Try searching for a different player name or adjusting your filters.</div>
        ))}
    </>
  );
}
