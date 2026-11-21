/** One tournament (or circuit) card, as the Hall of Fame and Every Tournament both show them. */
import type { CardView } from '../lib/tournament-card';
import Tile from './Tile';

/** TournamentCard links to a tournament's page, with its icon, short description and information box, and its figures. */
export default function TournamentCard({
  href,
  name,
  card,
  rows,
  cta,
}: {
  href: string;
  name: string;
  card: CardView;
  rows: [string, string][];
  cta: string;
}) {
  return (
    <Tile link={{ href }}>
      <div className="series-card">
        <div className="series-card-top">
          <div className="series-icon">{card.icon}</div>
          {card.tagline !== null && <div className="tag">{card.tagline}</div>}
        </div>
        <div className="series-name">{name}</div>
        {card.summary !== null && <div className="series-summary">{card.summary}</div>}
        {rows.map(([label, value]) => (
          <div key={label} className="series-meta-row">
            <span>{label}</span>
            <span>{value}</span>
          </div>
        ))}
      </div>
      <div className="series-cta">
        <span>{cta}</span>
        <span aria-hidden="true">&rarr;</span>
      </div>
    </Tile>
  );
}
