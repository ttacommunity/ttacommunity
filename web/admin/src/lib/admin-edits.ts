/** Whether an admin edit can be saved: what changed, and what is valid to send. */
import type { components } from '@ttac/shared/api';

type Card = components['schemas']['TournamentCard'];
type Story = components['schemas']['Story'];

/** STORIES_MAX is how many story cards a tournament may have. */
export const STORIES_MAX = 12;

/** isDiscordId says whether text is a Discord user id: 5 to 25 digits, spaces aside. */
export function isDiscordId(text: string): boolean {
  return /^[0-9]{5,25}$/.test(text.trim());
}

/** textChanged says whether a box differs from what is saved, spaces at its ends aside. */
export function textChanged(saved: string, text: string): boolean {
  return text.trim() !== saved;
}

/** cardChanged says whether any field of a card differs from what is saved. */
export function cardChanged(saved: Card, card: Card): boolean {
  return (Object.keys(card) as (keyof Card)[]).some((k) => card[k] !== saved[k]);
}

/** regulationsSavable says whether text can be saved: not empty, and not what shows already (ours, else CGE's). */
export function regulationsSavable(text: string, ours: string, cge: string): boolean {
  return !isRemoval(text) && text.trim() !== regulationsStart(ours, cge).trim();
}

/** isRemoval says whether saving text removes what is saved: an empty box removes it. */
export function isRemoval(text: string): boolean {
  return text.trim() === '';
}

/** regulationsStart is the text the regulations box starts with: ours, else CGE's. */
export function regulationsStart(ours: string, cge: string): string {
  return ours === '' ? cge : ours;
}

/** regulationsSource says which regulations the site shows: ours, CGE's, or none. */
export function regulationsSource(ours: string, cge: string): 'ours' | 'cge' | 'none' {
  if (ours !== '') return 'ours';
  return cge !== '' ? 'cge' : 'none';
}

/** storiesChanged says whether the story cards differ from what is saved, order included. */
export function storiesChanged(saved: readonly Story[], list: readonly Story[]): boolean {
  return JSON.stringify(list) !== JSON.stringify(saved);
}

/** movedStory returns list with story i moved by one place (-1 up, 1 down), unchanged past either end. */
export function movedStory(list: readonly Story[], i: number, by: number): Story[] {
  const j = i + by;
  if (j < 0 || j >= list.length) return [...list];
  const next = [...list];
  [next[i], next[j]] = [next[j], next[i]];
  return next;
}

/** editedStory returns list with one field of story i set to value. */
export function editedStory(list: readonly Story[], i: number, field: keyof Story, value: string): Story[] {
  return list.map((s, j) => (j === i ? { ...s, [field]: value } : s));
}

/** removedStory returns list without story i. */
export function removedStory(list: readonly Story[], i: number): Story[] {
  return list.filter((_, j) => j !== i);
}

/** addedStory returns list with an empty story at the end. */
export function addedStory(list: readonly Story[]): Story[] {
  return [...list, { player: '', badge: '', title: '', text: '' }];
}

/** canAddStory says whether another story card fits. */
export function canAddStory(list: readonly Story[]): boolean {
  return list.length < STORIES_MAX;
}

/** badgeChanged says whether a badge's edit can be saved: it has a label, and differs from what is saved. */
export function badgeChanged(saved: { label: string; description: string }, label: string, description: string): boolean {
  return label.trim() !== '' && (label.trim() !== saved.label || description.trim() !== saved.description);
}
