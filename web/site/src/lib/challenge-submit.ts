/** Submitting dominations: the form's rows as entries, what is wrong with them, and the notes the review shows. */

/** DISCORD_GUILD is the TtA Discord server: an entry is verified when it links a message there. */
export const DISCORD_GUILD = '358698681672925195';

const MESSAGE = /^https?:\/\/(?:www\.|ptb\.|canary\.)?discord(?:app)?\.com\/channels\/(\d+)\/(\d+)\/(\d+)\/?$/;

/** proofURL writes a Discord message link in the TtA server the one way the api keeps it, '' for none, or null for anything else. */
export function proofURL(raw: string): string | null {
  const s = raw.trim();
  if (s === '') return '';
  const m = MESSAGE.exec(s);
  if (m === null || m[1] !== DISCORD_GUILD) return null;
  return `https://discord.com/channels/${m[1]}/${m[2]}/${m[3]}`;
}

/** Row is what the form holds for a challenge: the domination and proof as typed. */
export interface Row {
  domination: string;
  proof: string;
}

/** Entry is a submitted entry as the api takes it. */
export interface Entry {
  challenge: string;
  domination: number;
  proof_url: string;
}

/** submission turns the filled rows, in the list's order, into entries, a row without its own proof taking shared; errors say what is wrong. */
export function submission(list: readonly { id: string; name: string }[], rows: Record<string, Row>, shared: string): { entries: Entry[]; errors: string[] } {
  const entries: Entry[] = [];
  const errors: string[] = [];
  for (const c of list) {
    const row = rows[c.id];
    if (row === undefined || row.domination.trim() === '') continue;
    const domination = Number(row.domination.trim());
    const proof = proofURL(row.proof.trim() === '' ? shared : row.proof);
    if (!Number.isInteger(domination)) errors.push(`${c.name}: the domination must be a whole number.`);
    else if (domination < -9999 || domination > 9999) errors.push(`${c.name}: the domination must be between -9999 and 9999.`);
    else if (proof === null) errors.push(`${c.name}: the proof must be a message link in the TtA Discord.`);
    else entries.push({ challenge: c.id, domination, proof_url: proof });
  }
  return { entries, errors };
}

/** Note is a remark on an entry under review: good news, or a warning. */
export interface Note {
  kind: 'good' | 'warn';
  text: string;
}

/** An entry under review, with the player's best, the verified record and everyone else's best there (undefined for none), and who else used its proof. */
interface Reviewed {
  domination: number;
  proof: string;
  best: number | undefined;
  record: number | undefined;
  othersBest: number | undefined;
  reusedBy: readonly string[];
}

/** reviewNotes says what an entry changes and what looks like a mistake. */
export function reviewNotes(e: Reviewed): Note[] {
  const notes: Note[] = [];
  if (e.domination < 0) notes.push({ kind: 'warn', text: 'Negative dominations count as 0 in sums and ratings' });
  else if (e.record === undefined || e.domination > e.record) {
    const more = 'Please post more screenshots than the required ones in Discord.';
    notes.push({ kind: 'good', text: e.proof !== '' ? `★ New record! ${more}` : `★ Beats the record (add proof to make it official). ${more}` });
  }
  if (e.best === undefined) notes.push({ kind: 'good', text: 'First entry for this challenge' });
  else if (e.domination > e.best) notes.push({ kind: 'good', text: `↑ +${e.domination - e.best} on your best` });
  else notes.push({ kind: 'warn', text: 'Not higher than your best' });
  if (e.othersBest !== undefined && e.othersBest > 0 && e.domination > e.othersBest * 2) {
    notes.push({ kind: 'warn', text: "More than double everyone else's best. Typo?" });
  }
  if (e.reusedBy.length > 0) notes.push({ kind: 'warn', text: `Proof link already used by ${e.reusedBy.join(', ')}` });
  return notes;
}

interface Made {
  challenge: string;
  player: string;
  domination: number;
  proof_url: string | null;
  bugged: boolean;
}

/** bestsOf gives player's best domination on each challenge they entered, ignoring case and bugged entries; with others, everyone else's best. */
export function bestsOf(entries: readonly Made[], player: string, others = false): Map<string, number> {
  const me = player.toLowerCase();
  const best = new Map<string, number>();
  for (const e of entries) {
    if (e.bugged || (e.player.toLowerCase() === me) === others) continue;
    best.set(e.challenge, Math.max(best.get(e.challenge) ?? -Infinity, e.domination));
  }
  return best;
}

/** proofOwners reads who used each proof: the returned function gives the names other than player that used proof. */
export function proofOwners(entries: readonly { player: string; proof_url: string | null }[]): (proof: string, player: string) => string[] {
  const owners = new Map<string, Set<string>>();
  for (const e of entries) {
    if (e.proof_url === null) continue;
    owners.set(e.proof_url, (owners.get(e.proof_url) ?? new Set()).add(e.player));
  }
  return (proof, player) => [...(owners.get(proof) ?? [])].filter((p) => p.toLowerCase() !== player.toLowerCase());
}
