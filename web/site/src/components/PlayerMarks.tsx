/** A player's title badge, admin's badges, country flag and link to their videos, as every page draws them. */
import type { LeaderboardRow, TitleCode } from '@ttac/shared/ratings';
import { badgeHover, flagSrc, titleBadge } from '../lib/leaderboard-display';
import { moviesHref } from '../lib/movies';

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

function PlayIcon() {
  return (
    <svg className="videos-icon" viewBox="1 3 14 10" aria-hidden="true" focusable="false">
      <rect x="1" y="3" width="14" height="10" rx="2.5" fill="currentColor" />
      <path className="videos-icon-play" d="M6.5 5.75v4.5L10.5 8z" />
    </svg>
  );
}

/** VideosLink is a play icon, as tall as capitals, linking to the player's videos on the Movies page; drawn only when they have some. */
export function VideosLink({ name, hasVideos }: { name: string; hasVideos: boolean }) {
  if (!hasVideos) return null;
  return (
    <a className="videos-link" href={moviesHref(name)} title={`Videos by ${name}`} aria-label={`Videos by ${name}`}>
      <PlayIcon />
    </a>
  );
}

/** VideosButton is the "Movies »" link among a player page's buttons, in the button class given; drawn only when they have videos. */
export function VideosButton({ name, hasVideos, className }: { name: string; hasVideos: boolean; className: string }) {
  if (!hasVideos) return null;
  return (
    <a className={`${className} videos-button`} href={moviesHref(name)} title={`Videos by ${name}`}>
      <PlayIcon /> Movies »
    </a>
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
