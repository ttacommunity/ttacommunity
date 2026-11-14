/** Admin's table of tournaments and groups: the main list, a group's members, and Modify's choices. */
import type { ReactNode } from 'react';
import type { AdminTournamentRow } from '../lib/admin-api';
import { dateSpan, detail, membersOf, ratedLabel, rowTotals, typeLabel } from '../lib/tournament-groups';
import { SETTINGS, settingsHref } from '../lib/tournament-page';
import { trackName, trackShort } from '@ttac/shared/tracks';

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
const [SHOWN_SETTING, ...TAIL_SETTINGS] = [SETTINGS[0], ...SETTINGS.filter((s) => s.key !== 'visible' && s.key !== 'rated' && s.key !== 'superblitz_rated')];

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
            <th scope="col" title={`${SHOWN_SETTING.label}: ${SHOWN_SETTING.hint}`} className="col-setting">
              {SHOWN_SETTING.short}
            </th>
            <th scope="col" title={`Rated track: ${trackShort('community')} (${trackName('community')}), ${trackShort('superblitz')} (${trackName('superblitz')}), or ? (unrated)`} className="col-setting">
              Rated
            </th>
            {TAIL_SETTINGS.map((s) => (
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
                <td className="col-setting">
                  {!t.is_group && <YesNo on={t[SHOWN_SETTING.key]} label={SHOWN_SETTING.label} name={t.name} />}
                </td>
                <td className="col-setting">
                  {(() => {
                    const track = ratedLabel(all, t.slug);
                    return (
                      <span className={track === '?' ? 'no' : 'yes'} title={`Rated for ${t.name}: ${track}`} aria-label={`Rated for ${t.name}: ${track}`}>
                        {track}
                      </span>
                    );
                  })()}
                </td>
                {TAIL_SETTINGS.map((s) => (
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
