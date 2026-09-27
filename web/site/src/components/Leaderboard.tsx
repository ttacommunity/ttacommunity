/** The leaderboard for one model and format, read from the current run. */
import { useEffect, useState } from 'react';
import { formatValues, modelValues } from '@ttac/shared/api';
import { currentRun, leaderboard, type Format, type Leaderboard as Board, type Model } from '@ttac/shared/ratings';
import './Leaderboard.css';

const formatNames: Record<Format, string> = { 0: 'All formats', 2: '2-player', 3: '3-player', 4: '4-player' };

type State = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; board: Board };

/** Shows the chosen leaderboard, with pickers for model and format. */
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
    <section className="leaderboard">
      <form className="pickers" onSubmit={(e) => e.preventDefault()}>
        <label>
          Model{' '}
          <select value={model} onChange={(e) => setModel(e.target.value as Model)}>
            {modelValues.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
        </label>
        <label>
          Format{' '}
          <select value={format} onChange={(e) => setFormat(Number(e.target.value) as Format)}>
            {formatValues.map((f) => (
              <option key={f} value={f}>
                {formatNames[f]}
              </option>
            ))}
          </select>
        </label>
      </form>

      {state.kind === 'loading' && <p role="status">Loading…</p>}
      {state.kind === 'error' && <p role="alert">Could not load the leaderboard: {state.message}</p>}
      {state.kind === 'ready' && (
        <table>
          <caption>
            {state.board.model}, {formatNames[state.board.format]}: {state.board.players.length} players, run{' '}
            {state.board.run_id}
          </caption>
          <thead>
            <tr>
              <th scope="col">Rank</th>
              <th scope="col">Δ</th>
              <th scope="col">Player</th>
              <th scope="col">Rating</th>
              <th scope="col">RD</th>
              <th scope="col">Conservative</th>
              <th scope="col">Games</th>
              <th scope="col">Opponents</th>
              <th scope="col">W</th>
              <th scope="col">L</th>
              <th scope="col">D</th>
              <th scope="col">Win %</th>
              <th scope="col">Last played</th>
            </tr>
          </thead>
          <tbody>
            {state.board.players.map((p) => (
              <tr key={p.name}>
                <td>{p.rank ?? '–'}</td>
                <td>{p.rank_delta}</td>
                <th scope="row">{p.name}</th>
                <td>{p.rating.toFixed(0)}</td>
                <td>{p.rd.toFixed(0)}</td>
                <td>{p.c_rating.toFixed(0)}</td>
                <td>{p.games}</td>
                <td>{p.opponents}</td>
                <td>{p.wins}</td>
                <td>{p.losses}</td>
                <td>{p.draws}</td>
                <td>{p.win_rate.toFixed(1)}</td>
                <td>{p.last_played ?? '–'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
