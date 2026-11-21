/** The tile, figure and highlight samples on /design, with invented content. */
import Tile, { TileLink } from './Tile';
import MetricCard, { MetricGrid } from './MetricCard';
import Highlight from './Highlight';
import Honours from './Honours';
import { TitleBadge } from './PlayerMarks';

/** TileSamples shows each form of tile: a link, the current one, one with a TileLink, a picture tile and a plain one. */
export default function TileSamples() {
  return (
    <div className="design-tiles">
      <Tile link={{ href: '/design/' }}>
        <strong>A link tile</strong>
        <p>All of it is one link. It lifts with a gold edge under the mouse.</p>
      </Tile>
      <Tile link={{ href: '/design/' }} current>
        <strong>The current tile</strong>
        <p>Among tiles that each change the page, the one it shows now.</p>
      </Tile>
      <Tile>
        <strong>
          <TileLink href="/design/">A tile with a TileLink</TileLink>
        </strong>
        <p>
          Its title&apos;s link covers the tile, and <a href="/design/">a link inside</a> stays its own.
        </p>
      </Tile>
      <Tile picture={<img className="design-tile-img" src="/img/groups/classic.jpg" alt="" loading="lazy" width={340} height={284} />}>
        <TileLink href="/design/">A picture tile</TileLink>
        <span>A caption under its picture.</span>
      </Tile>
      <Tile>
        <strong>A tile that is not a link</strong>
        <p>It stays still under the mouse.</p>
      </Tile>
    </div>
  );
}

/** FigureSamples shows figure tiles with an icon, words for a value, and a title badge beside a name. */
export function FigureSamples() {
  return (
    <MetricGrid>
      <MetricCard icon="🥈" label="Sample medals" value={4} tone="silver" hint="An explanation under the mouse" />
      <MetricCard icon="🏰" label="Sample peak" value="Prince 2" note="Best: 1 / 3 (Season 4)" tone="gold" words />
      <MetricCard
        label="Sample champion"
        value={
          <span>
            <TitleBadge title={{ code: 'WC' }} /> Sample_Player
          </span>
        }
        tone="gold"
      />
    </MetricGrid>
  );
}

/** HighlightSamples shows a highlight and a list of honours. */
export function HighlightSamples() {
  return (
    <div className="design-stack">
      <Highlight>
        <strong>A highlight</strong>: one fact singled out, such as a peak season, a champion or a story.
      </Highlight>
      <Honours accolades={['2x Sample Champion', '1x Sample Runner-up']} />
    </div>
  );
}
