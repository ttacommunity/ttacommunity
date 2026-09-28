/** A player's title badge and country flag, as upstream draws them in its tables. */
import type { LeaderboardRow } from '@ttac/shared/ratings';
import { flagSrc, titleName } from '../lib/leaderboard-display';

/** TitleBadge shows the run's title in its colour, or a dash for none. */
export function TitleBadge({ title }: { title: LeaderboardRow['title'] }) {
  if (title === null) return <span className="badge-none">-</span>;
  return (
    <span className={`badge badge-${title.code.toLowerCase()}`} title={title.reason || titleName(title.code)}>
      {title.code}
    </span>
  );
}

/** Flag shows the player's country flag, or a faint globe when it is not known. */
export function Flag({ country }: { country: string | null }) {
  if (country === null) {
    return (
      <span className="flag-icon flag-none" title="Country: Unknown" role="img" aria-label="Country unknown">
        🌐
      </span>
    );
  }
  return (
    <span className="flag-icon" title={`Country: ${country}`}>
      <img className="flag-icon-img" src={flagSrc(country)} alt={country} width={22} height={15} loading="lazy" />
    </span>
  );
}
