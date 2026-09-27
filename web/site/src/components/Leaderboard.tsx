/** The leaderboard for one model and format, read from the current run, in upstream's layout. */
import { useEffect, useState } from 'react';
import { formatValues, modelValues } from '@ttac/shared/api';
import { currentRun, leaderboard, type Format, type Leaderboard as Board, type Model } from '@ttac/shared/ratings';
import Segmented from './Segmented';
import * as show from '../lib/leaderboard-display';
import './Leaderboard.css';

type State = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; board: Board };

const formatOptions = formatValues.map((f) => ({ value: f, label: show.formatName(f) }));

/** Shows the chosen leaderboard, with the format bar and a model picker. */
export default function Leaderboard({ apiUrl }: { apiUrl: string }) {
  const [model, setModel] = useState<Model>('glicko2_daneo');
  const [format, setFormat] = useState<Format>(0);
  const [state, setState] = useState<State>({ kind: 'loading' });

  useEffect(() => {
    let stale = false;
    setState({ kind: 'loading' });
    currentRun(apiUrl)
      .then((run) => leaderboard(apiUrl, run.run_id, model, format))
      .then((board) => !stale && setState({ kind: 'ready', board }))
      .catch((e: unknown) => !stale && setState({ kind: 'error', message: e instanceof Error ? e.message : String(e) }));
    return () => {
      stale = true;
    };
  }, [apiUrl, model, format]);

  return (
    <div className="rating-card leaderboard">
      <div className="format-bar">
        <Segmented label="Format" options={formatOptions} value={format} onChange={setFormat} />
        <label className="model-picker">
          <span className="visually-hidden">Rating engine</span>
          <select className="input-antique" value={model} onChange={(e) => setModel(e.target.value as Model)}>
            {modelValues.map((m) => (
              <option key={m} value={m}>
                {show.modelLabel(m)}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className="leaderboard-heading">
        <div>
          <h1 className="rating-card-heading">Competitive Leaderboard</h1>
          <p className="leaderboard-engine">
            Engine: <strong>{show.modelLabel(model)}</strong>
            <span className="badge badge-format">{show.formatName(format)}</span>
          </p>
        </div>
        {state.kind === 'ready' && (
          <p className="leaderboard-count">
            Players: <strong>{state.board.players.length.toLocaleString('en')}</strong>{' '}
            <span className="leaderboard-status">(All Recorded)</span>
          </p>
        )}
      </div>

      {state.kind === 'loading' && (
        <p role="status" className="leaderboard-message">
          Loading…
        </p>
      )}
      {state.kind === 'error' && (
        <p role="alert" className="leaderboard-message">
          Could not load the leaderboard: {state.message}
        </p>
      )}
      {state.kind === 'ready' && <Table board={state.board} />}

      <section className="rating-card rating-card-note" aria-labelledby="leaderboard-guide">
        <h2 id="leaderboard-guide">Leaderboard Mechanics &amp; Rating Principles</h2>
        <p>
          Rankings are calculated dynamically across all recorded competitive Through the Ages tournaments. The primary
          sorting metric is the{' '}
          <strong>
            Conservative Rating <i>C</i> = <i>μ</i> − 3<i>σ</i>
          </strong>
          , guaranteeing that a player&apos;s standing represents an outcome that they can outperform 99.7% of the time.
        </p>
      </section>
    </div>
  );
}

const headers: [string, string][] = [
  ['Rank', 'Current competitive ranking based on Conservative Rating (C = μ - 3σ).'],
  ['Player', 'Player username.'],
  ['C-Rating', 'Conservative Rating: C = Rating - 3 × RD (99.7% confidence lower bound). Prevents unearned top spots with high uncertainty.'],
  ['Rating ± RD', 'Glicko-2 / WHR Skill Rating (mean μ) and Rating Deviation (RD / σ). Default starting rating is 1500 ± 350.'],
  ['Opps', 'Total competitive opponents faced across all recorded tournament games in this format.'],
  ['W-L-D', 'Total Wins, Losses, and Draws accumulated across competitive tournament matches.'],
  ['Win %', 'Effective win percentage: (Wins + 0.5 × Draws) / Opponents × 100.'],
  ['Last Played', "Date of player's most recent competitive match."],
];

/** The table of players, one row each, in rank order. */
function Table({ board }: { board: Board }) {
  return (
    <div className="table-scroll">
      <table className="leaderboard-table">
        <caption className="visually-hidden">
          {show.modelLabel(board.model)}, {show.formatName(board.format)}, run {board.run_id}
        </caption>
        <thead>
          <tr>
            {headers.map(([name, tip]) => (
              <th key={name} scope="col">
                <span data-tooltip={tip}>{name}</span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {board.players.map((p) => (
            <tr key={p.name}>
              <td className={show.isTopThree(p.rank) ? 'rank-col top-3-rank' : 'rank-col'}>{show.rank(p.rank)}</td>
              <th scope="row">
                <span className="player-name">{p.name}</span>
              </th>
              <td>
                <span className="c-rating-val">{show.cRating(p.c_rating)}</span>
              </td>
              <td>
                <span className="rating-primary">{show.rating(p.rating)}</span>{' '}
                <span className="rating-rd">{show.rd(p.rd)}</span>
              </td>
              <td>{p.opponents}</td>
              <td>
                <span className="wld-win">{p.wins}</span> - <span className="wld-loss">{p.losses}</span> -{' '}
                <span className="wld-draw">{p.draws}</span>
              </td>
              <td>
                <strong>{show.winRate(p.win_rate)}</strong>
              </td>
              <td className="last-played">{p.last_played ?? '-'}</td>
            </tr>
          ))}
          {board.players.length === 0 && (
            <tr>
              <td colSpan={headers.length} className="empty">
                No players found matching your criteria.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
