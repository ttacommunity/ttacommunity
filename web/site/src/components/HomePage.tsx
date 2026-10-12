/** The site's front page: the three pillar cards (Ratings, Tournaments, TTA Pulse) and the live community pulse feed. */
import { useEffect, useState } from 'react';
import {
  currentRun,
  leaderboard,
  hallOfFame,
  catalogueTournament,
  type Run,
  type LeaderboardRow,
  type Legend,
} from '@ttac/shared/ratings';
import PlayerLink from './PlayerLink';
import { worldChampions, type WorldChampion } from '../lib/tournament-series';
import { waitingText } from '../lib/waiting';
import { cRating, winRate } from '../lib/leaderboard-display';
import './HomePage.css';

interface HomeData {
  run: Run;
  topPlayers: LeaderboardRow[];
  champions: WorldChampion[];
  legends: Legend[];
}

type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: HomeData };

const count = (n: number | null | undefined) => (n ?? 0).toLocaleString('en-US');

function formatDate(iso: string | null | undefined): string {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' });
}

export default function HomePage({ apiUrl, adminUrl }: { apiUrl: string; adminUrl: string }) {
  const [load, setLoad] = useState<Load>({ kind: 'loading' });

  useEffect(() => {
    currentRun(apiUrl)
      .then(async (run) => {
        const [board, hall, worlds] = await Promise.all([
          leaderboard(apiUrl, run.run_id, 'glicko2_daneo', 0).catch(() => null),
          hallOfFame(apiUrl, run.run_id).catch(() => null),
          catalogueTournament(apiUrl, run.run_id, 'worlds').catch(() => null),
        ]);
        const champions = worlds ? worldChampions(worlds.seasons, hall?.legends ?? []) : [];
        setLoad({
          kind: 'ready',
          data: {
            run,
            topPlayers: board ? board.players.slice(0, 5) : [],
            champions: champions.slice(0, 3),
            legends: hall?.legends ? hall.legends.slice(0, 5) : [],
          },
        });
      })
      .catch((e: unknown) => {
        setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) });
      });
  }, [apiUrl]);

  const data = load.kind === 'ready' ? load.data : null;
  const waiting = waitingText(load, 'the community feed');

  return (
    <div className="home-container">
      {/* Hero Header */}
      <section className="home-hero">
        <div className="home-hero-badge-row">
          <span className="home-platform-badge">🏛️ Official Competitive Platform</span>
          {adminUrl && (
            <a href={adminUrl} className="home-admin-link" title="Administration Portal">
              ⚙️ Admin
            </a>
          )}
        </div>
        <h1 className="home-hero-title">Through the Ages Community</h1>
        <p className="home-hero-subtitle">
          The definitive home for competitive Through the Ages: verified Glicko-D community ratings, premier tournament circuits,
          the World Championship Hall of Fame, and automated blitz matchmaking.
        </p>

        {data && (
          <div className="home-stats-bar">
            <div className="home-stat-pill">
              Database: <strong>Run #{data.run.run_id}</strong>
            </div>
            {data.run.players !== null && (
              <div className="home-stat-pill">
                Active Players: <strong>{count(data.run.players)}</strong>
              </div>
            )}
            {data.run.games !== null && (
              <div className="home-stat-pill">
                Recorded Matches: <strong>{count(data.run.games)}</strong>
              </div>
            )}
            {data.run.finished_at && (
              <div className="home-stat-pill">
                Updated: <strong>{formatDate(data.run.finished_at)}</strong>
              </div>
            )}
          </div>
        )}
      </section>

      {/* 3 Pillar Cards (The Hub) */}
      <section className="home-pillars" aria-label="Core Sections">
        {/* Card 1: Ratings */}
        <div className="home-pillar-card">
          <div className="pillar-card-header">
            <span className="pillar-icon">🏆</span>
            <span className="pillar-tag">Glicko-D Engine</span>
          </div>
          <h2 className="pillar-title">Ratings &amp; Leaderboards</h2>
          <p className="pillar-desc">
            The multi-paradigm community rating system tracking performance across all competitive player counts with
            variance-calibrated priors and inactivity decay.
          </p>
          <ul className="pillar-features">
            <li>4-Player, 3-Player &amp; 2-Player duel rankings</li>
            <li>In-depth player profiles &amp; historical career trajectories</li>
            <li>Head-to-head match matrices &amp; predictive win rates</li>
          </ul>
          <div className="pillar-action">
            <a href="/ratings/" className="btn-antique pillar-btn">
              Explore Leaderboards &rarr;
            </a>
          </div>
        </div>

        {/* Card 2: Tournaments */}
        <div className="home-pillar-card">
          <div className="pillar-card-header">
            <span className="pillar-icon">🏛️</span>
            <span className="pillar-tag">6 Major Circuits</span>
          </div>
          <h2 className="pillar-title">Tournaments &amp; Hall of Fame</h2>
          <p className="pillar-desc">
            The historical registry of competitive Through the Ages. Explore official championships, season stages, match replay
            logs, and verified podium trophyboards.
          </p>
          <ul className="pillar-features">
            <li>World Championship Hall of Fame &amp; annual podiums</li>
            <li>Premier division records across 4 official major leagues</li>
            <li>Grand Slams, knockout cups &amp; competitive ladders</li>
          </ul>
          <div className="pillar-action">
            <a href="/ratings/hall_of_fame/" className="btn-antique pillar-btn">
              Explore Tournaments &rarr;
            </a>
          </div>
        </div>

        {/* Card 3: TTA Pulse */}
        <div className="home-pillar-card pillar-pulse">
          <div className="pillar-card-header">
            <span className="pillar-icon">⚡</span>
            <span className="pillar-tag pillar-tag-upcoming">In Development</span>
          </div>
          <h2 className="pillar-title">TTA Pulse</h2>
          <p className="pillar-desc">
            The upcoming blitz matchmaking queue pairing players by Glicko-D rating tier for rapid, scheduled matches launched
            directly via CGE&apos;s API.
          </p>
          <ul className="pillar-features">
            <li>Skill-based automated queue pairing</li>
            <li>Seamless game creation via CGE API integration</li>
            <li>Rapid blitz ratings &amp; live match notifications</li>
          </ul>
          <div className="pillar-action">
            <span className="pillar-coming-soon">Coming Soon &bull; In Development</span>
          </div>
        </div>
      </section>

      {/* Community Pulse Feed (Live Highlights Below the Fold) */}
      <section className="home-feed-section" aria-label="Community Pulse Feed">
        <h2 className="home-feed-heading">
          <span>📡</span> Community Pulse &amp; Highlights
        </h2>

        {data === null ? (
          <div className="rating-card home-waiting-card">{waiting ?? 'Loading the latest highlights…'}</div>
        ) : (
          <div className="home-feed-grid">
            {/* Spotlight A: Top Rated Players */}
            <div className="home-feed-card">
              <div className="feed-card-header">
                <h3 className="feed-card-title">
                  <span>📈</span> Leaderboard Spotlight (Overall)
                </h3>
                <span className="feed-card-badge">Top Competitors</span>
              </div>
              <p className="feed-card-intro">
                Current top rated players in the active Glicko-D continuous ranking.
              </p>
              <div className="feed-list">
                {data.topPlayers.map((p, idx) => (
                  <div key={p.name} className="feed-player-row">
                    <span className={`feed-rank ${idx < 3 ? 'feed-rank-top' : ''}`}>#{p.rank ?? idx + 1}</span>
                    <div className="feed-player-info">
                      <PlayerLink name={p.name} className="feed-player-name" />
                      <span className="feed-player-sub">
                        {winRate(p.win_rate)} win rate &bull; {p.games} games
                      </span>
                    </div>
                    <div className="feed-rating-pill">
                      <span className="feed-rating-value">{cRating(p.c_rating)}</span>
                      <span className="feed-rating-label">Conservative</span>
                    </div>
                  </div>
                ))}
              </div>
              <div className="feed-card-footer">
                <a href="/ratings/" className="feed-footer-link">
                  View Full Leaderboard &rarr;
                </a>
              </div>
            </div>

            {/* Spotlight B: Reigning Champions */}
            <div className="home-feed-card">
              <div className="feed-card-header">
                <h3 className="feed-card-title">
                  <span>👑</span> World Champions &amp; Titleholders
                </h3>
                <span className="feed-card-badge">Hall of Fame</span>
              </div>
              <p className="feed-card-intro">
                Reigning and benchmark champions crowned across contested editions.
              </p>
              <div className="feed-list">
                {data.champions.map((c) => (
                  <div key={c.season} className="feed-champ-row">
                    <span className="feed-champ-icon">{c.icon}</span>
                    <div className="feed-champ-info">
                      <div className="feed-champ-title-row">
                        <PlayerLink name={c.name} className="feed-player-name" />
                        <span className="badge badge-wc">{c.season} WC</span>
                      </div>
                      <span className="feed-champ-desc">{c.text}</span>
                    </div>
                  </div>
                ))}
              </div>
              <div className="feed-card-footer">
                <a href="/ratings/hall_of_fame/" className="feed-footer-link">
                  Explore Full Hall of Fame &rarr;
                </a>
              </div>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}
