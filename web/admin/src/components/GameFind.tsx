/** Finds the game a change removes or replaces by its replay code, and shows the game found, with its players. */
import { useState, type KeyboardEvent } from 'react';
import * as req from '../lib/admin-requests';
import { foundLine, type ChangeableGame } from '../lib/game-changes';

interface Props {
  found: ChangeableGame | null;
  /** onFound is given the game found, or null while none is. */
  onFound: (game: ChangeableGame | null) => void;
  /** onFailed is given why the game could not be found. */
  onFailed: (message: string) => void;
}

export default function GameFind({ found, onFound, onFailed }: Props) {
  const [code, setCode] = useState('');
  const find = async () => {
    onFound(null);
    const r = await req.send<ChangeableGame>(req.gameByCode(code));
    if (r.ok) onFound(r.data);
    else onFailed(r.message);
  };
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== 'Enter') return;
    e.preventDefault();
    void find();
  };

  return (
    <>
      <div className="field-row">
        <label>
          Its replay code
          <input className="input-antique" value={code} maxLength={64} onChange={(e) => setCode(e.target.value)} onKeyDown={onKeyDown} />
        </label>
        <button type="button" className="btn-antique" disabled={code.trim() === ''} onClick={() => void find()}>
          Find
        </button>
      </div>
      {found !== null && (
        <p>
          <strong>{found.name}</strong>, {foundLine(found)}
        </p>
      )}
    </>
  );
}
