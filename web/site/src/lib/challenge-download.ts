/** Every challenge entry as the site offers it to download: a row per entry, as CSV for spreadsheets or as JSON. */
import { groupLabel, setLabel, type SetId } from './challenges';

const COLUMNS = ['id', 'date', 'player', 'set', 'group', 'challenge_id', 'challenge', 'players', 'domination', 'verified', 'proof_url', 'bugged'] as const;

export type DownloadRow = Record<(typeof COLUMNS)[number], string | number>;

interface Entry {
  id: number;
  challenge: string;
  player: string;
  domination: number;
  proof_url: string | null;
  entered_at: string;
  bugged: boolean;
}

interface Listed {
  id: string;
  set: SetId;
  group: string;
  name: string;
  players: number;
}

/** downloadRows writes each entry with its challenge in words; an entry of a challenge not listed is left out. */
export function downloadRows(entries: readonly Entry[], challenges: readonly Listed[]): DownloadRow[] {
  const byId = new Map(challenges.map((c) => [c.id, c]));
  return entries.flatMap((e) => {
    const c = byId.get(e.challenge);
    if (c === undefined) return [];
    return [
      {
        id: e.id,
        date: e.entered_at,
        player: e.player,
        set: setLabel(c.set),
        group: groupLabel(c.group),
        challenge_id: c.id,
        challenge: c.name,
        players: c.players,
        domination: e.domination,
        verified: e.proof_url === null ? 'no' : 'yes',
        proof_url: e.proof_url ?? '',
        bugged: e.bugged ? 'yes' : 'no',
      },
    ];
  });
}

/** csvCell writes a value for a CSV cell: a text starting like a formula gets a leading apostrophe, so a spreadsheet shows it rather than runs it. */
function csvCell(value: string | number): string {
  let s = String(value);
  if (typeof value === 'string' && /^[=+\-@\t\r]/.test(s)) s = `'${s}`;
  return /[",\n\r]/.test(s) ? `"${s.replace(/"/g, '""')}"` : s;
}

/** toCsv writes rows as CSV with a header, and a byte-order mark so Excel reads it as UTF-8. */
export function toCsv(rows: readonly DownloadRow[]): string {
  const lines = [COLUMNS.join(','), ...rows.map((r) => COLUMNS.map((k) => csvCell(r[k])).join(','))];
  return `﻿${lines.join('\r\n')}`;
}

/** downloadName is the file's name on a day, with its extension. */
export function downloadName(day: Date, ext: 'csv' | 'json'): string {
  return `tta-challenge-dominations-${day.toISOString().slice(0, 10)}.${ext}`;
}
