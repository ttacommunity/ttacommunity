/** A player's header: the back link, the title they hold now, their name, flag and peak, then the page's own links; full (centred) or compact (one row). */
import type { ReactNode } from 'react';
import BackLink from '@ttac/shared/BackLink';
import type { Player } from '@ttac/shared/ratings';
import { headerPeak, headerTitle } from '../lib/player-header';
import { ExtraBadges, Flag } from './PlayerMarks';
import './PlayerHeader.css';

interface Props {
  player: Player;
  back: { href: string; label: string };
  /** One row, for a page whose header sits above a wide table (Career Years). */
  compact?: boolean;
  /** Beside the name, such as the calibrating mark. */
  nameExtra?: ReactNode;
  /** Under the name in the compact form, such as the format shown. */
  subtitle?: ReactNode;
  links?: ReactNode;
}

/** PlayerHeader shows player as both forms do; the title is the one held now, a past world title shows as the peak. */
export default function PlayerHeader({ player, back, compact = false, nameExtra, subtitle, links }: Props) {
  const title = headerTitle(player.title);
  const peak = headerPeak(player.peak);
  const big = title !== null && (title.code === 'WC' || title.code === 'SGM');
  const titleBadge = title !== null && (
    <span className={`badge badge-${title.code.toLowerCase()}${big && !compact ? ' badge-profile-lg' : ''}`} data-tooltip={title.hover}>
      {title.text}
    </span>
  );
  if (compact) {
    return (
      <div className="matrix-header-card">
        <div className="header-back">
          <BackLink href={back.href}>{back.label}</BackLink>
        </div>
        <div>
          <div className="matrix-identity">
            <Flag country={player.country} />
            <h1>{player.name}</h1>
            {titleBadge}
            <ExtraBadges badges={player.badges} tooltip />
            {peak !== null && <span className={`badge badge-inline badge-${peak.code.toLowerCase()}`}>Peak: {peak.text}</span>}
          </div>
          {subtitle !== undefined && <div className="matrix-subtitle">{subtitle}</div>}
        </div>
        {links !== undefined && <div className="matrix-header-links">{links}</div>}
      </div>
    );
  }
  return (
    <div className="profile-header">
      <div className="header-back">
        <BackLink href={back.href}>{back.label}</BackLink>
      </div>
      <div className="player-identity">
        <div className="player-identity-name">
          {titleBadge}
          <ExtraBadges badges={player.badges} tooltip />
          <h1 className="player-profile-name">
            {player.name}
            {nameExtra}
          </h1>
        </div>
        <div className="player-meta-pill">
          <Flag country={player.country} />
          {peak !== null && (
            <>
              <span className="meta-separator">•</span>
              <span>
                Peak Rank: <span className={`badge badge-inline badge-${peak.code.toLowerCase()}`}>{peak.text}</span>
              </span>
            </>
          )}
        </div>
      </div>
      {links !== undefined && <div className="profile-links">{links}</div>}
    </div>
  );
}
