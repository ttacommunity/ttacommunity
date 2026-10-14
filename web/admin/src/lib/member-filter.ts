/** The Members tab's search: which members the list shows for the text typed. */
import type { AdminMember } from './admin-api';
import { matchRank } from './match';

/** membersMatching keeps, in their order, the members matching any of the comma-separated names typed: by username, ratings spelling, Discord name or email as matchRank matches, or by their whole country code; everyone for no text. */
export function membersMatching(members: readonly AdminMember[], typed: string): AdminMember[] {
  const terms = typed
    .split(',')
    .map((t) => t.trim())
    .filter((t) => t !== '');
  if (terms.length === 0) return [...members];
  const matches = (m: AdminMember, t: string) =>
    (m.country ?? '').toLowerCase() === t.toLowerCase() || [m.name, m.player, m.discord_name, m.email].some((f) => f !== '' && matchRank(f, t) !== null);
  return members.filter((m) => terms.some((t) => matches(m, t)));
}
