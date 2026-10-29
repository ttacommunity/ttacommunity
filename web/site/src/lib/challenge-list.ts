/** The committed challenge list as the pages built from it read it: each challenge's text, rules and opponents. */
import raw from '../data/challenges.json';
import type { SetId } from './challenges';

export interface Opponent {
  level: 'easy' | 'medium' | 'hard';
  name: string | null;
}

export interface ChallengeInfo {
  id: string;
  set: SetId;
  group: string;
  slot: number;
  difficulty: number;
  players: number;
  name: string;
  description: string[];
  rules: string[];
  opponents: Opponent[];
}

/** CHALLENGE_LIST is every challenge in the game's order. */
export const CHALLENGE_LIST = raw as ChallengeInfo[];

const LEVELS: Record<Opponent['level'], string> = { easy: 'Easy AI', medium: 'Medium AI', hard: 'Top AI' };

/** opponentLabel is an opponent's name, or its level for an unnamed AI. */
export function opponentLabel(o: Opponent): string {
  return o.name ?? LEVELS[o.level];
}

/** previewText is what a link preview says of a challenge: its story, or else its rules. */
export function previewText(c: ChallengeInfo): string {
  return [...(c.description.length > 0 ? c.description : c.rules)].join(' ');
}
