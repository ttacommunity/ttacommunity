/** A player's header, the centred page header on each of their pages: the back link, the title they hold now, their name, a gold bar with flag and peak, then the page's own line and links. */
import type { ReactNode } from 'react';
import PageHeader from '@ttac/shared/PageHeader';
import type { Player } from '@ttac/shared/ratings';
import { headerPeak, headerTitle } from '../lib/player-header';
import { ExtraBadges, Flag } from './PlayerMarks';
import './PlayerHeader.css';

interface Props {
  player: Player;
  back: { href: string; label: string };
  /** Beside the name, such as the calibrating mark. */
  nameExtra?: ReactNode;
  /** Under the gold bar, such as the format a page shows. */
  subtitle?: ReactNode;
  links?: ReactNode;
}

/** PlayerHeader shows player in the centred page header; the title is the one held now, a past world title shows as the peak. */
export default function PlayerHeader({ player, back, nameExtra, subtitle, links }: Props) {
  const title = headerTitle(player.title);
  const peak = headerPeak(player.peak);
  const big = title !== null && (title.code === 'WC' || title.code === 'SGM');
  return (
    <PageHeader
      centred
      back={back}
      above={
        <div className="player-header-badges">
          {title !== null && (
            <span className={`badge badge-${title.code.toLowerCase()}${big ? ' badge-profile-lg' : ''}`} data-tooltip={title.hover}>
              {title.text}
            </span>
          )}
          <ExtraBadges badges={player.badges} tooltip />
        </div>
      }
      title={
        <>
          {player.name}
          {nameExtra}
        </>
      }
    >
      <div className="player-header-extras">
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
        {subtitle !== undefined && <div className="player-header-subtitle">{subtitle}</div>}
        {links !== undefined && <div className="profile-links">{links}</div>}
      </div>
    </PageHeader>
  );
}
