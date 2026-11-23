/** The page header of a tournament's page, and of a circuit's: the card's icon beside the title, its tagline at the top right, its short description under the subtitle, then the figures. */
import type { ReactNode } from 'react';
import PageHeader from '@ttac/shared/PageHeader';
import type { CardView } from '../lib/tournament-card';
import MetricCard, { MetricGrid } from './MetricCard';

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
      emblem={card.icon}
      tag={card.tagline ?? undefined}
      title={title}
      subtitle={
        <>
          <div>{subtitle}</div>
          {card.summary !== null && <p className="tournament-summary">{card.summary}</p>}
        </>
      }
    >
      <div className="detail-meta-grid">
        <MetricGrid compact>
          {facts.map((f) => (
            <MetricCard key={f.label} label={f.label} value={f.value} tone="gold" words={f.words} />
          ))}
        </MetricGrid>
      </div>
    </PageHeader>
  );
}
