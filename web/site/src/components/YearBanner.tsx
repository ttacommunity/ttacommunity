/** Upstream's season archive banner: the year, its snapshot, game count, record view and World Champion. */
import type { YearLeaderboard } from '@ttac/shared/ratings';
import type { View } from '../lib/leaderboard-view';
import { playerHref } from '../lib/player-view';
import type { LinkTo } from './OptionsDrawer';

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
          <div className="reset-btn-group" role="group" aria-label="Match stats mode">
            <a
              {...linkTo({ ...view, stats: 'season', page: 1 })}
              className={view.stats === 'season' ? 'reset-btn active' : 'reset-btn'}
              aria-current={view.stats === 'season' ? 'true' : undefined}
              data-tooltip={`Display wins, losses, and draws for Season ${y} only. Ratings are derived according to the selected engine and reset mode.`}
            >
              📅 Season {y} Only
            </a>
            <a
              {...linkTo({ ...view, stats: 'career', page: 1 })}
              className={view.stats === 'career' ? 'reset-btn active' : 'reset-btn'}
              aria-current={view.stats === 'career' ? 'true' : undefined}
              data-tooltip={`Display cumulative career wins, losses, and draws up through Season ${y}. Ratings are derived according to the selected engine and reset mode.`}
            >
              📈 Career to Date
            </a>
          </div>
        </div>
      </div>
      {champion !== undefined && (
        <div className="archive-champion">
          <div className="archive-champion-title">🏆 {champion.reigning ? 'Reigning World Champion' : 'World Champion'}</div>
          <div className="archive-champion-name">
            <a href={playerHref(champion.name)}>{champion.name}</a>
          </div>
        </div>
      )}
    </div>
  );
}
