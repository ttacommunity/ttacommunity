/** A player's title badge, admin's badges and country flag, as every page draws them. */
import type { LeaderboardRow, TitleCode } from '@ttac/shared/ratings';
import { badgeHover, flagSrc, titleBadge } from '../lib/leaderboard-display';

/** TitleBadge shows a title in its colour: its code where room is short, its name when long, or a dash for none; the reason, else the name, on hover. */
export function TitleBadge({ title, long = false, className }: { title: { code: TitleCode; reason?: string } | null; long?: boolean; className?: string }) {
  const badge = titleBadge(title, long);
  if (badge === null) return <span className="badge-none">-</span>;
  return (
    <span className={`badge badge-${badge.code.toLowerCase()}${className === undefined ? '' : ` ${className}`}`} title={badge.hover}>
      {badge.text}
    </span>
  );
}

/** ExtraBadges shows the badges admin gave a player, each with its description on hover (the page's tooltip when tooltip). */
export function ExtraBadges({ badges, tooltip = false }: { badges: LeaderboardRow['badges']; tooltip?: boolean }) {
  return (
    <>
      {(badges ?? []).map((b) => {
        const hover = badgeHover(b);
        return (
          <span key={b.label} className="badge badge-extra" {...(tooltip ? { 'data-tooltip': hover } : { title: hover })}>
            {b.label}
          </span>
        );
      })}
    </>
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
