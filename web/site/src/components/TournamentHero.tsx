/** The header of a tournament's page, and of a circuit's: the card's icon, short description and information box, with the figures. */
import type { ReactNode } from 'react';
import type { CardView } from '../lib/tournament-card';

/** A figure in the header: its label, its value, and whether the value is words (set smaller) rather than a number. */
export type HeroFact = { label: string; value: ReactNode; words?: boolean };

/** TournamentHero shows a page's title and subtitle beside its card's icon and short description, then its information box and figures. */
export default function TournamentHero({
  card,
  title,
  subtitle,
  facts,
}: {
  card: CardView;
  title: string;
  subtitle: ReactNode;
  facts: HeroFact[];
}) {
  return (
    <div className="detail-hero">
      <div className="detail-hero-top">
        <div className="detail-hero-left">
          <div className="detail-hero-icon">{card.icon}</div>
          <div>
            <h1 className="detail-hero-title">{title}</h1>
            <div className="detail-hero-subtitle">{subtitle}</div>
          </div>
        </div>
        {card.tagline !== null && <div className="detail-hero-badge">{card.tagline}</div>}
      </div>
      {card.summary !== null && <div className="detail-hero-desc">{card.summary}</div>}
      <div className="detail-meta-grid">
        {facts.map((f) => (
          <div key={f.label} className="meta-box">
            <div className="meta-box-label">{f.label}</div>
            <div className={f.words ? 'meta-box-val meta-box-text' : 'meta-box-val'}>{f.value}</div>
          </div>
        ))}
      </div>
    </div>
  );
}
