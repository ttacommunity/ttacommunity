/** Upstream's career box: a summary sentence, the peak career season, each model's peak and the model notes. */
import type { AchievementSummary, Format, Player, PlayerCareer, TitleCode } from '@ttac/shared/ratings';
import { honorRoll, totalTitles } from '../lib/achievements';
import * as show from '../lib/leaderboard-display';
import { careerPeakYear, divergenceNotes, isPeakHigher } from '../lib/career';
import { matrixHref, pageModel, ratingIn } from '../lib/player-view';
import type { ResetMode } from '../lib/leaderboard-view';

function SeasonBadge({ code }: { code: TitleCode }) {
  if (code === 'SGM') return <span className="badge badge-inline badge-season badge-sgm">👑 GM</span>;
  return <span className={`badge badge-inline badge-season badge-${code.toLowerCase()}`}>{show.titleName(code)}</span>;
}

/** CareerBox shows upstream's career box: medal counts, the honour roll (or a summary sentence) and accolades first. */
export default function CareerBox({
  player,
  career,
  summary,
  reset,
  format,
}: {
  player: Player;
  career: PlayerCareer | null;
  summary: AchievementSummary | null;
  reset: ResetMode;
  format: Format;
}) {
  const games = player.games.find((g) => g.format === format)?.games ?? 0;
  const season = career === null ? null : careerPeakYear(career.years, player.peak);
  const at = (engine: Parameters<typeof pageModel>[0]) => ratingIn(player, pageModel(engine, reset), format);
  const notes = divergenceNotes(at('glicko2_std'), at('glicko2_mp'), at('whr'));
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
                  {season.wins} Wins / {season.opponents} Matches (<strong>{season.win_rate.toFixed(1)}%</strong> Win Rate)
                  {season.peak_rating ? (
                    <>
                      {' '}
                      • Peak Rating: <strong className="career-season-peak">{season.peak_rating.toFixed(1)}</strong>
                    </>
                  ) : null}
                </div>
              </div>
            </div>
            <a href={matrixHref(player.name, format)} className="career-matrix-link">
              View Year-by-Year Matrix »
            </a>
          </div>
        )}

        {career !== null && career.peaks.length > 0 && (
          <div className="career-peaks">
            {career.peaks.map((p) => (
              <div key={p.model}>
                <span className="career-peak-label">{show.engineName(p.model)} Peak:</span>{' '}
                <strong className="career-peak-rating">{p.rating.toFixed(1)}</strong>{' '}
                <span className="career-peak-rank">(Rank #{p.rank})</span> <span className="career-peak-date">({p.date})</span>
              </div>
            ))}
          </div>
        )}

        {notes.length > 0 && (
          <ul className="career-notes">
            {notes.map((n) => (
              <li key={n}>{n}</li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
