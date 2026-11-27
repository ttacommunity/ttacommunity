/** Picks a tournament's group: the tournament, then its season, then the season's group (named with its stage), each loaded once chosen. */
import { useEffect, useState } from 'react';
import type { components } from '@ttac/shared/api';
import * as req from '../lib/admin-requests';
import { groupChoices, type PickedGroup } from '../lib/game-changes';

type Row = components['schemas']['AdminTournamentRow'];
type Tournament = components['schemas']['AdminTournament'];
type Stage = components['schemas']['SeasonStage'];

interface Props {
  /** current names the group a game is in now, when editing one, which stands until another is picked. */
  current?: string;
  /** onPick is given the group picked, or null while none is. */
  onPick: (group: PickedGroup | null) => void;
  /** onFailed is given why a list could not be loaded. */
  onFailed: (message: string) => void;
}

export default function GroupPick({ current, onPick, onFailed }: Props) {
  const [rows, setRows] = useState<Row[]>([]);
  const [slug, setSlug] = useState('');
  const [seasons, setSeasons] = useState<string[]>([]);
  const [season, setSeason] = useState('');
  const [groups, setGroups] = useState<ReturnType<typeof groupChoices>>([]);
  const [group, setGroup] = useState('');

  useEffect(() => {
    void req.send<Row[]>(req.tournaments).then((r) => (r.ok ? setRows(r.data.filter((t) => !t.is_group)) : onFailed(r.message)));
  }, []);
  const pickTournament = async (s: string) => {
    setSlug(s);
    setSeasons([]);
    setSeason('');
    setGroups([]);
    setGroup('');
    onPick(null);
    if (s === '') return;
    const r = await req.send<Tournament>(req.adminTournament(s));
    if (r.ok) setSeasons(r.data.seasons.map((x) => x.label));
    else onFailed(r.message);
  };
  const pickSeason = async (label: string) => {
    setSeason(label);
    setGroups([]);
    setGroup('');
    onPick(null);
    if (label === '') return;
    const r = await req.send<Stage[]>(req.seasonGroups(slug, label));
    if (r.ok) setGroups(groupChoices(r.data));
    else onFailed(r.message);
  };
  const pickGroup = (id: string) => {
    setGroup(id);
    const g = groups.find((x) => String(x.id) === id);
    onPick(g === undefined ? null : { id: g.id, next: g.next });
  };

  return (
    <>
    {current !== undefined && <p className="muted">In {current}. To move it, choose another group.</p>}
    <div className="field-row">
      <label>
        Tournament
        <select className="input-antique" value={slug} onChange={(e) => void pickTournament(e.target.value)}>
          <option value="">Choose…</option>
          {rows.map((t) => (
            <option key={t.slug} value={t.slug}>
              {t.name}
            </option>
          ))}
        </select>
      </label>
      <label>
        Season
        <select className="input-antique" value={season} disabled={seasons.length === 0} onChange={(e) => void pickSeason(e.target.value)}>
          <option value="">Choose…</option>
          {seasons.map((s) => (
            <option key={s} value={s}>
              {s}
            </option>
          ))}
        </select>
      </label>
      <label>
        Stage and group
        <select className="input-antique" value={group} disabled={groups.length === 0} onChange={(e) => pickGroup(e.target.value)}>
          <option value="">Choose…</option>
          {groups.map((g) => (
            <option key={g.id} value={g.id}>
              {g.label}
            </option>
          ))}
        </select>
      </label>
    </div>
    </>
  );
}
