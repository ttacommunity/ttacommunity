/** Finds the game a change removes or replaces: by its replay code, or, for a game with none, from its group's games. Shows the game found, with its players. */
import { useState, type KeyboardEvent } from 'react';
import * as req from '../lib/admin-requests';
import { foundLine, gameChoices, type ChangeableGame } from '../lib/game-changes';
import GroupPick from './GroupPick';

interface Props {
  found: ChangeableGame | null;
  /** onFound is given the game found, or null while none is. */
  onFound: (game: ChangeableGame | null) => void;
  /** onFailed is given why the game could not be found. */
  onFailed: (message: string) => void;
}

export default function GameFind({ found, onFound, onFailed }: Props) {
  const [code, setCode] = useState('');
  const [games, setGames] = useState<ChangeableGame[]>([]);
  const byCode = async () => {
    onFound(null);
    const r = await req.send<ChangeableGame[]>(req.gameByCode(code));
    if (!r.ok) return onFailed(r.message);
    if (r.data.length === 0) return onFailed(`No game we hold has the code ${code.trim()}.`);
    onFound(r.data[0]);
  };
  const ofGroup = async (group: number | null) => {
    setGames([]);
    onFound(null);
    if (group === null) return;
    const r = await req.send<ChangeableGame[]>(req.gamesOfGroup(group));
    if (r.ok) setGames(r.data);
    else onFailed(r.message);
  };
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== 'Enter') return;
    e.preventDefault();
    void byCode();
  };

  return (
    <>
      <div className="field-row">
        <label>
          Its replay code
          <input className="input-antique" value={code} maxLength={64} onChange={(e) => setCode(e.target.value)} onKeyDown={onKeyDown} />
        </label>
        <button type="button" className="btn-antique" disabled={code.trim() === ''} onClick={() => void byCode()}>
          Find
        </button>
      </div>
      <details>
        <summary>No replay code? Choose it from its group</summary>
        <GroupPick onPick={(g) => void ofGroup(g?.id ?? null)} onFailed={onFailed} />
        {games.length > 0 && (
          <div className="field-row">
            <label>
              Game
              <select className="input-antique" value={found?.id ?? ''} onChange={(e) => onFound(games.find((g) => String(g.id) === e.target.value) ?? null)}>
                <option value="">Choose…</option>
                {gameChoices(games).map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.label}
                  </option>
                ))}
              </select>
            </label>
          </div>
        )}
      </details>
      {found !== null && (
        <p>
          <strong>{found.name}</strong>, {foundLine(found)}
        </p>
      )}
    </>
  );
}
