/** The page header of a tournament's page, and of a circuit's: the card's icon beside the title, its tagline above, its short description under the subtitle, then the figures. */
import type { ReactNode } from 'react';
import PageHeader from '@ttac/shared/PageHeader';
import type { CardView } from '../lib/tournament-card';

/** A figure in the header: its label, its value, and whether the value is words (set smaller) rather than a number. */
export type HeroFact = { label: string; value: ReactNode; words?: boolean };

/** TournamentHero shows a page's title and subtitle in the page header, with its card's icon, tagline and short description, then its figures. */
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
    <PageHeader
      media={
        <div className="detail-hero-icon" aria-hidden="true">
          {card.icon}
        </div>
      }
      above={
        card.tagline !== null && (
          <p className="tournament-tagline">
            <span className="panel-tag">{card.tagline}</span>
          </p>
        )
      }
      title={title}
      subtitle={
        <>
          <div>{subtitle}</div>
          {card.summary !== null && <p className="tournament-summary">{card.summary}</p>}
        </>
      }
    >
      <div className="detail-meta-grid">
        {facts.map((f) => (
          <div key={f.label} className="meta-box">
            <div className="meta-box-label">{f.label}</div>
            <div className={f.words ? 'meta-box-val meta-box-text' : 'meta-box-val'}>{f.value}</div>
          </div>
        ))}
      </div>
    </PageHeader>
  );
}
