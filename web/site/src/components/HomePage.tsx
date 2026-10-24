/** The site's front page: the four pillar cards (Ratings, Tournaments, Blitz, TTA Pulse) and the interactive community pulse feed. */
import { useEffect, useMemo, useState } from 'react';
import {
  currentRun,
  leaderboard,
  catalogue,
  recentPodiums,
  type Run,
  type LeaderboardRow,
} from '@ttac/shared/ratings';
import { formatDay, formatInstant, viewerZone } from '@ttac/shared/time';
import { podiumCard, seasonName, type PodiumCard } from '../lib/catalogue-view';
import { seatRankClass, settled, type Section } from '../lib/home-view';
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

interface HomeData {
  run: Run;
  players: Section<LeaderboardRow[]>;
  recentTournaments: Section<PodiumCard[]>;
  availableTournaments: Section<AvailableTournament[]>;
}


type Load = { kind: 'loading' } | { kind: 'error'; message: string } | { kind: 'ready'; data: HomeData };

const count = (n: number | null | undefined) => (n ?? 0).toLocaleString('en-US');

function scoreFormatted(n: number) {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
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
  const [feedMatches, setFeedMatches] = useState<Section<FeedMatch[]>>({ kind: 'loading' });

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
        const [players, tournaments, podiums] = await Promise.all([
          settled(leaderboard(apiUrl, run.run_id, 'glicko2_daneo', 0).then((b) => b.players)),
          settled(catalogue(apiUrl, run.run_id).then((c) => c.tournaments.map((t) => ({ slug: t.slug, name: t.name })))),
          settled(recentPodiums(apiUrl, run.run_id).then((p) => p.podiums.map(podiumCard))),
        ]);
        setLoad({ kind: 'ready', data: { run, players, recentTournaments: podiums, availableTournaments: tournaments } });
      })
      .catch((e: unknown) => {
        setLoad({ kind: 'error', message: e instanceof Error ? e.message : String(e) });
      });
  }, [apiUrl]);

  // 2. Query matches whenever run or feedSettings changes
  useEffect(() => {
    if (load.kind !== 'ready') return;
    const runId = load.data.run.run_id;
    let live = true;
    setFeedMatches({ kind: 'loading' });
    void settled(fetchFeedMatches(apiUrl, runId, feedSettings, 8)).then((matches) => {
      if (live) setFeedMatches(matches);
    });

    return () => {
      live = false;
    };
  }, [apiUrl, load, feedSettings]);

  const data = load.kind === 'ready' ? load.data : null;
  const waiting = waitingText(load, 'the community feed');

  // Player search options for adding custom contenders
  const searchCandidates: SearchPlayer[] = useMemo(() => {
    if (data?.players.kind !== 'ready') return [];
    return data.players.data.map((r) => ({
      name: r.name,
      country: r.country,
      title: r.title,
    }));
  }, [data]);

  const removeTournamentFilter = (slug: string, division?: string) => {
    const next: FeedSettings = {
      ...feedSettings,
      tournaments: feedSettings.tournaments.filter(
        (t) => !(t.slug === slug && (t.division ?? '') === (division ?? '')),
      ),
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
        <div className="home-hero-layout">
          <div className="home-hero-logo-col">
            <img
              src="/img/HubLogo.png"
              alt="Through the Ages Community"
              className="home-hero-logo"
              width="825"
              height="460"
            />
          </div>
          <div className="home-hero-content-col">
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
                    Updated: <strong>{formatInstant(data.run.finished_at, viewerZone())}</strong>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
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

        {/* Card 3: Blitz */}
        <div className="home-pillar-card pillar-upcoming">
          <div className="pillar-card-header">
            <span className="pillar-icon">⏱️</span>
            <span className="pillar-tag pillar-tag-upcoming">In Development</span>
          </div>
          <h2 className="pillar-title">Blitz Tournaments</h2>
          <p className="pillar-desc">
            Community-run blitz tournaments: weekly Swiss rounds paired by score and time zone, then a seeded knockout to
            crown the champion.
          </p>
          <ul className="pillar-features">
            <li>Entrants, weekly pairings &amp; results</li>
            <li>Standings with tiebreaks</li>
            <li>Knockout bracket &amp; champion</li>
          </ul>
          <div className="pillar-action">
            <a href="/blitz/" className="btn-antique pillar-btn">
              Explore Blitz &rarr;
            </a>
          </div>
        </div>

        {/* Card 4: TTA Pulse */}
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
              {[waitingText(data.availableTournaments, 'the tournaments to choose from'), waitingText(data.players, 'the players to choose from')]
                .filter((problem) => problem !== null)
                .map((problem) => (
                  <p key={problem} className="feed-sub-loading" role="alert">
                    {problem}
                  </p>
                ))}

              {/* Feed Filter Toolbar */}
              <div className="player-filter-toolbar">
                <div className="filter-active-tags-row">
                  <div className="filter-tags-scroll" aria-label="Active feed filters">
                    {feedSettings.tournaments.map((t, idx) => (
                      <span key={`${t.slug}-${t.division || 'all'}-${idx}`} className="player-filter-tag tourney-filter-tag">
                        <span className="player-tag-name">
                          🏛️ {t.name}{t.division ? ` · ${t.division}` : ''}
                        </span>
                        <button
                          type="button"
                          className="player-tag-remove"
                          onClick={() => removeTournamentFilter(t.slug, t.division)}
                          title={`Remove ${t.name}${t.division ? ` (${t.division})` : ''} from feed`}
                          aria-label={`Remove ${t.name}${t.division ? ` (${t.division})` : ''}`}
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
                {feedMatches.kind !== 'ready' ? (
                  <div className="feed-sub-loading">{waitingText(feedMatches, 'recent matches')}</div>
                ) : feedMatches.data.length === 0 ? (
                  <div className="feed-empty-hint">
                    No recent matches found for the chosen filters. Try adjusting your selections with the Options button above.
                  </div>
                ) : (
                  feedMatches.data.map((m) => {
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
                              const rankClass = isRevealed ? seatRankClass(s.score, matchScores) : '';
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
                {data.recentTournaments.kind !== 'ready' ? (
                  <div className="feed-sub-loading">{waitingText(data.recentTournaments, 'the recent tournaments')}</div>
                ) : data.recentTournaments.data.length === 0 ? (
                  <div className="feed-empty-hint">No recent tournament podiums recorded.</div>
                ) : (
                  data.recentTournaments.data.map((t) => {
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
                          {t.finished && <span className="tourney-date">{formatDay(t.finished)}</span>}
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
          availableTournaments={data.availableTournaments.kind === 'ready' ? data.availableTournaments.data : []}
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

