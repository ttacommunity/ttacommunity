/** A tile: a card in a grid, raised; a link tile leads to one place and lifts under the mouse. */
import type { ReactNode } from 'react';
import type { Link } from '../lib/navigate';
import './Tile.css';

type Props = {
  /** Where the whole tile leads, when nothing else in it is a link or a button; otherwise put a TileLink in it. */
  link?: Link | { href: string };
  /** The tile for what the page shows now, among tiles that each change it. */
  current?: boolean;
  /** A picture across the tile's top, edge to edge, such as a challenge's art or a video's still. */
  picture?: ReactNode;
  children: ReactNode;
};

/** Tile draws the box, its picture and its body; the page lays out its own content inside. */
export default function Tile({ link, current, picture, children }: Props) {
  const look = ['tile', link && 'tile-link', current && 'tile-current'].filter(Boolean).join(' ');
  const body = (
    <>
      {picture !== undefined && <div className="tile-picture">{picture}</div>}
      <div className="tile-body">{children}</div>
    </>
  );
  if (link === undefined) return <div className={look}>{body}</div>;
  return (
    <a className={look} aria-current={current ? 'true' : undefined} {...link}>
      {body}
    </a>
  );
}

/** TileLink is a tile's main link (href) or action (onClick, given the button as the opener of what it opens), usually its title: its click area covers the whole tile, and the tile's other links stay their own; label names it for a reader when its words are not enough. */
export function TileLink({ href, onClick, label, children }: { href?: string; onClick?: (opener: HTMLElement) => void; label?: string; children: ReactNode }) {
  if (href !== undefined) {
    return (
      <a className="tile-cover" href={href} aria-label={label}>
        {children}
      </a>
    );
  }
  return (
    <button type="button" className="tile-cover" onClick={(e) => onClick?.(e.currentTarget)} aria-label={label}>
      {children}
    </button>
  );
}
