/** A tournament's seasons: counts, settings, notes and podiums. */
import { useState, type FormEvent } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournament } from '../lib/admin-api';
import { seasonSummary, stageRows } from '../lib/season-counts';
import { fieldId, hasPenalty, renamedFrom, toggled } from '../lib/tournament-page';
import { refusalOf, shownAt, type Outcome } from '../lib/outcome';
import NoteBox from './NoteBox';
import Notice from './Notice';
import PenaltyBox from './PenaltyBox';
import PodiumBox from './PodiumBox';
import StagesTable from './StagesTable';

/** SeasonsCard lists a tournament's seasons with their counts, settings, notes and podiums, each season's stages on
 * demand, and adds or removes a season without games. */
export default function SeasonsCard({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [refusal, setRefusal] = useState<Outcome | null>(null);
  const close = () => setRefusal(null);
  const [shown, setShown] = useState<string[]>([]);
  const toggle = (label: string) => setShown((s) => toggled(s, label));
  const [newSeason, setNewSeason] = useState('');
  const addSeason = async (e: FormEvent) => {
    e.preventDefault();
    setRefusal(null);
    const r = await req.send(req.addSeason(t.slug, newSeason));
    setRefusal(refusalOf(r, 'add season'));
    if (r.ok) {
      setNewSeason('');
      await onReload();
    }
  };
  const setFlag = async (label: string, flag: req.SeasonFlag, on: boolean) => {
    setRefusal(null);
    const r = await req.send(req.setSeasonFlag(t.slug, label, flag, on));
    setRefusal(refusalOf(r, `season ${label}`));
    if (r.ok) await onReload();
  };
  const removeSeason = async (label: string) => {
    if (!window.confirm(`Remove ${t.name} season ${label} and its notes?`)) return;
    setRefusal(null);
    const r = await req.send(req.removeSeason(t.slug, label));
    setRefusal(refusalOf(r, `season ${label}`));
    if (r.ok) await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="seasons-heading">
      <h2 id="seasons-heading" className="rating-card-heading">
        Seasons
      </h2>
      <form className="season-add" onSubmit={(e) => void addSeason(e)}>
        <label>
          Add a season with no games yet (its label as the site shows it, without "Season")
          <input className="input-antique" value={newSeason} maxLength={64} placeholder="e.g. 1 or 2023" onChange={(e) => setNewSeason(e.target.value)} />
        </label>
        <button type="submit" className="btn-antique" disabled={newSeason.trim() === ''}>
          Add season
        </button>
      </form>
      <Notice outcome={shownAt(refusal, 'add season')} onClose={close} />
      {t.seasons.length === 0 && <p className="muted">No seasons yet.</p>}
      {t.seasons.map((se) => {
        const sum = seasonSummary(se);
        return (
          <div key={se.label} className="season-notes">
            <h3>
              Season {se.label}
              {renamedFrom(se.label, se.original_name) !== null && (
                <span className="muted"> · imported as {renamedFrom(se.label, se.original_name)}</span>
              )}
            </h3>
            <p className="season-summary">
              <span className="muted">
                {sum.stages} {sum.stages === 1 ? 'stage' : 'stages'} · {sum.players} players · {sum.games} games
              </span>{' '}
              {sum.stages > 0 && (
                <button type="button" className="btn-antique btn-settings" aria-expanded={shown.includes(se.label)} onClick={() => toggle(se.label)}>
                  {shown.includes(se.label) ? 'Hide stages' : 'Show stages'}
                </button>
              )}
            </p>
            {shown.includes(se.label) && <StagesTable slug={t.slug} season={se.label} rows={stageRows(se.stages)} onReload={onReload} />}
            <label className="season-rated">
              <input
                id={fieldId('season-rated', se.label)}
                type="checkbox"
                checked={se.rated}
                onChange={(e) => void setFlag(se.label, 'rated', e.target.checked)}
              />{' '}
              Counts towards ratings <span className="muted">(from the next engine run; its games still show, marked not rated)</span>
            </label>
            <label className="season-rated">
              <input
                id={fieldId('season-champions', se.label)}
                type="checkbox"
                checked={se.in_champions}
                onChange={(e) => void setFlag(se.label, 'champions', e.target.checked)}
              />{' '}
              Winner listed among the champions <span className="muted">(such as the Hall of Fame's World Champions; untick a play-off)</span>
            </label>
            <label className="season-rated">
              <input
                id={fieldId('season-community', se.label)}
                type="checkbox"
                checked={se.in_community}
                onChange={(e) => void setFlag(se.label, 'community', e.target.checked)}
              />{' '}
              Counts in the community leaderboard{' '}
              <span className="muted">(from the next engine run, when the tournament counts; untick a play-off that should not score)</span>
            </label>
            {!se.has_games && (
              <div className="season-empty">
                <span className="muted">No games recorded.</span>
                <button type="button" className="btn-antique" onClick={() => void removeSeason(se.label)}>
                  Remove season
                </button>
              </div>
            )}
            <Notice outcome={shownAt(refusal, `season ${se.label}`)} onClose={close} />
            {hasPenalty(t.kind, se.label) && <PenaltyBox slug={t.slug} season={se.label} initial={se.timeout_penalty} />}
            <NoteBox slug={t.slug} season={se.label} stage="" initial={se.note} />
            <PodiumBox
              slug={t.slug}
              season={se.label}
              saved={se.podium}
              savedNote={se.podium_note}
              suggested={se.suggested_podium}
              suggestedFrom={se.suggested_from}
              onReload={onReload}
            />
          </div>
        );
      })}
    </section>
  );
}
