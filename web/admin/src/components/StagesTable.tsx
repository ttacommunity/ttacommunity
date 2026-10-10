/** A season's stages, with their counts and notes. */
import { Fragment, useState } from 'react';
import type { AdminTournament } from '../lib/admin-api';
import { noteAction, renamedFrom } from '../lib/tournament-page';
import NoteBox from './NoteBox';

const NOTE_BUTTON = { close: 'Close', add: 'Add note', edit: 'Edit note' };

/** StagesTable lists a season's stages with their players, games and notes; a row's button opens its note to edit. */
export default function StagesTable({ slug, season, rows, onReload }: { slug: string; season: string; rows: AdminTournament['seasons'][number]['stages']; onReload: () => Promise<void> }) {
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
