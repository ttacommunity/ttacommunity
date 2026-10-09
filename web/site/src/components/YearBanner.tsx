/** Upstream's season archive banner: the year, its snapshot, game count, record view and World Champion. */
import type { YearLeaderboard } from '@ttac/shared/ratings';
import type { View } from '../lib/leaderboard-view';
import type { LinkTo } from './OptionsDrawer';
import Segmented from '@ttac/shared/Segmented';
import PlayerLink from './PlayerLink';

interface Props {
  board: YearLeaderboard;
  latest: boolean;
  ranked: number;
  view: View;
  linkTo: LinkTo;
}

/** YearBanner heads a year's board; the switch shows that year's record or the career up to it. */
export default function YearBanner({ board, latest, ranked, view, linkTo }: Props) {
  const y = board.year;
  const champion = board.champions.find((c) => c.reigning) ?? board.champions[0];
  return (
    <div className="archive-hero-banner">
      <div>
        <div className="archive-head">
          <span className="archive-badge">{latest ? `Active Season ${y} Standings` : `Season ${y} Final Archive`}</span>
          <span className="archive-snapshot">
            Snapshot Date: <strong>{board.snapshot}</strong>
          </span>
        </div>
        <p className="archive-facts">
          Recorded Matches in {y}: <strong className="archive-games">{board.games.toLocaleString('en')}</strong> • Ranked
          Competitors: <strong className="archive-ranked">{ranked.toLocaleString('en')}</strong>
          {board.previous !== null && (
            <>
              {' '}
              • <span className="archive-deltas">Deltas vs {y - 1} Year-End</span>
            </>
          )}
        </p>
        <div className="archive-stats">
          <span className="archive-stats-label">📊 Match Stats Mode:</span>
          <Segmented
            label="Match stats mode"
            look="banner"
            options={[
              {
                value: 'season',
                label: `📅 Season ${y} Only`,
                tooltip: `Display wins, losses, and draws for Season ${y} only. Ratings are derived according to the selected engine and reset mode.`,
              },
              {
                value: 'career',
                label: '📈 Career to Date',
                tooltip: `Display cumulative career wins, losses, and draws up through Season ${y}. Ratings are derived according to the selected engine and reset mode.`,
              },
            ]}
            value={view.stats}
            link={(stats) => linkTo({ ...view, stats, page: 1 })}
          />
        </div>
      </div>
      {champion !== undefined && (
        <div className="archive-champion">
          <div className="archive-champion-title">🏆 {champion.reigning ? 'Reigning World Champion' : 'World Champion'}</div>
          <div className="archive-champion-name">
            <PlayerLink name={champion.name} />
          </div>
        </div>
      )}
    </div>
  );
}
