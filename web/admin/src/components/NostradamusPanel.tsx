/** Nostradamus, for admins and organisers: the games, each opening its own page, and a form to make one over a tournament. */
import { useEffect, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import type { Load } from '@ttac/shared/load';
import { adminRequest, type AdminTournament, type AdminTournamentRow } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { gameHref, gameSlug, seriesChoices } from '../lib/nostradamus-admin';
import { outcomeOf, shownAt, type Outcome } from '../lib/outcome';
import Notice, { LoadNotice } from './Notice';
import PickBox from './PickBox';

type Editions = components['schemas']['NostradamusEditions'];
type NewGame = components['schemas']['AdminNostradamusEditionNew'];

const EMPTY: NewGame = { slug: '', name: '', series: '', tournament: '' };

/** NostradamusPanel lists the games, newest first, and makes a new one. */
export default function NostradamusPanel() {
  const [games, setGames] = useState<Load<Editions>>({ kind: 'loading' });
  const [draft, setDraft] = useState<NewGame>(EMPTY);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [series, setSeries] = useState<AdminTournamentRow[]>([]);
  const [seriesText, setSeriesText] = useState('');
  const [seasons, setSeasons] = useState<string[]>([]);
  const load = async () => {
    const r = await adminRequest<Editions>('/api/nostradamus', {});
    setGames(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  useEffect(() => {
    void load();
    void adminRequest<AdminTournamentRow[]>('/api/tournaments', {}).then((r) => r.ok && setSeries(r.data));
  }, []);
  const pickSeries = async (slug: string, name: string) => {
    setSeriesText(name);
    setDraft((d) => ({ ...d, series: slug, tournament: '' }));
    setSeasons([]);
    const r = await adminRequest<AdminTournament>(`/api/tournaments/${encodeURIComponent(slug)}`, {});
    if (r.ok) setSeasons(r.data.seasons.map((x) => x.label).reverse());
  };
  const slug = draft.slug.trim() === '' ? gameSlug(draft.name) : draft.slug.trim();
  const add = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send(req.addNostradamusGame({ ...draft, slug, name: draft.name.trim() }));
    setOutcome(outcomeOf(r, `Made ${draft.name.trim()}.`, 'new game'));
    if (r.ok) {
      setDraft(EMPTY);
      setSeriesText('');
      setSeasons([]);
      await load();
    }
  };
  return (
    <section className="rating-card admin-card" aria-labelledby="nostradamus-heading">
      <h2 id="nostradamus-heading" className="rating-card-heading">
        Nostradamus
      </h2>
      {games.kind === 'error' && <LoadNotice message={games.message} />}
      {games.kind === 'loading' && <p className="muted">Loading the games…</p>}
      {games.kind === 'ready' &&
        (games.data.editions.length === 0 ? (
          <p className="muted">No game yet.</p>
        ) : (
          <div className="table-responsive">
            <table className="leaderboard-table admin-table">
              <caption className="visually-hidden">Each Nostradamus game and its tournament</caption>
              <thead>
                <tr>
                  <th scope="col">Game</th>
                  <th scope="col">Tournament</th>
                </tr>
              </thead>
              <tbody>
                {games.data.editions.map((g) => (
                  <tr key={g.slug}>
                    <td>
                      <a href={gameHref(g.slug)}>{g.name}</a>
                    </td>
                    <td className="muted">
                      {g.series} {g.tournament}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
      <form className="admin-subsection" onSubmit={(e) => void add(e)}>
        <h3>New game</h3>
        <div className="admin-add">
          <label>
            Name
            <input className="input-antique" value={draft.name} maxLength={100} required onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
          </label>
          <label>
            Address name (optional)
            <input className="input-antique" value={draft.slug} maxLength={40} pattern="[a-z0-9-]{1,40}" placeholder={gameSlug(draft.name) || 'made from the name'}
              onChange={(e) => setDraft({ ...draft, slug: e.target.value })} />
          </label>
          <PickBox label="Series" text={seriesText} placeholder="Search a series" options={seriesChoices(series, seriesText)}
            onText={(t) => { setSeriesText(t); setDraft({ ...draft, series: '', tournament: '' }); setSeasons([]); }}
            onPick={(o) => void pickSeries(o.key, o.label)} />
          <label>
            Season
            <select className="input-antique" required value={draft.tournament} disabled={seasons.length === 0} onChange={(e) => setDraft({ ...draft, tournament: e.target.value })}>
              <option value="">{draft.series === '' ? 'Pick a series first' : seasons.length === 0 ? 'No season yet' : 'Choose a season'}</option>
              {seasons.map((x) => (
                <option key={x} value={x}>
                  {x}
                </option>
              ))}
            </select>
          </label>
          <button type="submit" className="btn-antique" disabled={draft.series === '' || draft.tournament === '' || slug === ''}>
            Make the game
          </button>
        </div>
        <Notice outcome={shownAt(outcome, 'new game')} onClose={() => setOutcome(null)} />
      </form>
    </section>
  );
}
