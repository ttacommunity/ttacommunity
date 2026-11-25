/** The filters above a games table: the page's own choices first, then a player and games with a replay code. */
import type { ReactNode } from 'react';
import PlayerPicker from './PlayerPicker';

/** GamesFilterBar shows children (the page's own choices), a player picker over players and the replay-code box, reporting each change. */
export default function GamesFilterBar({ players, player, onPlayer, coded, onCoded, children }: {
  players: string[];
  player: string;
  onPlayer: (player: string) => void;
  coded: boolean;
  onCoded: (coded: boolean) => void;
  children?: ReactNode;
}) {
  return (
    <div className="games-filters">
      {children}
      <PlayerPicker players={players} value={player} onPick={onPlayer} />
      <label className="games-coded check-inline">
        <input type="checkbox" checked={coded} onChange={(e) => onCoded(e.target.checked)} />
        <span>Only games with a replay code</span>
      </label>
    </div>
  );
}
