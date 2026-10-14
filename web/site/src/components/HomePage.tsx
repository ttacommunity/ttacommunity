/** The site's front page: the three pillar cards (Ratings, Tournaments, TTA Pulse) and the interactive community pulse feed. */
import { useEffect, useMemo, useState } from 'react';
import {
  currentRun,
  leaderboard,
  catalogue,
  catalogueTournament,
  type Run,
  type LeaderboardRow,
  type CatalogueEntry,
} from '@ttac/shared/ratings';
import { rankPool, inactiveCutoff, DEFAULT_VIEW } from '../lib/leaderboard-view';
import { placeNames, seasonName } from '../lib/catalogue-view';
import { waitingText } from '../lib/waiting';

import {
  DEFAULT_FEED_SETTINGS,
  loadFeedSettings,
  saveFeedSettings,
  type FeedSettings,
} from '../lib/feed-settings';
import { fetchFeedMatches, type FeedMatch } from '../lib/feed-data';
import FeedOptionsModal, { type AvailableTournament } from './FeedOptionsModal';
import PlayerLink from './PlayerLink';
import { type SearchPlayer } from './SearchBox';
import './HomePage.css';

interface ConcludedTournament {
  slug: string;
  name: string;
  seasonLabel: string;
  seasonWord: string;
  finished: string;
  champion: string | null;
  runnerUp: string | null;
  third: string | null;
}

interface HomeData {
  run: Run;
  top20Players: LeaderboardRow[];
  allLeaderboardRows: LeaderboardRow[];
  recentTournaments: ConcludedTournament[];
  availableTournaments: AvailableTournament[];
}


type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: HomeData };

const count = (n: number | null | undefined) => (n ?? 0).toLocaleString('en-US');

function formatDate(iso: string | null | undefined): string {
  if (!iso) return '';
  return new Date(iso).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', year: 'numeric' });
}

function scoreFormatted(n: number) {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

/** Rank class for color-grading match participants when revealed.
 * 2p: Gold -> Gray
 * 3p: Gold, Silver, Bronze
 * 4p+: Gold, Silver, Bronze, Gray
 */
function getPlayerRankClass(score: number, allScores: number[]): string {
  if (allScores.length < 2) return 'rank-gold';
  const sorted = [...allScores].sort((a, b) => b - a);
  const maxScore = sorted[0];
  const minScore = sorted[sorted.length - 1];

  if (maxScore === minScore) return 'rank-gold';
  if (score === maxScore) return 'rank-gold';

  const n = allScores.length;
  if (n === 2) {
    return 'rank-gray';
  }

  if (n === 3) {
    const rank = sorted.indexOf(score) + 1;
    if (rank === 2) return 'rank-silver';
    return 'rank-bronze';
  }

  // 4p+
  if (score === minScore) return 'rank-gray';
  const rank = sorted.indexOf(score) + 1;
  if (rank === 2) return 'rank-silver';
  if (rank === 3) return 'rank-bronze';
  return 'rank-gray';
}

/** One-click copyable replay code */
function ReplayBadge({ code }: { code: string | null }) {
  const [copied, setCopied] = useState(false);
  if (!code) return null;
  const copy = () => {
    if (!navigator.clipboard) return;
    void navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };
  return (
    <button
      type="button"
      className="home-replay-btn"
      onClick={copy}
      title="Click to copy CGE replay code"
      aria-label={`Copy replay code ${code}`}
    >
      <code>{code}</code>
      <span className="home-replay-hint">{copied ? '✓ Copied' : 'Copy'}</span>
    </button>
  );
}

export default function HomePage({ apiUrl }: { apiUrl: string }) {
  const [load, setLoad] = useState<Load>({ kind: 'loading' });

  // Feed configuration and match state
  const [feedSettings, setFeedSettings] = useState<FeedSettings>(DEFAULT_FEED_SETTINGS);
  const [showOptionsModal, setShowOptionsModal] = useState(false);
  const [feedMatches, setFeedMatches] = useState<FeedMatch[]>([]);
  const [matchesLoading, setMatchesLoading] = useState(false);

  // Spoilers state
  const [globalMatchSpoilers, setGlobalMatchSpoilers] = useState(false); // false = hidden, true = revealed
  const [revealedMatchIds, setRevealedMatchIds] = useState<Set<number>>(new Set());

  const [globalTourneySpoilers, setGlobalTourneySpoilers] = useState(false); // false = hidden, true = revealed
  const [revealedTourneyKeys, setRevealedTourneyKeys] = useState<Set<string>>(new Set());

  // Load saved feed settings from localStorage once mounted
  useEffect(() => {
    if (typeof window !== 'undefined' && window.localStorage) {
      setFeedSettings(loadFeedSettings(window.localStorage));
    }
  }, []);

  // 1. Initial Load: Current run, default filtered Leaderboard (active, min opps 30), and Tournaments
  useEffect(() => {
    currentRun(apiUrl)
      .then(async (run) => {
        const [board, cat] = await Promise.all([
          leaderboard(apiUrl, run.run_id, 'glicko2_daneo', 0).catch(() => null),
          catalogue(apiUrl, run.run_id).catch(() => null),
        ]);

        const rawRows = board ? board.players : [];
        // Apply default active view filter: active status, min opponents 30, with proper 12-month cutoff
        const activeRanked = rankPool(rawRows, DEFAULT_VIEW, undefined, inactiveCutoff(run.last_game));
        const top20 = activeRanked.slice(0, 20);

        // Process recent tournaments with recorded podiums from catalogue
        const recentTournaments: ConcludedTournament[] = [];
        const availableTournaments: AvailableTournament[] = [];

        if (cat) {
          for (const t of cat.tournaments) {
            availableTournaments.push({ slug: t.slug, name: t.name });
          }

          // Sort tournaments by last_game descending to fetch the freshest active circuits
          const sortedTournaments = [...cat.tournaments].sort((a, b) => b.last_game.localeCompare(a.last_game));
          const featuredTournaments = sortedTournaments.slice(0, 12);
          const detailed = await Promise.all(
            featuredTournaments.map((t: CatalogueEntry) =>
              catalogueTournament(apiUrl, run.run_id, t.slug).catch(() => null),
            ),
          );

          for (const item of detailed) {
            if (!item) continue;
            // Scan seasons with podiums in reverse (newest first)
            for (let i = item.seasons.length - 1; i >= 0; i--) {
              const s = item.seasons[i];
              if (s && s.podium && s.podium.length > 0) {
                const champNames = placeNames(s.podium, 1);
                const runnerNames = placeNames(s.podium, 2);
                const thirdNames = placeNames(s.podium, 3);
                const finishedDate = s.last_game ?? s.podium[0]?.finished ?? '';

                recentTournaments.push({
                  slug: item.slug,
                  name: item.name,
                  seasonLabel: s.label,
                  seasonWord: item.season_name || 'Season',
                  finished: finishedDate,
                  champion: champNames[0]?.name ?? null,
                  runnerUp: runnerNames[0]?.name ?? null,
                  third: thirdNames[0]?.name ?? null,
                });
                break; // Take the latest concluded season for this tournament
              }
            }
          }
          // Sort by finished date descending
          recentTournaments.sort((a, b) => b.finished.localeCompare(a.finished));
        }

        setLoad({
          kind: 'ready',
          data: {
            run,
            top20Players: top20,
            allLeaderboardRows: rawRows,
            recentTournaments: recentTournaments.slice(0, 8),
            availableTournaments,
          },
        });
      })
      .catch((e: unknown) => {
        setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) });
      });
  }, [apiUrl]);

  // 2. Query matches whenever run or feedSettings changes
  useEffect(() => {
    if (load.kind !== 'ready') {
      setFeedMatches([]);
      return;
    }
    const runId = load.data.run.run_id;
    let live = true;
    setMatchesLoading(true);

    fetchFeedMatches(apiUrl, runId, feedSettings, 8)
      .then((matches) => {
        if (live) {
          setFeedMatches(matches);
          setMatchesLoading(false);
        }
      })
      .catch(() => {
        if (live) setMatchesLoading(false);
      });

    return () => {
      live = false;
    };
  }, [apiUrl, load, feedSettings]);

  const data = load.kind === 'ready' ? load.data : null;
  const waiting = waitingText(load, 'the community feed');

  // Player search options for adding custom contenders
  const searchCandidates: SearchPlayer[] = useMemo(() => {
    if (!data) return [];
    return data.allLeaderboardRows.map((r) => ({
      name: r.name,
      country: r.country,
      title: r.title,
    }));
  }, [data]);

  const removeTournamentFilter = (slug: string) => {
    const next: FeedSettings = {
      ...feedSettings,
      tournaments: feedSettings.tournaments.filter((t) => t.slug !== slug),
    };
    setFeedSettings(next);
    if (typeof window !== 'undefined' && window.localStorage) {
      saveFeedSettings(window.localStorage, next);
    }
  };

  const removePlayerFilter = (name: string) => {
    const next: FeedSettings = {
      ...feedSettings,
      players: feedSettings.players.filter((p) => p.toLowerCase() !== name.toLowerCase()),
    };
    setFeedSettings(next);
    if (typeof window !== 'undefined' && window.localStorage) {
      saveFeedSettings(window.localStorage, next);
    }
  };

  const resetToPremier = () => {
    setFeedSettings(DEFAULT_FEED_SETTINGS);
    if (typeof window !== 'undefined' && window.localStorage) {
      saveFeedSettings(window.localStorage, DEFAULT_FEED_SETTINGS);
    }
  };


  // Toggle match spoiler
  const toggleMatchSpoiler = (matchId: number) => {
    setRevealedMatchIds((prev) => {
      const next = new Set(prev);
      if (next.has(matchId)) next.delete(matchId);
      else next.add(matchId);
      return next;
    });
  };

  // Toggle tournament spoiler
  const toggleTourneySpoiler = (key: string) => {
    setRevealedTourneyKeys((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  return (
    <div className="home-container">
      {/* Hero Header */}
      <section className="home-hero">
        <div className="home-hero-badge-row">
          <span className="home-platform-badge">🏛️ Official Competitive Platform</span>
        </div>
        <h1 className="home-hero-title">Through the Ages Community</h1>
        <p className="home-hero-subtitle">
          The definitive home for competitive Through the Ages: verified Glicko-D community ratings, premier tournament circuits,
          the World Championship Hall of Fame, and automated blitz matchmaking.
        </p>

        {data && (
          <div className="home-stats-bar">
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
      <section className="rating-card home-feed-section" aria-label="Community Pulse Feed">
        <div className="home-feed-header-bar">
          <h2 className="home-feed-heading">
            <span>📡</span> Community Pulse &amp; Highlights
          </h2>
          <span className="home-feed-badge">Live Updates</span>
        </div>

        {data === null ? (
          <div className="home-waiting-message">{waiting ?? 'Loading the latest highlights…'}</div>
        ) : (
          <div className="home-feed-grid">
            {/* ── Left Column: Recent Matches (Top Contenders & Premier Divisions) ── */}
            <div className="home-feed-card">
              <div className="feed-card-header">
                <h3 className="feed-card-title">
                  <span>🎮</span> Recent Matches
                </h3>
                <div className="feed-header-actions">
                  <button
                    type="button"
                    className="feed-options-btn"
                    onClick={() => setShowOptionsModal(true)}
                    title="Customize tournaments, divisions, and players in feed"
                    aria-label="Feed filter options"
                  >
                    <span>⚙️</span> Options
                  </button>
                  <button
                    type="button"
                    className="spoiler-toggle-btn"
                    onClick={() => setGlobalMatchSpoilers(!globalMatchSpoilers)}
                    title={globalMatchSpoilers ? 'Hide all match scores' : 'Reveal all match scores'}
                  >
                    {globalMatchSpoilers ? '👁️ Spoilers: Shown' : '🔒 Spoilers: Hidden'}
                  </button>
                </div>
              </div>
              <p className="feed-card-intro">
                Recently finished tournament matches featuring your selected divisions and competitors.
              </p>

              {/* Feed Filter Toolbar */}
              <div className="player-filter-toolbar">
                <div className="filter-active-tags-row">
                  <div className="filter-tags-scroll" aria-label="Active feed filters">
                    {feedSettings.tournaments.map((t) => (
                      <span key={t.slug} className="player-filter-tag tourney-filter-tag">
                        <span className="player-tag-name">
                          🏛️ {t.name}{t.division ? ` · ${t.division}` : ''}
                        </span>
                        <button
                          type="button"
                          className="player-tag-remove"
                          onClick={() => removeTournamentFilter(t.slug)}
                          title={`Remove ${t.name} from feed`}
                          aria-label={`Remove ${t.name}`}
                        >
                          ✕
                        </button>
                      </span>
                    ))}
                    {feedSettings.players.map((name) => (
                      <span key={name} className="player-filter-tag">
                        <span className="player-tag-name">👤 {name}</span>
                        <button
                          type="button"
                          className="player-tag-remove"
                          onClick={() => removePlayerFilter(name)}
                          title={`Remove ${name} from feed`}
                          aria-label={`Remove ${name}`}
                        >
                          ✕
                        </button>
                      </span>
                    ))}
                  </div>

                  <div className="filter-quick-actions">
                    <button
                      type="button"
                      className="feed-options-btn feed-filter-inline-btn"
                      onClick={() => setShowOptionsModal(true)}
                    >
                      <span>⚙️</span> Filter / Add
                    </button>
                    <button
                      type="button"
                      className="btn-antique reset-top-btn"
                      onClick={resetToPremier}
                      title="Reset to top divisions of IM, IC, and RL"
                    >
                      Reset to Premier
                    </button>
                  </div>
                </div>
              </div>

              {/* Matches List */}
              <div className="feed-list">
                {matchesLoading ? (
                  <div className="feed-sub-loading">Loading recent matches…</div>
                ) : feedMatches.length === 0 ? (
                  <div className="feed-empty-hint">
                    No recent matches found for the chosen filters. Try adjusting your selections with the Options button above.
                  </div>
                ) : (
                  feedMatches.map((m) => {
                    const isRevealed = globalMatchSpoilers || revealedMatchIds.has(m.matchId);
                    const matchScores = m.seats.map((st) => st.score);
                    return (
                      <div key={m.matchId} className="feed-match-item">
                        <div className="match-item-meta">
                          <span className="match-tournament">{m.tournament}</span>
                          {m.division && <span className="badge badge-division">{m.division}</span>}
                          <span className="badge badge-seats">{m.playerCount}P</span>
                          <span className="match-date">{m.date}</span>
                          <ReplayBadge code={m.replayCode} />
                        </div>

                        {/* Participants and Scores with Spoiler Protection for Points */}
                        <div className="match-seats-row">
                          <div className="match-seats-content">
                            {m.seats.map((s, idx) => {
                              const rankClass = isRevealed ? getPlayerRankClass(s.score, matchScores) : '';
                              return (
                                <span key={s.name} className={`match-seat ${rankClass}`.trim()}>
                                  <PlayerLink
                                    name={s.name}
                                    className={`match-player-link ${rankClass}`.trim()}
                                  />
                                  <span className={`match-score-badge ${isRevealed ? 'revealed' : 'blurred'}`}>
                                    {isRevealed ? `(${scoreFormatted(s.score)})` : '(???)'}
                                  </span>
                                  {idx < m.seats.length - 1 && <span className="match-sep">vs</span>}
                                </span>
                              );
                            })}
                          </div>

                          <button
                            type="button"
                            className="spoiler-cell-btn"
                            onClick={() => toggleMatchSpoiler(m.matchId)}
                            title={isRevealed ? 'Hide scores' : 'Reveal scores'}
                            aria-label={isRevealed ? 'Hide scores' : 'Reveal scores'}
                          >
                            {isRevealed ? 'Hide' : 'Reveal'}
                          </button>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>

              <div className="feed-card-footer">
                <a href="/ratings/tournaments/" className="feed-footer-link">
                  Browse All Matches in Catalogue &rarr;
                </a>
              </div>
            </div>


            {/* ── Right Column: Recent Tournaments & Champions ── */}
            <div className="home-feed-card">
              <div className="feed-card-header">
                <h3 className="feed-card-title">
                  <span>🏆</span> Recent Tournaments
                </h3>
                <button
                  type="button"
                  className="spoiler-toggle-btn"
                  onClick={() => setGlobalTourneySpoilers(!globalTourneySpoilers)}
                  title={globalTourneySpoilers ? 'Hide all tournament podiums' : 'Reveal all tournament podiums'}
                >
                  {globalTourneySpoilers ? '👁️ Spoilers: Shown' : '🔒 Spoilers: Hidden'}
                </button>
              </div>
              <p className="feed-card-intro">
                Recently concluded premier tournament editions and crowned titleholders.
              </p>

              <div className="feed-list">
                {data.recentTournaments.length === 0 ? (
                  <div className="feed-empty-hint">No recent tournament podiums recorded.</div>
                ) : (
                  data.recentTournaments.map((t) => {
                    const key = `${t.slug}-${t.seasonLabel}`;
                    const isRevealed = globalTourneySpoilers || revealedTourneyKeys.has(key);
                    return (
                      <div key={key} className="feed-tourney-item">
                        <div className="tourney-item-header">
                          <div className="tourney-title-row">
                            <a href={`/ratings/tournaments/?t=${encodeURIComponent(t.slug)}`} className="tourney-link">
                              {t.name}
                            </a>
                            <span className="tourney-season-badge">
                              {seasonName(t.seasonLabel, t.seasonWord)}
                            </span>
                          </div>
                          {t.finished && <span className="tourney-date">{formatDate(t.finished)}</span>}
                        </div>

                        {/* Podium with Spoiler Protection */}
                        <div className="tourney-podium-wrap">
                          {isRevealed ? (
                            <div className="tourney-podium-content revealed">
                              {t.champion && (
                                <div className="podium-row champion-row">
                                  <span className="podium-icon">🥇</span>
                                  <span className="podium-rank">Champion:</span>
                                  <PlayerLink name={t.champion} className="podium-player" />
                                </div>
                              )}
                              {t.runnerUp && (
                                <div className="podium-row">
                                  <span className="podium-icon">🥈</span>
                                  <span className="podium-rank">Runner-Up:</span>
                                  <PlayerLink name={t.runnerUp} className="podium-player" />
                                </div>
                              )}
                              {t.third && (
                                <div className="podium-row">
                                  <span className="podium-icon">🥉</span>
                                  <span className="podium-rank">3rd Place:</span>
                                  <PlayerLink name={t.third} className="podium-player" />
                                </div>
                              )}
                            </div>
                          ) : (
                            <div
                              className="tourney-podium-content podium-censored-mask"
                              onClick={() => toggleTourneySpoiler(key)}
                              role="button"
                              tabIndex={0}
                              onKeyDown={(e) => {
                                if (e.key === 'Enter' || e.key === ' ') toggleTourneySpoiler(key);
                              }}
                              title="Click to reveal podium results"
                            >
                              <div className="podium-row champion-row">
                                <span className="podium-icon">🥇</span>
                                <span className="podium-rank">Champion:</span>
                                <span className="podium-censored-pill">████████████</span>
                              </div>
                              {t.runnerUp && (
                                <div className="podium-row">
                                  <span className="podium-icon">🥈</span>
                                  <span className="podium-rank">Runner-Up:</span>
                                  <span className="podium-censored-pill">████████████</span>
                                </div>
                              )}
                              {t.third && (
                                <div className="podium-row">
                                  <span className="podium-icon">🥉</span>
                                  <span className="podium-rank">3rd Place:</span>
                                  <span className="podium-censored-pill">████████████</span>
                                </div>
                              )}
                            </div>
                          )}

                          <button
                            type="button"
                            className="spoiler-cell-btn"
                            onClick={() => toggleTourneySpoiler(key)}
                            title={isRevealed ? 'Hide podium' : 'Reveal podium'}
                            aria-label={isRevealed ? 'Hide podium' : 'Reveal podium'}
                          >
                            {isRevealed ? 'Hide' : 'Reveal'}
                          </button>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>

              <div className="feed-card-footer">
                <a href="/ratings/tournaments/" className="feed-footer-link">
                  Explore Full Tournament Registry &rarr;
                </a>
              </div>
            </div>

            {/* ── Third Column: TTA Pulse Blitz Feed (In Development) ── */}
            <div className="home-feed-card feed-card-pulse">
              <div className="feed-card-header">
                <h3 className="feed-card-title">
                  <span>⚡</span> TTA Pulse Feed
                </h3>
                <span className="feed-card-badge pulse-badge">In Development</span>
              </div>
              <p className="feed-card-intro">
                Live blitz games, instant matchmaking queue pairing, and rating velocity updates.
              </p>

              <div className="pulse-feed-placeholder">
                <div className="pulse-placeholder-status">
                  <div className="pulse-radar-anim">
                    <span className="radar-circle"></span>
                    <span className="radar-dot"></span>
                  </div>
                  <div className="pulse-radar-title">Matchmaking Engine Syncing</div>
                  <p className="pulse-radar-text">
                    Automated pairing and real-time CGE API game dispatch are currently undergoing integration testing.
                  </p>
                </div>

                <div className="pulse-features-list">
                  <div className="pulse-feature-item">
                    <span className="pulse-feature-icon">⏱️</span>
                    <div>
                      <strong>Rapid Blitz Ladder</strong>
                      <p>Fast-clock competitive duels with dedicated Elo / Glicko blitz volatility tracking.</p>
                    </div>
                  </div>
                  <div className="pulse-feature-item">
                    <span className="pulse-feature-icon">🤖</span>
                    <div>
                      <strong>Automated Game Spawning</strong>
                      <p>Instant lobby launch without manual player invites via direct CGE API handshake.</p>
                    </div>
                  </div>
                  <div className="pulse-feature-item">
                    <span className="pulse-feature-icon">🔔</span>
                    <div>
                      <strong>Match Alerts &amp; Spectating</strong>
                      <p>Real-time turn notifications, live spectator boards, and discord match reminders.</p>
                    </div>
                  </div>
                </div>
              </div>

              <div className="feed-card-footer">
                <span className="feed-footer-tag">Coming Soon in Phase 2</span>
              </div>
            </div>
          </div>
        )}
      </section>

      {showOptionsModal && data && (
        <FeedOptionsModal
          apiUrl={apiUrl}
          runId={data.run.run_id}
          availableTournaments={data.availableTournaments}
          searchPlayers={searchCandidates}
          settings={feedSettings}
          onSave={(next) => {
            setFeedSettings(next);
            if (typeof window !== 'undefined' && window.localStorage) {
              saveFeedSettings(window.localStorage, next);
            }
          }}
          onClose={() => setShowOptionsModal(false)}
        />
      )}
    </div>
  );
}

