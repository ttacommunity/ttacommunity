/** Every change admin sends: each a path and the method and body adminRequest takes, names in paths escaped. */
import type { components } from '@ttac/shared/api';
import { adminRequest, type AdminResult } from './admin-api';

type Schemas = components['schemas'];
type Method = 'POST' | 'PUT' | 'PATCH' | 'DELETE';
export type SeasonFlag = 'rated' | 'champions' | 'community';

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
/** setStories saves the story cards, in order. */
export const setStories = (slug: string, stories: Schemas['Story'][]) => to(tournament(slug), 'PATCH', { stories });
/** addSeason adds a season with no games yet, its label trimmed. */
export const addSeason = (slug: string, label: string) => to(`${tournament(slug)}/seasons`, 'POST', { label: label.trim() });
/** removeSeason removes a season that has no games. */
export const removeSeason = (slug: string, label: string) => to(season(slug, label), 'DELETE');
/** setNote saves a season's note (stage "") or a stage's, trimmed; an empty one removes it. */
export function setNote(slug: string, label: string, stage: string, note: string): Request {
  const change: Schemas['SeasonChange'] = stage === '' ? { note: note.trim() } : { stage_notes: { [stage]: note.trim() } };
  return to(season(slug, label), 'PATCH', change);
}
/** setPenalty sets a season's time-out penalty. */
export const setPenalty = (slug: string, label: string, penalty: number) => to(season(slug, label), 'PATCH', { timeout_penalty: penalty });
/** setPodium sets a season's podium by hand. */
export const setPodium = (slug: string, label: string, podium: Schemas['PodiumSetting']) => to(season(slug, label), 'PATCH', { podium });
/** clearPodium removes the podium set by hand from a season. */
export const clearPodium = (slug: string, label: string) => to(season(slug, label), 'PATCH', { clear_podium: true });

const FLAGS: Record<SeasonFlag, 'rated' | 'in_champions' | 'in_community'> = {
  rated: 'rated',
  champions: 'in_champions',
  community: 'in_community',
};
/** setSeasonFlag turns one of a season's switches on or off: rated, among the champions, in the community leaderboard. */
export const setSeasonFlag = (slug: string, label: string, flag: SeasonFlag, on: boolean) =>
  to(season(slug, label), 'PATCH', { [FLAGS[flag]]: on });

/** setAdmin adds or changes an admin. */
export const setAdmin = (id: string, name: string, role: string) => to(`/api/admins/${id}`, 'PUT', { name, role });
/** removeAdmin removes an admin. */
export const removeAdmin = (id: string) => to(`/api/admins/${id}`, 'DELETE');
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
/** stopRun stops the run that is going. */
export const stopRun = () => to('/api/runs/stop', 'POST');
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
