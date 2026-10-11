/** The community leaderboard's editions as upstream's page lays them out (routes/community.py at cb0c422), and each edition's event columns in its sheet's order. */
import type { CommunityEdition } from '@ttac/shared/ratings';
import type { Edition } from './types';
import { edition as e2026 } from './edition-2026';
import { edition as e2025 } from './edition-2025';
import { edition as e2024 } from './edition-2024';
import { edition as race26 } from './edition-race26';

export type { EventColumn, Category, Tournament, Edition } from './types';

export const EDITIONS: Record<CommunityEdition, Edition> = {
  "2026": e2026,
  "2025": e2025,
  "2024": e2024,
  "race26": race26,
};
