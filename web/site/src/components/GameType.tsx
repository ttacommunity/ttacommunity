/** A game's type in a games table: how many played, as a tag, and "Unrated" beside it when the game does not count. */
import './GameType.css';

/** GameType shows a game of players seats, marked unrated unless rated. */
export default function GameType({ players, rated }: { players: number; rated: boolean }) {
  return (
    <span className="game-type">
      <span className="tag">{players}P</span>
      {!rated && <span className="status">Unrated</span>}
    </span>
  );
}
