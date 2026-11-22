/** A finishing place in a game: its number in a circle, in the medal's colour for the first three. */
import { placeTone } from '../lib/place';
import './Place.css';

/** Place shows place n of a game. */
export default function Place({ n }: { n: number }) {
  return <span className={`place-circle place-circle-${placeTone(n)}`}>{n}</span>;
}
