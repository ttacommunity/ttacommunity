/** Upstream's career box: medal counts, the honour roll or a summary sentence, accolades and the peak career season. */
import type { AchievementSummary, Player, PlayerCareer, TitleCode } from '@ttac/shared/ratings';
import { honorRoll, totalTitles } from '../lib/achievements';
import * as show from '../lib/leaderboard-display';
import { careerPeakYear, isPeakHigher } from '../lib/career';
import { matrixHref, type PlayerView } from '../lib/player-view';

function SeasonBadge({ code }: { code: TitleCode }) {
  if (code === 'SGM') return <span className="badge badge-inline badge-season badge-sgm">👑 GM</span>;
  return <span className={`badge badge-inline badge-season badge-${code.toLowerCase()}`}>{show.titleName(code)}</span>;
}

/** CareerBox shows upstream's career box: medal counts, the honour roll (or a summary sentence) and accolades first. */
export default function CareerBox({
  player,
  career,
  summary,
  view,
}: {
  player: Player;
  career: PlayerCareer | null;
  summary: AchievementSummary | null;
  view: PlayerView;
}) {
  const games = player.games.find((g) => g.format === view.format)?.games ?? 0;
  const season = career === null ? null : careerPeakYear(career.years, player.peak);
  const { title, peak } = player;

  return (
    <div className="rating-card career-box">
      <div className="career-head">
        <h3>
          <span>📜 Career Profile &amp; Tournament Accolades</span>
        </h3>
        {summary !== null && (
          <div className="career-medals">
            {totalTitles(summary) > 0 && (
              <span className="career-medal-gold">
                🏆 <strong>{totalTitles(summary)}</strong> Titles
              </span>
            )}
            {summary.gold > 0 && (
              <span className="career-medal-gold">
                🥇 <strong>{summary.gold}</strong>
              </span>
            )}
            {summary.silver > 0 && (
              <span className="career-medal-silver">
                🥈 <strong>{summary.silver}</strong>
              </span>
            )}
            {summary.bronze > 0 && (
              <span className="career-medal-bronze">
                🥉 <strong>{summary.bronze}</strong>
              </span>
            )}
          </div>
        )}
      </div>
      <div className="career-body">
        {summary !== null ? (
          <p className="career-honor-roll">🏅 {honorRoll(player.name, summary)}</p>
        ) : (
          <p className="career-summary">
            <strong>{player.name}</strong> has competed in <strong>{games}</strong> competitive matches across TTA tournament
            divisions{' '}
            {title !== null ? (
              <>
                and holds the official title of <strong>{show.titleName(title.code)}</strong>
                {peak !== null && isPeakHigher(title.code, peak) && (
                  <>
                    {' '}
                    <span className="career-peak-note">
                      (Peak: {show.titleName(peak.code)} in {peak.year})
                    </span>
                  </>
                )}
                .
              </>
            ) : peak !== null ? (
              <>
                and held the peak career title of <strong>{show.titleName(peak.code)}</strong> in <strong>{peak.year}</strong>.
              </>
            ) : (
              '.'
            )}
          </p>
        )}

        {summary !== null && summary.accolades.length > 0 && (
          <div className="career-honors-grid">
            {summary.accolades.map((a) => (
              <div className="career-honor-badge" key={a}>
                <span className="honor-star">★</span>
                <span className="honor-text">{a}</span>
              </div>
            ))}
          </div>
        )}

        {season !== null && (
          <div className="career-season">
            <div className="career-season-main">
              <span className="career-season-icon" aria-hidden="true">
                🌟
              </span>
              <div>
                <div className="career-season-title">
                  Peak Career Season: {season.year}
                  {season.title !== null && <SeasonBadge code={season.title} />}
                </div>
                <div className="career-season-stats">
                  {season.wins} Wins / {season.opponents} Opponents (<strong>{season.win_rate.toFixed(1)}%</strong> Win Rate)
                  {season.peak_rating ? (
                    <>
                      {' '}
                      • Peak Rating: <strong className="career-season-peak">{season.peak_rating.toFixed(1)}</strong>
                    </>
                  ) : null}
                </div>
              </div>
            </div>
            <a href={matrixHref(player.name, view)} className="career-matrix-link">
              View Year-by-Year Matrix »
            </a>
          </div>
        )}
      </div>
    </div>
  );
}
