/** The shape of a community leaderboard edition: its event columns, categories and tournaments. */
import type { CommunityEdition } from '@ttac/shared/ratings';

export interface EventColumn { id: string; name: string; short: string; maxPts: number; desc?: string }
export interface Category { category: string; badgeClass: string; columns: EventColumn[] }
export interface Tournament { id: string; name: string; short: string; cols: string[]; maxPts: number; badgeClass: string; desc: string }
export interface Edition {
  info: { id: CommunityEdition; name: string; title: string; short: string; badge: string; year: number; wc: string; wcDesc: string };
  categories: Category[];
  summary: Tournament[];
  /** How many best events count toward a player's total in this edition; defaults to 8. */
  countingEvents?: number;
  /** The event columns in the order the maintainer's sheet lists them, which breaks ties among a player's best events. */
  sheetOrder: string[];
}
