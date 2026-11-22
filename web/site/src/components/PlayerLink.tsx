/** A link to a player's page, the one way every page links to a player, in the site's one look for a name. */
import type { ReactNode } from 'react';
import type { Track } from '../lib/leaderboard-view';
import { playerLinkClass, type Place } from '../lib/player-link';
import { hasPlayerPage, playerHref } from '../lib/player-view';

interface Props {
  name: string;
  ranking?: Track;
  /** Where the player finished, for a name that stands for a result: first, second and third in the medals' colours, last set back. */
  place?: Place;
  /** A name that is not the subject here, such as the other seats in a game between two players. */
  quiet?: boolean;
  children?: ReactNode;
}

/** PlayerLink links to name's page, showing the name unless children are given. */
export default function PlayerLink({ name, ranking, place, quiet, children }: Props) {
  const className = playerLinkClass({ place, quiet });
  if (!hasPlayerPage(name)) {
    return <span className={className}>{children ?? name}</span>;
  }
  return (
    <a href={playerHref(name, ranking)} className={className}>
      {children ?? name}
    </a>
  );
}
