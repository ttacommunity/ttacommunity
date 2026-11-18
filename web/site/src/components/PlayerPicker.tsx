/** A games list's player filter: a name typed in full, the top suggestion for part of one, or a suggestion picked. */
import { useMemo, useState } from 'react';
import { pickPlayer } from '../lib/leaderboard-view';
import SearchBox from './SearchBox';
import './GamesTable.css';

/** PlayerPicker narrows a games list to one of players: onPick gets the name, or '' when the search is cleared; onFocus is for a page that loads the players only once the box is used; inline drops the label, for a ViewBar beside the Options button, as the leaderboard's search. */
export default function PlayerPicker({ players, value, onPick, onFocus, inline = false }: { players: string[]; value: string; onPick: (player: string) => void; onFocus?: () => void; inline?: boolean }) {
  const [missed, setMissed] = useState(false);
  const names = useMemo(() => players.map((name) => ({ name })), [players]);
  const search = (term: string) => {
    const name = term === '' ? '' : pickPlayer(players, term);
    setMissed(name === null);
    if (name !== null) onPick(name);
  };
  return (
    <div className={inline ? 'games-player games-player-inline' : 'games-player'}>
      {!inline && <span>Player</span>}
      <SearchBox players={names} value={value} onSearch={search} onFocus={onFocus} />
      {missed && (
        <span className="games-player-missed" role="status">
          No player matches
        </span>
      )}
    </div>
  );
}
