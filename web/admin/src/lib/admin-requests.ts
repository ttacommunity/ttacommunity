/** Every change admin sends: each a path and the method and body adminRequest takes, names in paths escaped. */
import type { components } from '@ttac/shared/api';
import { adminRequest, type AdminResult } from './admin-api';

type Schemas = components['schemas'];
type Method = 'POST' | 'PUT' | 'PATCH' | 'DELETE';
export type SeasonFlag = 'rated' | 'champions' | 'community' | 'here';

/** Request is one change: where it goes, and how. */
export type Request = { path: string; init: { method: Method; json?: unknown } };

const to = (path: string, method: Method, json?: unknown): Request => ({ path, init: { method, json } });
const tournament = (slug: string) => `/api/tournaments/${slug}`;
const season = (slug: string, label: string) => `${tournament(slug)}/seasons/${encodeURIComponent(label)}`;
type Badge = Pick<Schemas['AdminBadge'], 'id' | 'label' | 'description' | 'players'>;

/** send makes the request and returns its result. */
export function send<T>(r: Request): Promise<AdminResult<T>> {
  return adminRequest<T>(r.path, r.init);
}

/** setTournament changes the settings given. */
export const setTournament = (slug: string, patch: Schemas['TournamentSettings']) => to(tournament(slug), 'PATCH', patch);
/** moveInto puts a tournament or group in the group parent, or out of every group for "". */
export const moveInto = (slug: string, parent: string) => to(tournament(slug), 'PATCH', { parent });
/** newGroup makes a group. */
export const newGroup = (body: Schemas['NewGroup']) => to('/api/tournaments', 'POST', body);
/** setMembers makes members exactly what a group holds. */
export const setMembers = (slug: string, members: string[]) => to(tournament(slug), 'PATCH', { members });
/** setCard saves how the tournament shows on the site. */
export const setCard = (slug: string, card: Schemas['TournamentCard']) => to(tournament(slug), 'PATCH', { card });
/** setRegulations saves the format and regulations text; an empty one shows CGE's again. */
export const setRegulations = (slug: string, text: string) => to(tournament(slug), 'PATCH', { regulations: text });
/** setFormatSummary saves a tournament's format in brief, trimmed; an empty one removes it. */
export const setFormatSummary = (slug: string, text: string) => to(tournament(slug), 'PATCH', { format_summary: text.trim() });
/** setStories saves the story cards, in order. */
export const setStories = (slug: string, stories: Schemas['Story'][]) => to(tournament(slug), 'PATCH', { stories });
/** addSeason adds a season with no games yet, its label trimmed. */
export const addSeason = (slug: string, label: string) => to(`${tournament(slug)}/seasons`, 'POST', { label: label.trim() });
/** removeSeason removes a season that has no games. */
export const removeSeason = (slug: string, label: string) => to(season(slug, label), 'DELETE');
/** moveSeason moves a season to another tournament, optionally with a new season label. */
export const moveSeason = (slug: string, label: string, toTournament: string, newSeason?: string) =>
  to(season(slug, label), 'PATCH', { move_to_tournament: toTournament, ...(newSeason ? { new_season: newSeason } : {}) });
/** setNote saves a season's note (stage "") or a stage's, trimmed; an empty one removes it. */
export function setNote(slug: string, label: string, stage: string, note: string): Request {
  const change: Schemas['SeasonChange'] = stage === '' ? { note: note.trim() } : { stage_notes: { [stage]: note.trim() } };
  return to(season(slug, label), 'PATCH', change);
}
/** setOrganiser saves who organised a season, trimmed; an empty name removes it. */
export const setOrganiser = (slug: string, label: string, name: string) => to(season(slug, label), 'PATCH', { organiser: name.trim() });
/** setPenalty sets a season's time-out penalty. */
export const setPenalty = (slug: string, label: string, penalty: number) => to(season(slug, label), 'PATCH', { timeout_penalty: penalty });
/** setPodium sets a season's podium by hand. */
export const setPodium = (slug: string, label: string, podium: Schemas['PodiumSetting']) => to(season(slug, label), 'PATCH', { podium });
/** clearPodium removes the podium set by hand from a season. */
export const clearPodium = (slug: string, label: string) => to(season(slug, label), 'PATCH', { clear_podium: true });

const FLAGS: Record<SeasonFlag, 'rated' | 'in_champions' | 'in_community' | 'run_here'> = {
  rated: 'rated',
  champions: 'in_champions',
  community: 'in_community',
  here: 'run_here',
};
/** setSeasonFlag turns one of a season's switches on or off: rated, among the champions, in the community leaderboard, run on this site. */
export const setSeasonFlag = (slug: string, label: string, flag: SeasonFlag, on: boolean) =>
  to(season(slug, label), 'PATCH', { [FLAGS[flag]]: on });

const blitz = (slug: string, edition: string) => `/api/blitz/${slug}/${encodeURIComponent(edition)}`;
/** addBlitzEntrants adds entrants to an edition, all or none. */
export const addBlitzEntrants = (slug: string, edition: string, entrants: Schemas['NewBlitzEntrant'][]) =>
  to(`${blitz(slug, edition)}/entrants`, 'POST', { entrants });
/** setBlitzEntrant changes an entrant's Discord name, time zone or status. */
export const setBlitzEntrant = (slug: string, edition: string, id: number, change: Schemas['BlitzEntrantChange']) =>
  to(`${blitz(slug, edition)}/entrants/${id}`, 'PATCH', change);
/** replaceBlitzPlayer puts an entrant in the place of a knockout player who dropped. */
export const replaceBlitzPlayer = (slug: string, edition: string, round: number, match: number, replacement: Schemas['BlitzReplacement']) =>
  to(`${blitz(slug, edition)}/rounds/${round}/matches/${match}/replacement`, 'PUT', replacement);
/** removeBlitzEntrant removes an entrant added by mistake. */
export const removeBlitzEntrant = (slug: string, edition: string, id: number) => to(`${blitz(slug, edition)}/entrants/${id}`, 'DELETE');
/** skipBlitzRound has an entrant sit out the next Swiss round. */
export const skipBlitzRound = (slug: string, edition: string, id: number) => to(`${blitz(slug, edition)}/entrants/${id}/skip`, 'POST');
/** unskipBlitzRound undoes an entrant's skip of a round not yet published. */
export const unskipBlitzRound = (slug: string, edition: string, id: number) => to(`${blitz(slug, edition)}/entrants/${id}/skip`, 'DELETE');
/** resetBlitzResult undoes a match's result in the latest round, game and all. */
export const resetBlitzResult = (slug: string, edition: string, round: number, match: number) =>
  to(`${blitz(slug, edition)}/rounds/${round}/matches/${match}/result`, 'DELETE');
/** resetBlitzRound undoes every result of the latest round, its pairing kept. */
export const resetBlitzRound = (slug: string, edition: string, round: number) => to(`${blitz(slug, edition)}/rounds/${round}/results`, 'DELETE');
/** resetBlitzEdition starts an edition again from its entrants. */
export const resetBlitzEdition = (slug: string, edition: string) => to(`${blitz(slug, edition)}/reset`, 'POST');
/** setBlitzFormat sets an edition's Swiss rounds and knockout size. */
export const setBlitzFormat = (slug: string, edition: string, format: Schemas['BlitzFormat']) => to(blitz(slug, edition), 'PATCH', format);
/** setBlitzPoints sets what each outcome of an edition is worth. */
export const setBlitzPoints = (slug: string, edition: string, points: Schemas['BlitzPoints']) => to(`${blitz(slug, edition)}/points`, 'PUT', points);
/** resetBlitzPoints puts FIDE's points back. */
export const resetBlitzPoints = (slug: string, edition: string) => to(`${blitz(slug, edition)}/points`, 'DELETE');
/** setBlitzPairing sets how an edition's Swiss rounds are paired, round 1 and the rounds after it. */
export const setBlitzPairing = (slug: string, edition: string, pairing: Schemas['BlitzPairingMethods']) => to(`${blitz(slug, edition)}/pairing`, 'PUT', pairing);
/** pairBlitzRound pairs the edition's next round as a draft, by its pairing methods. */
export const pairBlitzRound = (slug: string, edition: string) => to(`${blitz(slug, edition)}/rounds`, 'POST');
/** proposeBlitzPairing asks for a round with the matches in keep (and those with a result) as they are and everyone else paired, without saving it. */
export const proposeBlitzPairing = (slug: string, edition: string, round: number, pairing: Schemas['BlitzPairing']) =>
  to(`${blitz(slug, edition)}/rounds/${round}/pairing`, 'POST', pairing);
/** setBlitzRoundMatches replaces a round's matches, as paired by hand. */
export const setBlitzRoundMatches = (slug: string, edition: string, round: number, matches: Schemas['BlitzRoundMatches']['matches']) =>
  to(`${blitz(slug, edition)}/rounds/${round}/matches`, 'PUT', { matches });
/** publishBlitzRound makes a round public. */
export const publishBlitzRound = (slug: string, edition: string, round: number) => to(`${blitz(slug, edition)}/rounds/${round}/publish`, 'POST');
/** setBlitzRoundDeadline sets the deadline to play a round by, as a day and a time on the clocks of a zone. */
export const setBlitzRoundDeadline = (slug: string, edition: string, round: number, deadline: Schemas['BlitzDeadlineSet']) =>
  to(`${blitz(slug, edition)}/rounds/${round}/deadline`, 'PUT', deadline);
/** blitzDiscordTextPath is where a round's games are read as text to paste into Discord. */
export const blitzDiscordTextPath = (slug: string, edition: string, round: number) => `${blitz(slug, edition)}/rounds/${round}/discord`;
/** deleteBlitzRound removes a draft round. */
export const deleteBlitzRound = (slug: string, edition: string, round: number) => to(`${blitz(slug, edition)}/rounds/${round}`, 'DELETE');
/** recordBlitzGame records, or corrects, the game a match was played as. */
export const recordBlitzGame = (slug: string, edition: string, round: number, match: number, game: Schemas['BlitzGameRecord']) =>
  to(`${blitz(slug, edition)}/rounds/${round}/matches/${match}/game`, 'PUT', game);
/** keepBlitzReport resolves a flagged report by keeping its result. */
export const keepBlitzReport = (slug: string, edition: string, round: number, match: number) => to(`${blitz(slug, edition)}/rounds/${round}/matches/${match}/keep`, 'POST');
/** recordBlitzRuling decides a match by the organisers' ruling. */
export const recordBlitzRuling = (slug: string, edition: string, round: number, match: number, ruling: Schemas['BlitzRuling']) =>
  to(`${blitz(slug, edition)}/rounds/${round}/matches/${match}/ruling`, 'PUT', ruling);

/** addMember adds a member of the community. */
export const addMember = (member: Schemas['NewMember']) => to('/api/members', 'POST', member);
/** setMember changes a member's details. */
export const setMember = (id: number, change: Schemas['MemberChange']) => to(`/api/members/${id}`, 'PATCH', change);
/** removeMember removes a member added by mistake. */
export const removeMember = (id: number) => to(`/api/members/${id}`, 'DELETE');
/** banMember bans a member from taking part, with a reason. */
export const banMember = (id: number, reason: string) => to(`/api/members/${id}/ban`, 'PUT', { reason });
/** unbanMember lifts a member's ban. */
export const unbanMember = (id: number) => to(`/api/members/${id}/ban`, 'DELETE');

/** setAdmin adds or changes an admin. */
export const setAdmin = (id: string, name: string, role: string) => to(`/api/admins/${id}`, 'PUT', { name, role });
/** removeAdmin removes an admin. */
export const removeAdmin = (id: string) => to(`/api/admins/${id}`, 'DELETE');
/** approveLink links a player's sign-in to the member their request names, adding one when there is none. */
export const approveLink = (request: number) => to(`/api/link-requests/${request}/approval`, 'PUT');
/** refuseLink refuses a player's request to be linked, with a reason they see, trimmed. */
export const refuseLink = (request: number, reason: string) => to(`/api/link-requests/${request}/refusal`, 'PUT', { reason: reason.trim() });
/** unlinkAccount unlinks a player's account from its member. */
export const unlinkAccount = (account: number) => to(`/api/accounts/${account}/member`, 'DELETE');
/** removeAccount removes a player's account with every way in, signing the player out everywhere. */
export const removeAccount = (account: number) => to(`/api/accounts/${account}`, 'DELETE');
/** addBadge makes a badge, trimmed. */
export const addBadge = (label: string, description: string) =>
  to('/api/badges', 'POST', { label: label.trim(), description: description.trim() });
/** setBadge changes a badge, trimmed. */
export const setBadge = (id: number, label: string, description: string) =>
  to(`/api/badges/${id}`, 'PUT', { label: label.trim(), description: description.trim() });
/** removeBadge removes a badge. */
export const removeBadge = (id: number) => to(`/api/badges/${id}`, 'DELETE');
const holders = (b: Badge, players: string[]) =>
  to(`/api/badges/${b.id}`, 'PUT', { label: b.label, description: b.description, players });
/** giveBadge gives a saved badge to a player too, its label and description as saved. */
export const giveBadge = (b: Badge, player: string) =>
  holders(b, b.players.includes(player.trim()) ? b.players : [...b.players, player.trim()]);
/** takeBadge takes a saved badge from a player, its label and description as saved. */
export const takeBadge = (b: Badge, player: string) => holders(b, b.players.filter((p) => p !== player));
/** setTitle sets a player's title by hand, with its reason. */
export const setTitle = (player: string, title: string, reason: string) =>
  to(`/api/titles/${encodeURIComponent(player.trim())}`, 'PUT', { title, reason: reason.trim() });
/** clearTitle removes a title set by hand. */
export const clearTitle = (player: string) => to(`/api/titles/${encodeURIComponent(player.trim())}`, 'DELETE');

/** startRun starts a rating run. */
export const startRun = () => to('/api/runs', 'POST');
/** stopRun stops run, which must be the one going. */
export const stopRun = (run: number) => to(`/api/runs/${run}/stop`, 'POST');
/** publishRun makes a run the one the site shows. */
export const publishRun = (run: number) => to(`/api/runs/${run}/publish`, 'POST');
/** protectRun keeps a run from being deleted, or lets it be. */
export const protectRun = (run: number, on: boolean) => to(`/api/runs/${run}`, 'PATCH', { protected: on });
/** deleteRun deletes a run and its results. */
export const deleteRun = (run: number) => to(`/api/runs/${run}`, 'DELETE');

/** createCommunityEdition makes a new community leaderboard edition. */
export const createCommunityEdition = (body: Schemas['CommunityEditionItem']) =>
  to('/api/community/editions', 'POST', body);
/** setCommunityEdition updates an edition of the community leaderboard. */
export const setCommunityEdition = (slug: string, patch: Schemas['CommunityEditionPatch']) =>
  to(`/api/community/editions/${encodeURIComponent(slug)}`, 'PATCH', patch);
