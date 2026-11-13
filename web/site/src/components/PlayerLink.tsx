/** A link to a player's page, the one way every page links to a player. */
import type { ReactNode } from 'react';
import type { Track } from '../lib/leaderboard-view';
import { hasPlayerPage, playerHref } from '../lib/player-view';

/** PlayerLink links to name's page, showing the name unless children are given. */
export default function PlayerLink({ name, ranking, className, children }: { name: string; ranking?: Track; className?: string; children?: ReactNode }) {
  if (!hasPlayerPage(name)) {
    return <span className={className}>{children ?? name}</span>;
  }
  return (
    <a href={playerHref(name, ranking)} className={className}>
      {children ?? name}
    </a>
  );
}
