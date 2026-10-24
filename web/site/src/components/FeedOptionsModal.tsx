/** Modal dialog allowing users to customize their community pulse feed tournaments, divisions, and players. */
import { useEffect, useRef, useState } from 'react';
import type { FeedSettings, TournamentFeedFilter } from '../lib/feed-settings';
import { DEFAULT_FEED_SETTINGS } from '../lib/feed-settings';
import { divisionsToFetch, fetchTournamentDivisions, PREMIER_DIVISIONS } from '../lib/feed-data';
import SearchBox, { type SearchPlayer } from './SearchBox';
import './FeedOptionsModal.css';

export interface AvailableTournament {
  slug: string;
  name: string;
}

interface FeedOptionsModalProps {
  apiUrl: string;
  runId: number;
  availableTournaments: AvailableTournament[];
  searchPlayers: SearchPlayer[];
  settings: FeedSettings;
  onSave: (nextSettings: FeedSettings) => void;
  onClose: () => void;
}

export default function FeedOptionsModal({
  apiUrl,
  runId,
  availableTournaments,
  searchPlayers,
  settings,
  onSave,
  onClose,
}: FeedOptionsModalProps) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const [staged, setStaged] = useState<FeedSettings>(settings);
  const [divisionsMap, setDivisionsMap] = useState<Record<string, string[]>>(PREMIER_DIVISIONS);
  const [divisionsState, setDivisionsState] = useState<Record<string, 'loading' | 'ready' | 'failed'>>({});
  const askedDivisions = useRef(new Set(Object.keys(PREMIER_DIVISIONS)));

  useEffect(() => {
    const d = dialogRef.current;
    if (!d) return;
    d.showModal();

    const onBackdrop = (e: MouseEvent) => {
      if (e.target === d) d.close();
    };
    d.addEventListener('click', onBackdrop);

    return () => {
      d.removeEventListener('click', onBackdrop);
    };
  }, []);

  useEffect(() => {
    for (const slug of divisionsToFetch(staged.tournaments, askedDivisions.current)) {
      askedDivisions.current.add(slug);
      setDivisionsState((prev) => ({ ...prev, [slug]: 'loading' }));
      fetchTournamentDivisions(apiUrl, runId, slug).then(
        (groups) => {
          setDivisionsMap((prev) => ({ ...prev, [slug]: groups }));
          setDivisionsState((prev) => ({ ...prev, [slug]: 'ready' }));
        },
        () => setDivisionsState((prev) => ({ ...prev, [slug]: 'failed' })),
      );
    }
  }, [apiUrl, runId, staged.tournaments]);

  const addTournament = (slug: string) => {
    if (!slug) return;
    const tourney = availableTournaments.find((t) => t.slug === slug);
    if (!tourney) return;

    // Pick the first unused division or premier division default
    const used = new Set(staged.tournaments.filter((t) => t.slug === slug).map((t) => t.division ?? ''));
    const divisions = divisionsMap[slug] || PREMIER_DIVISIONS[slug] || [];
    const defaultDiv = divisions.find((d) => !used.has(d)) ?? PREMIER_DIVISIONS[slug]?.[0] ?? '';
    const newEntry: TournamentFeedFilter = {
      slug: tourney.slug,
      name: tourney.name,
      division: defaultDiv,
    };

    setStaged((prev) => ({
      ...prev,
      tournaments: [...prev.tournaments, newEntry],
    }));
  };

  const addDivision = (slug: string) => {
    addTournament(slug);
  };

  const removeTournament = (index: number) => {
    setStaged((prev) => ({
      ...prev,
      tournaments: prev.tournaments.filter((_, i) => i !== index),
    }));
  };

  const changeDivision = (index: number, division: string) => {
    setStaged((prev) => ({
      ...prev,
      tournaments: prev.tournaments.map((t, i) =>
        i === index ? { ...t, division } : t,
      ),
    }));
  };

  const addPlayer = (name: string) => {
    const trimmed = name.trim();
    if (!trimmed) return;
    if (!staged.players.some((p) => p.toLowerCase() === trimmed.toLowerCase())) {
      setStaged((prev) => ({
        ...prev,
        players: [...prev.players, trimmed],
      }));
    }
  };

  const removePlayer = (name: string) => {
    setStaged((prev) => ({
      ...prev,
      players: prev.players.filter((p) => p.toLowerCase() !== name.toLowerCase()),
    }));
  };

  const resetToPremier = () => {
    setStaged(DEFAULT_FEED_SETTINGS);
  };

  const clearAll = () => {
    setStaged({ tournaments: [], players: [] });
  };

  const handleApply = () => {
    onSave(staged);
    dialogRef.current?.close();
  };

  return (
    <dialog
      ref={dialogRef}
      className="feed-options-modal"
      aria-labelledby="feed-modal-title"
      onClose={onClose}
    >
      <div className="feed-modal-head">
        <h3 id="feed-modal-title" className="feed-modal-title">
          <span>⚙️</span> Customize Community Feed
        </h3>
        <button
          type="button"
          className="feed-modal-close"
          aria-label="Close"
          onClick={() => dialogRef.current?.close()}
        >
          ×
        </button>
      </div>

      <div className="feed-modal-body">
        <p className="feed-modal-intro">
          Choose which tournaments and divisions appear in your Recent Matches feed, or highlight individual competitors.
        </p>

        {/* Section 1: Tournaments & Divisions */}
        <section className="feed-modal-section" aria-labelledby="tourneys-heading">
          <h4 id="tourneys-heading" className="feed-section-heading">
            <span>🏛️ Tournaments &amp; Divisions</span>
            <span className="feed-section-hint">Premier circuits preselected</span>
          </h4>

          <div className="feed-tournaments-list">
            {staged.tournaments.length === 0 ? (
              <div className="feed-empty-hint-small">No tournaments selected. Add one below or reset to premier.</div>
            ) : (
              staged.tournaments.map((t, idx) => {
                const divisions = divisionsMap[t.slug] || [];
                const state = divisionsState[t.slug];

                return (
                  <div key={`${t.slug}-${t.division || 'all'}-${idx}`} className="feed-tourney-row">
                    <div className="feed-tourney-info">
                      <span className="feed-tourney-name" title={t.name}>
                        {t.name}
                      </span>
                    </div>

                    <div className="feed-tourney-controls">
                      <select
                        className="feed-division-select"
                        value={t.division ?? ''}
                        onChange={(e) => changeDivision(idx, e.target.value)}
                        aria-label={`Division for ${t.name}`}
                      >
                        <option value="">All Divisions</option>
                        {divisions.map((d) => (
                          <option key={d} value={d}>
                            {d}
                          </option>
                        ))}
                        {t.division && !divisions.includes(t.division) && (
                          <option value={t.division}>{t.division}</option>
                        )}
                        {state === 'loading' && <option value="" disabled>Loading divisions…</option>}
                        {state === 'failed' && <option value="" disabled>Divisions could not be loaded</option>}
                      </select>

                      <button
                        type="button"
                        className="feed-btn-add-division"
                        onClick={() => addDivision(t.slug)}
                        title={`Add another division for ${t.name}`}
                      >
                        + Division
                      </button>

                      <button
                        type="button"
                        className="feed-row-remove"
                        onClick={() => removeTournament(idx)}
                        title={`Remove ${t.name}${t.division ? ` (${t.division})` : ''}`}
                        aria-label={`Remove ${t.name}${t.division ? ` (${t.division})` : ''}`}
                      >
                        ✕
                      </button>
                    </div>
                  </div>
                );
              })
            )}

            {availableTournaments.length > 0 && (
              <div className="feed-add-tourney-row">
                <select
                  className="feed-add-tourney-select"
                  value=""
                  onChange={(e) => {
                    addTournament(e.target.value);
                    e.target.value = '';
                  }}
                  aria-label="Add tournament to feed"
                >
                  <option value="" disabled>+ Add Tournament / Division to Feed…</option>
                  {availableTournaments.map((item) => (
                    <option key={item.slug} value={item.slug}>
                      {item.name}
                    </option>
                  ))}
                </select>
              </div>
            )}
          </div>
        </section>

        {/* Section 2: Individual Players */}
        <section className="feed-modal-section" aria-labelledby="players-heading">
          <h4 id="players-heading" className="feed-section-heading">
            <span>👤 Highlighted Competitors</span>
            <span className="feed-section-hint">Optional player tracking</span>
          </h4>

          <SearchBox
            players={searchPlayers}
            value=""
            placeholder="Search and add competitor..."
            onSearch={addPlayer}
            clearOnSearch
          />

          <div className="player-tags-container" aria-label="Selected competitors">
            {staged.players.length === 0 ? (
              <div className="feed-empty-hint-small">No specific players tracked.</div>
            ) : (
              staged.players.map((name) => (
                <span key={name} className="player-filter-tag">
                  <span className="player-tag-name">{name}</span>
                  <button
                    type="button"
                    className="player-tag-remove"
                    onClick={() => removePlayer(name)}
                    title={`Remove ${name}`}
                    aria-label={`Remove ${name}`}
                  >
                    ✕
                  </button>
                </span>
              ))
            )}
          </div>
        </section>
      </div>

      <div className="feed-modal-footer">
        <div className="feed-footer-left">
          <button
            type="button"
            className="feed-btn-reset"
            onClick={resetToPremier}
            title="Preselect top divisions of IM, IC and RL"
          >
            Reset to Premier
          </button>
          <button
            type="button"
            className="feed-btn-reset"
            onClick={clearAll}
            title="Clear all filters"
          >
            Clear All
          </button>
        </div>

        <div className="feed-footer-right">
          <button
            type="button"
            className="feed-btn-cancel"
            onClick={() => dialogRef.current?.close()}
          >
            Cancel
          </button>
          <button
            type="button"
            className="btn-antique feed-btn-apply"
            onClick={handleApply}
          >
            Apply &amp; Save
          </button>
        </div>
      </div>
    </dialog>
  );
}
