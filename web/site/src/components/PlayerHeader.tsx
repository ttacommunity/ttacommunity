/** A player's header, the centred page header on each of their pages: the back link, the title they hold now, their name, a gold bar with flag and peak, the page's own line, then buttons to their other pages. */
import type { ReactNode } from 'react';
import PageHeader from '@ttac/shared/PageHeader';
import type { Player } from '@ttac/shared/ratings';
import { headerPeak, headerTitle, otherPlayerPages, type PlayerPage } from '../lib/player-header';
import { ExtraBadges, Flag, VideosButton } from './PlayerMarks';
import './PlayerHeader.css';

interface Props {
  player: Player;
  back: { href: string; label: string };
  /** The page this header heads; its buttons lead to the other two. */
  page: PlayerPage;
  /** Each of the player's pages' address, in the choices the page has. */
  hrefs: Record<PlayerPage, string>;
  /** Beside the name, such as the calibrating mark. */
  nameExtra?: ReactNode;
  /** Under the gold bar, such as the format a page shows. */
  subtitle?: ReactNode;
}

const PAGE_LINKS: Record<PlayerPage, { icon: string; label: string }> = {
  profile: { icon: '👤', label: 'Player Profile' },
  achievements: { icon: '🏆', label: 'Tournament Achievements' },
  years: { icon: '📊', label: 'Career Years' },
};

/** PlayerHeader shows player in the centred page header, with buttons to their other pages; the title is the one held now, a past world title shows as the peak. */
export default function PlayerHeader({ player, back, page, hrefs, nameExtra, subtitle }: Props) {
  const title = headerTitle(player.title);
  const peak = headerPeak(player.peak);
  return (
    <PageHeader
      centred
      back={back}
      above={
        <div className="player-header-badges">
          {title !== null && (
            <span className={`badge badge-${title.code.toLowerCase()}`} data-tooltip={title.hover}>
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
                Peak Rank: <span className={`badge badge-${peak.code.toLowerCase()}`}>{peak.text}</span>
              </span>
            </>
          )}
        </div>
        {subtitle !== undefined && <div className="player-header-subtitle">{subtitle}</div>}
        <div className="profile-links">
          {otherPlayerPages(page).map((p) => (
            <a key={p} href={hrefs[p]} className="btn-antique">
              <span aria-hidden="true">{PAGE_LINKS[p].icon}</span> {PAGE_LINKS[p].label} »
            </a>
          ))}
          <VideosButton name={player.name} hasVideos={player.has_videos} />
        </div>
      </div>
    </PageHeader>
  );
}
