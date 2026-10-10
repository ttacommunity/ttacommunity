/** A tournament's or group's page: the groups it is in; a group's settings and members, or a tournament's settings, format and regulations, and seasons. */
import {
  addedStory,
  canAddStory,
  cardChanged,
  editedStory,
  isRemoval,
  movedStory,
  regulationsSavable,
  regulationsSource,
  regulationsStart,
  removedStory,
  storiesChanged,
  textChanged,
} from '../lib/admin-edits';
import { Fragment, useEffect, useState, type FormEvent } from 'react';
import * as req from '../lib/admin-requests';
import { adminRequest, type AdminMe, type AdminTournament, type AdminTournamentRow, type Load } from '../lib/admin-api';
import { podiumNoteStart, podiumRequest, podiumStart, type PodiumEntry } from '../lib/podium';
import { seasonSummary, stageRows } from '../lib/season-counts';
import {
  divisionList,
  divisionsChanged,
  divisionText,
  fieldId,
  hasPenalty,
  noteAction,
  patchReady,
  penaltySavable,
  penaltyValue,
  pointsText,
  renamedFrom,
  levelNamesPatch,
  scoringPatch,
  SETTINGS,
  slugParam,
  toggled,
  toggledCounts,
  tournamentLoad,
} from '../lib/tournament-page';
import { tabsFor } from '../lib/admin-tabs';
import AdminHeader from './AdminHeader';
import GroupPage from './GroupPage';
import RelatedCard from './RelatedCard';
import './AdminApp.css';

/** NoteBox edits the note shown beside a season (stage '') or one of its stages. */
function NoteBox({
  slug,
  season,
  stage,
  initial,
  original = null,
  onSaved,
}: {
  slug: string;
  season: string;
  stage: string;
  initial: string;
  original?: string | null;
  onSaved?: () => void;
}) {
  const [text, setText] = useState(initial);
  const [saved, setSaved] = useState(initial);
  const [status, setStatus] = useState('');
  const id = fieldId('note', slug, season, stage);
  const save = async () => {
    setStatus('Saving…');
    const r = await req.send(req.setNote(slug, season, stage, text));
    if (r.ok) {
      setSaved(text.trim());
      setStatus(isRemoval(text) ? 'Removed' : 'Saved');
      onSaved?.();
    } else {
      setStatus(r.message);
    }
  };
  return (
    <div className="note-box">
      <label htmlFor={id}>
        {stage === '' ? 'Season note' : `${stage} note`}
        {original !== null && <span className="muted"> · imported as {original}</span>}
      </label>
      <textarea className="input-antique" id={id} value={text} maxLength={2000} rows={2} onChange={(e) => setText(e.target.value)} />
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={save} disabled={!textChanged(saved, text)}>
          Save note
        </button>
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </div>
  );
}

/** PenaltyBox edits the points a numbered season's standings take off for each game a player times out of. */
function PenaltyBox({ slug, season, initial }: { slug: string; season: string; initial: number }) {
  const [text, setText] = useState(String(initial));
  const [saved, setSaved] = useState(initial);
  const [status, setStatus] = useState('');
  const id = fieldId('penalty', slug, season);
  const value = penaltyValue(text);
  const save = async () => {
    if (value === null) return;
    setStatus('Saving…');
    const r = await req.send(req.setPenalty(slug, season, value));
    if (r.ok) {
      setSaved(value);
      setStatus('Saved');
    } else {
      setStatus(r.message);
    }
  };
  return (
    <div className="setting">
      <label htmlFor={id}>
        Time-out penalty <span className="muted">(points per game timed out of, 0 or below, from the next engine run)</span>
      </label>{' '}
      <input className="input-antique" id={id} value={text} maxLength={8} inputMode="decimal" onChange={(e) => setText(e.target.value)} />{' '}
      <button type="button" className="btn-antique" onClick={save} disabled={!penaltySavable(value, saved)}>
        Save
      </button>{' '}
      <span className="note-status" role="status">
        {value === null ? 'A number, 0 or below' : status}
      </span>
    </div>
  );
}

const PLACES = ['1st', '2nd', '3rd'];

// What the box's legend says of where its names came from.
const ORIGIN = {
  admin: '(set by hand)',
  recorded: '(as recorded by the organisers: set it to keep it here)',
  final_stage: "(suggested from the final stage's games: check it, then set it)",
  none: '(nothing recorded or suggested)',
};

/** PodiumBox sets a season's podium by hand, over the recorded or derived one, or removes it. */
function PodiumBox({
  slug,
  season,
  saved,
  savedNote,
  suggested,
  suggestedFrom,
  onReload,
}: {
  slug: string;
  season: string;
  saved: PodiumEntry[];
  savedNote: string;
  suggested: PodiumEntry[];
  suggestedFrom: string;
  onReload: () => Promise<void>;
}) {
  const start = podiumStart(saved, suggested, suggestedFrom);
  const [names, setNames] = useState(start.names);
  const [note, setNote] = useState(podiumNoteStart(savedNote, start.origin));
  const [status, setStatus] = useState('');
  const id = fieldId('podium', slug, season);
  const save = async () => {
    const body = podiumRequest(names, note);
    if (body === null) return;
    setStatus('Saving…');
    const r = await req.send(req.setPodium(slug, season, body));
    setStatus(r.ok ? 'Saved. Titles follow at the next run.' : r.message);
    if (r.ok) await onReload();
  };
  const clear = async () => {
    setStatus('Removing…');
    const r = await req.send(req.clearPodium(slug, season));
    setStatus(r.ok ? 'Removed. The recorded or derived podium shows again.' : r.message);
    if (r.ok) await onReload();
  };
  return (
    <fieldset className="podium-box">
      <legend>
        Season {season} podium {ORIGIN[start.origin]}
      </legend>
      <div className="podium-places">
        {PLACES.map((label, i) => (
          <label key={label}>
            {label}
            <input className="input-antique"
              id={`${id}-${i + 1}`}
              value={names[i]}
              maxLength={64}
              onChange={(e) => setNames(names.map((n, j) => (j === i ? e.target.value : n)))}
            />
          </label>
        ))}
        <label className="podium-note">
          Why, or where it comes from
          <input className="input-antique" id={`${id}-note`} value={note} maxLength={500} onChange={(e) => setNote(e.target.value)} />
        </label>
      </div>
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={podiumRequest(names, note) === null}>
          Set podium
        </button>
        {saved.length > 0 && (
          <button type="button" className="btn-antique" onClick={() => void clear()}>
            Use the results again
          </button>
        )}
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </fieldset>
  );
}

/** SettingsCard switches a tournament's settings, saving each at once. */
function SettingsCard({ t, onChange }: { t: AdminTournament; onChange: (t: AdminTournament) => void }) {
  const [error, setError] = useState('');
  const [divisions, setDivisions] = useState(divisionText(t.top_divisions));
  const [points, setPoints] = useState({ 2: pointsText(t.points_2p), 3: pointsText(t.points_3p), 4: pointsText(t.points_4p) });
  const [levels, setLevels] = useState({ season_name: t.season_name, stage_name: t.stage_name });
  const scoring = scoringPatch(points, t);
  const levelChange = levelNamesPatch(levels, t);
  // Shows the change at once, and puts the tournament back if the API refuses it.
  const patch = async (change: Partial<AdminTournament>) => {
    setError('');
    onChange({ ...t, ...change });
    const r = await req.send(req.setTournament(t.slug, change));
    if (!r.ok) {
      onChange(t);
      setError(r.message);
    }
  };
  const setCount = (n: number, on: boolean) => {
    const next = toggledCounts(t.rated_player_counts, n, on);
    if (next === null) setError('At least one player count stays rated; to rate none, untick Rated above.');
    else void patch({ rated_player_counts: next });
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="settings-heading">
      <h2 id="settings-heading" className="rating-card-heading">
        Settings
      </h2>
      <p className="muted">Changes save at once and are recorded with your name. Rating changes apply from the next engine run.</p>
      <div className="settings-list">
        {SETTINGS.map((s) => (
          <div key={s.key} className="setting">
            <input
              id={`setting-${s.key}`}
              type="checkbox"
              checked={t[s.key]}
              onChange={(e) => void patch({ [s.key]: e.target.checked })}
            />
            <label htmlFor={`setting-${s.key}`}>
              <strong>{s.label}</strong>
              <span className="muted"> · {s.hint}</span>
            </label>
          </div>
        ))}
        <fieldset className="setting-counts">
          <legend>
            <strong>Rated player counts</strong>
            <span className="muted"> · Only these games count towards ratings, from the next engine run</span>
          </legend>
          {[2, 3, 4].map((n) => (
            <label key={n}>
              <input
                id={`rated-count-${n}`}
                type="checkbox"
                checked={t.rated_player_counts.includes(n)}
                onChange={(e) => setCount(n, e.target.checked)}
              />{' '}
              {n}-player
            </label>
          ))}
        </fieldset>
        <div className="setting">
          <label htmlFor="setting-kind">
            <strong>Kind</strong>
            <span className="muted"> · A league's podium is its top divisions' standings; a ladder has none</span>
          </label>{' '}
          <select className="input-antique" id="setting-kind" value={t.kind} onChange={(e) => void patch({ kind: e.target.value as AdminTournament['kind'] })}>
            <option value="league">League</option>
            <option value="cup">Cup</option>
            <option value="ladder">Ladder</option>
          </select>
        </div>
        {t.kind === 'league' && (
          <div className="setting">
            <label htmlFor="setting-top">
              <strong>Top divisions</strong>
              <span className="muted"> · Comma-separated; the champions table follows at once</span>
            </label>{' '}
            <input className="input-antique" id="setting-top" value={divisions} maxLength={300} onChange={(e) => setDivisions(e.target.value)} />{' '}
            <button
              type="button"
              className="btn-antique"
              onClick={() => void patch({ top_divisions: divisionList(divisions) })}
              disabled={!divisionsChanged(divisions, t.top_divisions)}
            >
              Save
            </button>
          </div>
        )}
        <fieldset className="setting-counts">
          <legend>
            <strong>Points per place</strong>
            <span className="muted">
              {' '}
              · Best place first, such as 5-2-0; empty where it does not score that player count. Standings use them from the next engine run
            </span>
          </legend>
          {([2, 3, 4] as const).map((n) => (
            <label key={n}>
              {n}-player{' '}
              <input className="input-antique"
                id={`points-${n}`}
                value={points[n]}
                maxLength={40}
                onChange={(e) => setPoints({ ...points, [n]: e.target.value })}
              />
            </label>
          ))}{' '}
          <button
            type="button"
            className="btn-antique"
            onClick={() => {
              if (scoring !== null) void patch(scoring);
            }}
            disabled={!patchReady(scoring)}
          >
            Save
          </button>
          {scoring === null && <span className="muted"> One number per place, never higher for a worse place</span>}
        </fieldset>
        <fieldset className="setting-counts">
          <legend>
            <strong>Words on its pages</strong>
            <span className="muted"> · What it calls a season and a stage, such as Edition or Round; empty for the defaults</span>
          </legend>
          <label>
            A season is called{' '}
            <input className="input-antique"
              id="level-season"
              value={levels.season_name}
              placeholder="Season"
              maxLength={32}
              onChange={(e) => setLevels({ ...levels, season_name: e.target.value })}
            />
          </label>
          <label>
            A stage is called{' '}
            <input className="input-antique"
              id="level-stage"
              value={levels.stage_name}
              placeholder="Stage"
              maxLength={32}
              onChange={(e) => setLevels({ ...levels, stage_name: e.target.value })}
            />
          </label>{' '}
          <button
            type="button"
            className="btn-antique"
            onClick={() => {
              if (levelChange !== null) void patch(levelChange);
            }}
            disabled={!patchReady(levelChange)}
          >
            Save
          </button>
        </fieldset>
      </div>
      {error !== '' && (
        <p className="row-error" role="alert">
          {error}
        </p>
      )}
    </section>
  );
}

/** CardSettings edits how the tournament presents itself on its card and its page's header. */
function CardSettings({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [card, setCard] = useState(t.card);
  const [status, setStatus] = useState('');
  const changed = cardChanged(t.card, card);
  const save = async () => {
    setStatus('Saving…');
    const r = await req.send(req.setCard(t.slug, card));
    setStatus(r.ok ? 'Saved. The cards and the tournament page show it at once.' : r.message);
    if (r.ok) await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="card-heading">
      <h2 id="card-heading" className="rating-card-heading">
        Card
      </h2>
      <p className="muted">
        How the tournament shows on the Hall of Fame and Every Tournament, and at the top of its own page. Leave a box empty
        to leave it out; with no icon it shows 📜.
      </p>
      <div className="card-fields">
        <label>
          Icon
          <input className="input-antique" id="card-icon" value={card.icon} maxLength={8} placeholder="📜" onChange={(e) => setCard({ ...card, icon: e.target.value })} />
        </label>
        <label className="card-tagline">
          Short description
          <input className="input-antique"
            id="card-tagline"
            value={card.tagline}
            maxLength={60}
            placeholder="e.g. Annual World Championship"
            onChange={(e) => setCard({ ...card, tagline: e.target.value })}
          />
        </label>
        <label className="card-tagline">
          Page subtitle
          <input className="input-antique"
            id="card-subtitle"
            value={card.subtitle}
            maxLength={60}
            placeholder="e.g. 4-Player Competitive Format"
            onChange={(e) => setCard({ ...card, subtitle: e.target.value })}
          />
        </label>
        <label className="card-tagline">
          Cadence
          <input className="input-antique"
            id="card-cadence"
            value={card.cadence}
            maxLength={60}
            placeholder="e.g. Annual (Every Year)"
            onChange={(e) => setCard({ ...card, cadence: e.target.value })}
          />
        </label>
        <label className="card-summary">
          Information box
          <textarea className="input-antique"
            id="card-summary"
            value={card.summary}
            maxLength={1000}
            rows={4}
            onChange={(e) => setCard({ ...card, summary: e.target.value })}
          />
        </label>
      </div>
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={!changed}>
          Save card
        </button>
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </section>
  );
}

// What the card says of the regulations the site shows now.
const REGULATIONS_SHOWN = {
  ours: "Written here, and shown on the site instead of CGE's description.",
  cge: "The site shows the latest edition's description on CGE, below. Edit it and save to show your own.",
  none: 'Neither written here nor on CGE: the site shows no box. Write it below.',
};

/** RegulationsCard edits the format and regulations the site shows, starting from CGE's text when none is written. */
function RegulationsCard({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [text, setText] = useState(regulationsStart(t.regulations, t.cge_regulations));
  const [status, setStatus] = useState('');
  const source = regulationsSource(t.regulations, t.cge_regulations);
  const save = async (value: string) => {
    setStatus('Saving…');
    const r = await req.send(req.setRegulations(t.slug, value));
    if (!r.ok) {
      setStatus(r.message);
      return;
    }
    setStatus(isRemoval(value) ? "Removed. The site shows CGE's text again." : 'Saved. The site shows it at once.');
    if (isRemoval(value)) setText(t.cge_regulations);
    await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="regulations-heading">
      <h2 id="regulations-heading" className="rating-card-heading">
        Format &amp; Regulations
      </h2>
      <p className="muted">
        {REGULATIONS_SHOWN[source]}
      </p>
      <textarea
        id="regulations-text"
        className="regulations-text"
        aria-labelledby="regulations-heading"
        value={text}
        maxLength={10000}
        rows={12}
        onChange={(e) => setText(e.target.value)}
      />
      <div className="note-actions">
        <button
          type="button"
          className="btn-antique"
          onClick={() => void save(text)}
          disabled={!regulationsSavable(text, t.regulations, t.cge_regulations)}
        >
          Save
        </button>
        {source === 'ours' && (
          <button type="button" className="btn-antique" onClick={() => void save('')}>
            {t.cge_regulations !== '' ? "Use CGE's text again" : 'Remove'}
          </button>
        )}
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </section>
  );
}

type Story = AdminTournament['stories'][number];

/** StoriesSettings edits the tournament's stories, as DANeo's pages tell them, and saves them all at once in this order. */
function StoriesSettings({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [list, setList] = useState<Story[]>(t.stories);
  const [status, setStatus] = useState('');
  const changed = storiesChanged(t.stories, list);
  const set = (i: number, field: keyof Story, value: string) => setList(editedStory(list, i, field, value));
  const move = (i: number, by: number) => setList(movedStory(list, i, by));
  const save = async () => {
    setStatus('Saving…');
    const r = await req.send(req.setStories(t.slug, list));
    setStatus(r.ok ? 'Saved. The tournament page shows them at once.' : r.message);
    if (r.ok) await onReload();
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="stories-heading">
      <h2 id="stories-heading" className="rating-card-heading">
        Stories
      </h2>
      <p className="muted">
        The story cards on the tournament page: a player, an optional badge beside them, a title and the story. Up to 12,
        shown in this order. With none, the page shows no stories section.
      </p>
      {list.map((st, i) => (
        <fieldset key={i} className="story-box">
          <legend>Story {i + 1}</legend>
          <div className="card-fields">
            <label>
              Player
              <input className="input-antique" id={`story-${i}-player`} value={st.player} maxLength={200} onChange={(e) => set(i, 'player', e.target.value)} />
            </label>
            <label>
              Badge
              <input className="input-antique"
                id={`story-${i}-badge`}
                value={st.badge}
                maxLength={60}
                placeholder="e.g. Reigning World Champion"
                onChange={(e) => set(i, 'badge', e.target.value)}
              />
            </label>
            <label className="card-tagline">
              Title
              <input className="input-antique" id={`story-${i}-title`} value={st.title} maxLength={120} onChange={(e) => set(i, 'title', e.target.value)} />
            </label>
            <label className="card-summary">
              Story
              <textarea className="input-antique" id={`story-${i}-text`} value={st.text} maxLength={1000} rows={3} onChange={(e) => set(i, 'text', e.target.value)} />
            </label>
          </div>
          <div className="note-actions">
            <button type="button" className="btn-antique" onClick={() => move(i, -1)} disabled={i === 0} aria-label={`Move story ${i + 1} up`}>
              ↑
            </button>
            <button
              type="button"
              className="btn-antique"
              onClick={() => move(i, 1)}
              disabled={i === list.length - 1}
              aria-label={`Move story ${i + 1} down`}
            >
              ↓
            </button>
            <button type="button" className="btn-antique" onClick={() => setList(removedStory(list, i))}>
              Remove
            </button>
          </div>
        </fieldset>
      ))}
      <div className="note-actions">
        <button type="button" className="btn-antique" onClick={() => setList(addedStory(list))} disabled={!canAddStory(list)}>
          Add a story
        </button>
        <button type="button" className="btn-antique" onClick={() => void save()} disabled={!changed}>
          Save stories
        </button>
        <span className="note-status" role="status">
          {status}
        </span>
      </div>
    </section>
  );
}

const NOTE_BUTTON = { close: 'Close', add: 'Add note', edit: 'Edit note' };

/** StagesTable lists a season's stages with their players, games and notes; a row's button opens its note to edit. */
function StagesTable({ slug, season, rows, onReload }: { slug: string; season: string; rows: AdminTournament['seasons'][number]['stages']; onReload: () => Promise<void> }) {
  const [editing, setEditing] = useState('');
  return (
    <div className="table-responsive">
      <table className="leaderboard-table admin-table stages-table">
        <caption className="visually-hidden">Season {season}'s stages</caption>
        <thead>
          <tr>
            <th scope="col">Stage</th>
            <th scope="col">Players</th>
            <th scope="col">Games</th>
            <th scope="col">Note</th>
            <th scope="col">
              <span className="visually-hidden">Action</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((st) => (
            <Fragment key={st.label}>
              <tr>
                <th scope="row">
                  {st.label}
                  {renamedFrom(st.label, st.original_name) !== null && (
                    <span className="row-detail muted">imported as {renamedFrom(st.label, st.original_name)}</span>
                  )}
                </th>
                <td>{st.players}</td>
                <td>{st.finished}</td>
                <td className="stage-note">{st.note}</td>
                <td>
                  <button
                    type="button"
                    className="btn-antique btn-settings"
                    aria-expanded={editing === st.label}
                    onClick={() => setEditing(editing === st.label ? '' : st.label)}
                  >
                    {NOTE_BUTTON[noteAction(editing === st.label, st.note)]}
                  </button>
                </td>
              </tr>
              {editing === st.label && (
                <tr>
                  <td colSpan={5}>
                    <NoteBox slug={slug} season={season} stage={st.label} initial={st.note} onSaved={() => void onReload()} />
                  </td>
                </tr>
              )}
            </Fragment>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** SeasonsCard lists a tournament's seasons with their counts, settings, notes and podiums, each season's stages on
 * demand, and adds or removes a season without games. */
function SeasonsCard({ t, onReload }: { t: AdminTournament; onReload: () => Promise<void> }) {
  const [error, setError] = useState('');
  const [shown, setShown] = useState<string[]>([]);
  const toggle = (label: string) => setShown((s) => toggled(s, label));
  const [newSeason, setNewSeason] = useState('');
  const addSeason = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    const r = await req.send(req.addSeason(t.slug, newSeason));
    if (r.ok) {
      setNewSeason('');
      await onReload();
    } else setError(r.message);
  };
  const setFlag = async (label: string, flag: req.SeasonFlag, on: boolean) => {
    setError('');
    const r = await req.send(req.setSeasonFlag(t.slug, label, flag, on));
    if (r.ok) await onReload();
    else setError(r.message);
  };
  const removeSeason = async (label: string) => {
    if (!window.confirm(`Remove ${t.name} season ${label} and its notes?`)) return;
    setError('');
    const r = await req.send(req.removeSeason(t.slug, label));
    if (r.ok) await onReload();
    else setError(r.message);
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
      {error !== '' && (
        <p className="row-error" role="alert">
          {error}
        </p>
      )}
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

/** TournamentSettings loads who is signed in and the tournament the address names, then its cards. */
export default function TournamentSettings() {
  const [me, setMe] = useState<Load<AdminMe>>({ kind: 'loading' });
  const [t, setT] = useState<Load<AdminTournament>>({ kind: 'loading' });
  const [all, setAll] = useState<AdminTournamentRow[]>([]);
  const load = async () => {
    const slug = slugParam(new URLSearchParams(window.location.search));
    if (slug === null) {
      setT({ kind: 'error', message: 'This address names no tournament. Go back to the list.' });
      return;
    }
    const [one, rows] = await Promise.all([
      adminRequest<AdminTournament>(`/api/tournaments/${encodeURIComponent(slug)}`, {}),
      adminRequest<AdminTournamentRow[]>('/api/tournaments', {}),
    ]);
    if (rows.ok) setAll(rows.data);
    setT(tournamentLoad(slug, one, rows));
  };
  useEffect(() => {
    void adminRequest<AdminMe>('/api/me', {}).then((r) =>
      setMe(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message }),
    );
    void load();
  }, []);

  return (
    <>
      <AdminHeader tabs={me.kind === 'ready' ? tabsFor(me.data.role) : []} current="tournaments" />
      <main className="admin-main">
      {me.kind === 'error' && <p className="row-error" role="alert">{me.message}</p>}
      {me.kind === 'ready' && (
        <p className="signed-in">
          Signed in as <strong>{me.data.name}</strong> ({me.data.role})
        </p>
      )}
      {t.kind === 'loading' && <p className="muted">Loading…</p>}
      {t.kind === 'error' && <p className="row-error" role="alert">{t.message}</p>}
      {t.kind === 'ready' && (
        <>
          <section className="rating-card admin-card page-header" aria-labelledby="page-title">
            <RelatedCard t={t.data} all={all} onReload={load} />
            <h2 id="page-title" className="tournament-title">
              {t.data.name}
            </h2>
            {renamedFrom(t.data.name, t.data.original_name) !== null && (
              <p className="muted">Imported as {renamedFrom(t.data.name, t.data.original_name)}</p>
            )}
          </section>
          {t.data.is_group ? (
            <GroupPage g={t.data} all={all} onReload={load} />
          ) : (
            <>
              <SettingsCard t={t.data} onChange={(next) => setT({ kind: 'ready', data: next })} />
              <CardSettings t={t.data} onReload={load} />
              <RegulationsCard t={t.data} onReload={load} />
              <StoriesSettings t={t.data} onReload={load} />
              <SeasonsCard t={t.data} onReload={load} />
            </>
          )}
        </>
      )}
      </main>
    </>
  );
}
