/** Admin's table of tournaments and groups: the main list, a group's members, and Modify's choices. */
import type { ReactNode } from 'react';
import type { AdminTournamentRow } from '../lib/admin-api';
import { dateSpan, detail, membersOf, rowTotals, typeLabel } from '../lib/tournament-groups';
import { SETTINGS, settingsHref } from '../lib/tournament-page';

/** YesNo marks a setting on or off in the read-only list. */
function YesNo({ on, label, name }: { on: boolean; label: string; name: string }) {
  return (
    <span className={on ? 'yes' : 'no'} title={`${label}: ${on ? 'yes' : 'no'}`} aria-label={`${label} for ${name}: ${on ? 'yes' : 'no'}`}>
      {on ? '✓' : '–'}
    </span>
  );
}

/**
 * TournamentTable lists rows, each name opening it (a group shows its contents' games and dates, from all), with
 * details (a description) when asked, and action(row) in the last column: a Settings link, or Modify's Add and Remove.
 */
export default function TournamentTable({
  rows,
  all,
  caption,
  action,
  details = false,
}: {
  rows: readonly AdminTournamentRow[];
  all: readonly AdminTournamentRow[];
  caption: string;
  action: (t: AdminTournamentRow) => ReactNode;
  details?: boolean;
}) {
  return (
    <div className="table-responsive">
      <table className="leaderboard-table admin-table tournaments-table">
        <caption className="visually-hidden">{caption}</caption>
        <thead>
          <tr>
            <th scope="col">Tournament</th>
            <th scope="col">Type</th>
            <th scope="col">Games</th>
            <th scope="col">Dates</th>
            {SETTINGS.map((s) => (
              <th scope="col" key={s.key} title={`${s.label}: ${s.hint}`} className="col-setting">
                {s.short}
              </th>
            ))}
            <th scope="col" className="col-seasons">
              In it
            </th>
            <th scope="col">
              <span className="visually-hidden">Action</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((t) => {
            const total = rowTotals(all, t);
            return (
              <tr key={t.slug}>
                <th scope="row">
                  <a href={settingsHref(t.slug)}>{t.name}</a>
                  {details && detail(t) !== '' && <span className="row-detail muted">{detail(t)}</span>}
                </th>
                <td>{typeLabel(t)}</td>
                <td>{total.games}</td>
                <td className="row-dates">{dateSpan(total.first, total.last)}</td>
                {SETTINGS.map((s) => (
                  <td key={s.key} className="col-setting">
                    {!t.is_group && <YesNo on={t[s.key]} label={s.label} name={t.name} />}
                  </td>
                ))}
                <td className="col-seasons">{membersOf(all, t.slug).length || ''}</td>
                <td>{action(t)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
