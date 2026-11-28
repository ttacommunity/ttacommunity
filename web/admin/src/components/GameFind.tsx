/** Finds the game a change removes or replaces by its replay code, and shows the game found, with its players. */
import { useState } from 'react';
import GameCodeField from '@ttac/shared/GameCodeField';
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

  return (
    <>
      <div className="field-row">
        <GameCodeField label="Its replay code" value={code} onChange={setCode} onEnter={() => void find()} />
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
