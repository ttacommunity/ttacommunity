/** A player's peak career season as a highlight, on the player page and the matrix. */
import type { ReactNode } from 'react';
import type { ScoredYear } from '../lib/career';
import Highlight from './Highlight';
import { TitleBadge } from './PlayerMarks';
import './PeakSeason.css';

type Props = {
  season: ScoredYear;
  /** A small line above the title, such as what the season is measured by. */
  kicker?: string;
  /** A link beside it, such as to the year-by-year matrix. */
  link?: ReactNode;
  /** The page's own words about the season. */
  children: ReactNode;
};

/** PeakSeason shows the season's year and title, the page's words under them, and the link at the side. */
export default function PeakSeason({ season, kicker, link, children }: Props) {
  return (
    <Highlight>
      <div className="peak-season">
        <span className="peak-season-icon" aria-hidden="true">
          🌟
        </span>
        <div className="peak-season-body">
          {kicker !== undefined && <div className="peak-season-kicker">{kicker}</div>}
          <div className="peak-season-title">
            Peak Career Season: {season.year}
            {season.title !== null && <TitleBadge title={{ code: season.title }} long className="badge-inline" />}
          </div>
          <div className="peak-season-text">{children}</div>
        </div>
        {link}
      </div>
    </Highlight>
  );
}
