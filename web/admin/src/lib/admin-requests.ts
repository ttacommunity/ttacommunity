/** Every change admin sends: each a path and the method and body adminRequest takes, names in paths escaped. */
import type { components } from '@ttac/shared/api';
import { adminRequest, type AdminResult } from './admin-api';

type Schemas = components['schemas'];
type Method = 'POST' | 'PUT' | 'PATCH' | 'DELETE';
type SeasonFlag = 'rated' | 'champions' | 'community';

/** Request is one change: where it goes, and how. */
export type Request = { path: string; init: { method: Method; json?: unknown } };

const to = (path: string, method: Method, json?: unknown): Request => ({ path, init: { method, json } });
const tournament = (slug: string) => `/api/tournaments/${slug}`;

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
export const setMembers = (slug: string, members: string[]) => to(`${tournament(slug)}/members`, 'PUT', { members });
/** setNote saves a season's note (stage "") or a stage's, trimmed; an empty one removes it. */
export const setNote = (slug: string, season: string, stage: string, note: string) =>
  to(`${tournament(slug)}/notes`, 'PUT', { season, stage, note: note.trim() });
/** setPenalty sets a season's time-out penalty. */
export const setPenalty = (slug: string, season: string, penalty: number) =>
  to(`${tournament(slug)}/season-penalty`, 'PUT', { season, timeout_penalty: penalty });
/** setPodium sets a season's podium by hand. */
export const setPodium = (slug: string, body: Schemas['SeasonPodium']) => to(`${tournament(slug)}/podium`, 'PUT', body);
/** clearPodium removes the podium set by hand from a season. */
export const clearPodium = (slug: string, season: string) =>
  to(`${tournament(slug)}/podium?season=${encodeURIComponent(season)}`, 'DELETE');
/** setCard saves how the tournament shows on the site. */
export const setCard = (slug: string, card: Schemas['TournamentCard']) => to(`${tournament(slug)}/card`, 'PUT', card);
/** setRegulations saves the format and regulations text; an empty one shows CGE's again. */
export const setRegulations = (slug: string, text: string) => to(`${tournament(slug)}/regulations`, 'PUT', { text });
/** setStories saves the story cards, in order. */
export const setStories = (slug: string, stories: Schemas['Story'][]) => to(`${tournament(slug)}/stories`, 'PUT', { stories });
/** addSeason adds a season with no games yet, its label trimmed. */
export const addSeason = (slug: string, label: string) => to(`${tournament(slug)}/seasons`, 'POST', { label: label.trim() });
/** removeSeason removes a season that has no games. */
export const removeSeason = (slug: string, label: string) =>
  to(`${tournament(slug)}/seasons?label=${encodeURIComponent(label)}`, 'DELETE');

const FLAGS: Record<SeasonFlag, [string, string]> = {
  rated: ['season-rating', 'rated'],
  champions: ['season-champions', 'in_champions'],
  community: ['season-community', 'in_community'],
};
/** setSeasonFlag turns one of a season's switches on or off: rated, among the champions, in the community leaderboard. */
export function setSeasonFlag(slug: string, season: string, flag: SeasonFlag, on: boolean): Request {
  const [path, field] = FLAGS[flag];
  return to(`${tournament(slug)}/${path}`, 'PUT', { season, [field]: on });
}

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
/** giveBadge gives a badge to a player. */
export const giveBadge = (id: number, player: string) => to(`/api/badges/${id}/players/${encodeURIComponent(player.trim())}`, 'PUT');
/** takeBadge takes a badge from a player. */
export const takeBadge = (id: number, player: string) => to(`/api/badges/${id}/players/${encodeURIComponent(player.trim())}`, 'DELETE');
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
export const protectRun = (run: number, on: boolean) => to(`/api/runs/${run}/protected`, 'PUT', { protected: on });
/** deleteRun deletes a run and its results. */
export const deleteRun = (run: number) => to(`/api/runs/${run}`, 'DELETE');
