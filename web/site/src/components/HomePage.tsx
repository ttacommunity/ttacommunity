/** The front page's community feed: recent matches of the chosen tournaments and players, and recent podiums, behind spoiler covers. */
import { useEffect, useState, type MouseEvent } from 'react';
import { currentRun, leaderboard, catalogue, recentPodiums, type Run } from '@ttac/shared/ratings';
import { formatDay } from '@ttac/shared/time';
import { podiumCard, seasonName, type PodiumCard } from '../lib/catalogue-view';
import { errorText, settled, type Load } from '@ttac/shared/load';
import { score } from '@ttac/shared/numbers';
import { useCopy } from '@ttac/shared/copy';
import { browserStorage } from '../lib/account';
import { seatRankClass, toggled } from '../lib/home-view';
import { waitingText } from '../lib/waiting';

import {
  DEFAULT_FEED_SETTINGS,
  loadFeedSettings,
  saveFeedSettings,
  withoutPlayer,
  withoutTournament,
  type FeedSettings,
} from '../lib/feed-settings';
import { fetchFeedMatches, type FeedMatch } from '../lib/feed-data';
import FeedOptionsModal from './FeedOptionsModal';
import PlayerLink from './PlayerLink';
import { useLoad } from './useLoad';

interface HomeData {
  run: Run;
  recentTournaments: Load<PodiumCard[]>;
}

/** One-click copyable replay code */
function ReplayBadge({ code }: { code: string | null }) {
  const { copied, copy } = useCopy();
  if (!code) return null;
  return (
    <button
      type="button"
      className="home-replay-btn"
      onClick={() => copy(code)}
      title="Click to copy CGE replay code"
      aria-label={`Copy replay code ${code}`}
    >
      <code>{code}</code>
      <span className="home-replay-hint">{copied === true ? '✓ Copied' : 'Copy'}</span>
    </button>
  );
}

interface Spoiler {
  shown: boolean;
  /** What the button hides or reveals, as its title says it: "match scores", "podium". */
  what: string;
  onToggle: () => void;
}

/** SpoilerButton hides or reveals every item in a list at once. */
function SpoilerButton({ shown, what, onToggle }: Spoiler) {
  return (
    <button type="button" className="spoiler-toggle-btn" onClick={onToggle} title={`${shown ? 'Hide' : 'Reveal'} all ${what}`}>
      {shown ? '👁️ Spoilers: Shown' : '🔒 Spoilers: Hidden'}
    </button>
  );
}

/** RevealButton hides or reveals one item. */
function RevealButton({ shown, what, onToggle }: Spoiler) {
  const label = `${shown ? 'Hide' : 'Reveal'} ${what}`;
  return (
    <button type="button" className="spoiler-cell-btn" onClick={onToggle} title={label} aria-label={label}>
      {shown ? 'Hide' : 'Reveal'}
    </button>
  );
}

export default function HomePage({ apiUrl }: { apiUrl: string }) {
  const [load, setLoad] = useState<Load<HomeData>>({ kind: 'loading' });

  // Feed configuration and match state
  const [feedSettings, setFeedSettings] = useState<FeedSettings>(DEFAULT_FEED_SETTINGS);
  const [optionsOpener, setOptionsOpener] = useState<HTMLElement | null>(null);
  const [optionsOpened, setOptionsOpened] = useState(false);
  const [feedMatches, setFeedMatches] = useState<Load<FeedMatch[]>>({ kind: 'loading' });

  // Spoilers state
  const [globalMatchSpoilers, setGlobalMatchSpoilers] = useState(false); // false = hidden, true = revealed
  const [revealedMatchIds, setRevealedMatchIds] = useState<Set<number>>(new Set());

  const [globalTourneySpoilers, setGlobalTourneySpoilers] = useState(false); // false = hidden, true = revealed
  const [revealedTourneyKeys, setRevealedTourneyKeys] = useState<Set<string>>(new Set());

  useEffect(() => setFeedSettings(loadFeedSettings(browserStorage())), []);

  useEffect(() => {
    currentRun(apiUrl)
      .then(async (run) => {
        const podiums = await settled(recentPodiums(apiUrl, run.run_id).then((p) => p.podiums.map(podiumCard)));
        setLoad({ kind: 'ready', data: { run, recentTournaments: podiums } });
      })
      .catch((e: unknown) => {
        setLoad({ kind: 'error', message: errorText(e) });
      });
  }, [apiUrl]);

  // Matches are asked for again whenever the run or the feed settings change.
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

  // The options dialog's tournaments and players are asked for when it first opens, and kept.
  const choosingIn = optionsOpened && data !== null ? data.run.run_id : null;
  const tournaments = useLoad(
    String(choosingIn),
    choosingIn === null ? null : () => catalogue(apiUrl, choosingIn).then((c) => c.tournaments.map((t) => ({ slug: t.slug, name: t.name }))),
  );
  const players = useLoad(
    String(choosingIn),
    choosingIn === null
      ? null
      : () => leaderboard(apiUrl, choosingIn, 'glicko2_daneo', 0).then((b) => b.players.map((r) => ({ name: r.name, country: r.country, title: r.title }))),
  );
  const openOptions = (e: MouseEvent<HTMLButtonElement>) => {
    setOptionsOpened(true);
    setOptionsOpener(e.currentTarget);
  };

  const changeSettings = (next: FeedSettings) => {
    setFeedSettings(next);
    saveFeedSettings(browserStorage(), next);
  };
  const toggleMatchSpoiler = (matchId: number) => setRevealedMatchIds((prev) => toggled(prev, matchId));
  const toggleTourneySpoiler = (key: string) => setRevealedTourneyKeys((prev) => toggled(prev, key));

  return (
    <>
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
                    onClick={openOptions}
                    title="Customize tournaments, divisions, and players in feed"
                    aria-label="Feed filter options"
                  >
                    <span>⚙️</span> Options
                  </button>
                  <SpoilerButton shown={globalMatchSpoilers} what="match scores" onToggle={() => setGlobalMatchSpoilers(!globalMatchSpoilers)} />
                </div>
              </div>
              <p className="feed-card-intro">
                Recently finished tournament matches featuring your selected divisions and competitors.
              </p>
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
                          onClick={() => changeSettings(withoutTournament(feedSettings, t.slug, t.division))}
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
                          onClick={() => changeSettings(withoutPlayer(feedSettings, name))}
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
                      onClick={openOptions}
                    >
                      <span>⚙️</span> Filter / Add
                    </button>
                    <button
                      type="button"
                      className="btn-antique reset-top-btn"
                      onClick={() => changeSettings(DEFAULT_FEED_SETTINGS)}
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
                                    {isRevealed ? `(${score(s.score)})` : '(???)'}
                                  </span>
                                  {idx < m.seats.length - 1 && <span className="match-sep">vs</span>}
                                </span>
                              );
                            })}
                          </div>

                          <RevealButton shown={isRevealed} what="scores" onToggle={() => toggleMatchSpoiler(m.matchId)} />
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
                <SpoilerButton shown={globalTourneySpoilers} what="tournament podiums" onToggle={() => setGlobalTourneySpoilers(!globalTourneySpoilers)} />
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
                              aria-hidden="true"
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

                          <RevealButton shown={isRevealed} what="podium" onToggle={() => toggleTourneySpoiler(key)} />
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

      {optionsOpener !== null && data && (
        <FeedOptionsModal
          opener={optionsOpener}
          apiUrl={apiUrl}
          runId={data.run.run_id}
          availableTournaments={tournaments}
          searchPlayers={players}
          settings={feedSettings}
          onSave={changeSettings}
          onClose={() => setOptionsOpener(null)}
        />
      )}
    </>
  );
}
