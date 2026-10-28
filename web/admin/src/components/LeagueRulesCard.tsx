/** A league's rules, which admins alone change: its divisions' title tiers, Super GM, and season record games. */
import { useState } from 'react';
import * as req from '../lib/admin-requests';
import type { AdminTournament } from '../lib/admin-api';
import { recordGames, tiersChanged, tiersList, tiersText } from '../lib/league-rules';
import { refusalOf, shownAt, type Outcome } from '../lib/outcome';
import Notice from './Notice';

/** LeagueRulesCard edits a league's rules, saving each at once; a refusal shows under the rule refused. */
export default function LeagueRulesCard({ t, onChange }: { t: AdminTournament; onChange: (t: AdminTournament) => void }) {
  const [refusal, setRefusal] = useState<Outcome | null>(null);
  const notice = (at: string) => <Notice outcome={shownAt(refusal, at)} onClose={() => setRefusal(null)} />;
  const [tiers, setTiers] = useState(tiersText(t.divisions));
  const [games, setGames] = useState(String(t.record_min_games));
  const gamesValue = recordGames(games);
  // Shows the change at once, and puts the tournament back if the API refuses it.
  const patch = async (change: Partial<AdminTournament>, at: string) => {
    setRefusal(null);
    onChange({ ...t, ...change });
    const r = await req.send(req.setTournament(t.slug, change));
    setRefusal(refusalOf(r, at));
    if (!r.ok) onChange(t);
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="league-rules-heading">
      <h2 id="league-rules-heading" className="rating-card-heading">
        League rules
      </h2>
      <p className="muted">Titles and season records use them from the next engine run. Only admins change them.</p>
      <div className="settings-list">
        <div className="setting league-tiers-setting">
          <label htmlFor="league-tiers">
            <strong>Divisions and titles</strong>
            <span className="muted"> · For titles, not the champions table: highest first, each with its title tier from 1 (GM) to 7 (W), such as Emperor 1, King 2</span>
          </label>{' '}
          <input className="input-antique league-tiers-input" id="league-tiers" value={tiers} maxLength={600} onChange={(e) => setTiers(e.target.value)} />{' '}
          <button
            type="button"
            className="btn-antique"
            onClick={() => {
              const list = tiersList(tiers);
              if (list !== null) void patch({ divisions: list }, 'tiers');
            }}
            disabled={!tiersChanged(tiers, t.divisions)}
          >
            Save
          </button>
          {tiersList(tiers) === null && <span className="muted"> A name and a tier from 1 to 7 for each division, each named once</span>}
        </div>
        {notice('tiers')}
        <div className="setting">
          <input id="league-super-gm" type="checkbox" checked={t.super_gm} onChange={(e) => void patch({ super_gm: e.target.checked }, 'super gm')} />
          <label htmlFor="league-super-gm">
            <strong>Super GM</strong>
            <span className="muted"> · A win in its top divisions makes a Super GM</span>
          </label>
        </div>
        {notice('super gm')}
        <div className="setting">
          <label htmlFor="league-record-games">
            <strong>Games for a season record</strong>
            <span className="muted"> · The fewest games a player's season needs to count</span>
          </label>{' '}
          <input className="input-antique" id="league-record-games" value={games} maxLength={3} inputMode="numeric" onChange={(e) => setGames(e.target.value)} />{' '}
          <button
            type="button"
            className="btn-antique"
            onClick={() => {
              if (gamesValue !== null) void patch({ record_min_games: gamesValue }, 'record games');
            }}
            disabled={gamesValue === null || gamesValue === t.record_min_games}
          >
            Save
          </button>
        </div>
        {notice('record games')}
      </div>
    </section>
  );
}
