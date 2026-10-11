/** A tournament's settings, each saved at once. */
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournament } from '../lib/admin-api';
import {
  divisionList,
  divisionsChanged,
  divisionText,
  patchReady,
  pointsText,
  levelNamesPatch,
  scoringPatch,
  SETTINGS,
  toggledCounts,
} from '../lib/tournament-page';

/** SettingsCard switches a tournament's settings, saving each at once. */
export default function SettingsCard({ t, onChange }: { t: AdminTournament; onChange: (t: AdminTournament) => void }) {
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
