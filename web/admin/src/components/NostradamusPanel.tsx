/** Nostradamus, for admins and organisers: the games, each opening its own page, and a form to make one over a tournament. */
import { useEffect, useState, type FormEvent } from 'react';
import type { components } from '@ttac/shared/api';
import type { Load } from '@ttac/shared/load';
import { adminRequest } from '../lib/admin-api';
import * as req from '../lib/admin-requests';
import { gameHref } from '../lib/nostradamus-admin';
import { outcomeOf, shownAt, type Outcome } from '../lib/outcome';
import Notice, { LoadNotice } from './Notice';

type Editions = components['schemas']['NostradamusEditions'];
type NewGame = components['schemas']['AdminNostradamusEditionNew'];

const EMPTY: NewGame = { slug: '', name: '', series: 'worlds', tournament: '' };

/** NostradamusPanel lists the games, newest first, and makes a new one. */
export default function NostradamusPanel() {
  const [games, setGames] = useState<Load<Editions>>({ kind: 'loading' });
  const [draft, setDraft] = useState<NewGame>(EMPTY);
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const load = async () => {
    const r = await adminRequest<Editions>('/api/nostradamus', {});
    setGames(r.ok ? { kind: 'ready', data: r.data } : { kind: 'error', message: r.message });
  };
  useEffect(() => void load(), []);
  const add = async (e: FormEvent) => {
    e.preventDefault();
    const r = await req.send(req.addNostradamusGame({ ...draft, slug: draft.slug.trim(), name: draft.name.trim(), tournament: draft.tournament.trim() }));
    setOutcome(outcomeOf(r, `Made ${draft.name.trim()}.`, 'new game'));
    if (r.ok) {
      setDraft(EMPTY);
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
            Address name (a-z, 0-9, -)
            <input className="input-antique" value={draft.slug} maxLength={40} pattern="[a-z0-9-]{1,40}" required onChange={(e) => setDraft({ ...draft, slug: e.target.value })} />
          </label>
          <label>
            Series
            <input className="input-antique" value={draft.series} maxLength={100} required onChange={(e) => setDraft({ ...draft, series: e.target.value })} />
          </label>
          <label>
            Season
            <input className="input-antique" value={draft.tournament} maxLength={100} required placeholder="2027" onChange={(e) => setDraft({ ...draft, tournament: e.target.value })} />
          </label>
          <button type="submit" className="btn-antique">
            Make the game
          </button>
        </div>
        <Notice outcome={shownAt(outcome, 'new game')} onClose={() => setOutcome(null)} />
      </form>
    </section>
  );
}
