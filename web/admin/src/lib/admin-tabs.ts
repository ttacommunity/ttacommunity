/** The admin page's tabs: which a role sees, and which one the address opens. Rating runs sit in Data. */

export type Role = 'admin' | 'organiser';
export type TabKey = 'tournaments' | 'community' | 'blitz' | 'members' | 'data' | 'titles' | 'admins' | 'audit';

/** TABS are admin's tabs in order; adminOnly ones an organiser does not see. */
export const TABS: { key: TabKey; label: string; adminOnly: boolean }[] = [
  { key: 'tournaments', label: 'Tournaments', adminOnly: false },
  { key: 'community', label: 'Community', adminOnly: true },
  { key: 'blitz', label: 'Blitz', adminOnly: false },
  { key: 'members', label: 'Members', adminOnly: false },
  { key: 'data', label: 'Data', adminOnly: false },
  { key: 'titles', label: 'Titles & badges', adminOnly: true },
  { key: 'admins', label: 'Admins', adminOnly: true },
  { key: 'audit', label: 'Audit', adminOnly: true },
];

/** tabsFor returns the tabs role sees. */
export function tabsFor(role: Role): typeof TABS {
  return TABS.filter((t) => role === 'admin' || !t.adminOnly);
}

// Tabs folded into others: their old addresses open where they went.
const MOVED: Record<string, string> = { '#runs': '#data', '#signins': '#members' };

/** tabFrom returns the tab the address's hash names, when role sees it, else Tournaments; #runs opens Data and #signins Members, where they went. */
export function tabFrom(hash: string, role: Role): TabKey {
  const named = MOVED[hash] ?? hash;
  return tabsFor(role).find((t) => `#${t.key}` === named)?.key ?? 'tournaments';
}

export type Panel = 'tournaments' | 'community' | 'blitz' | 'members' | 'data' | 'runs' | 'standings' | 'titles' | 'badges' | 'admins' | 'audit';

/** panelsFor returns the panels a tab shows role, in order: rating runs only to an admin. */
export function panelsFor(tab: TabKey, role: Role): Panel[] {
  if (tab === 'data') return role === 'admin' ? ['data', 'runs', 'standings'] : ['data', 'standings'];
  if (tab === 'titles') return ['titles', 'badges'];
  return [tab];
}

/** tabCounts are the numbers beside tabs, when there are any: on Members, the requests to be linked waiting for an answer; on Blitz, the players' reports flagged for an organiser. */
export function tabCounts(waiting: number, flagged: number): Partial<Record<TabKey, number>> {
  return { ...(waiting > 0 ? { members: waiting } : {}), ...(flagged > 0 ? { blitz: flagged } : {}) };
}

/** siteUrlFor returns the public Community Hub URL matching the admin host: localhost/127.0.0.1 for local dev, dev.ttacommunity.org for dev admin, else the production hub. */
export function siteUrlFor(hostname: string = ''): string {
  if (hostname === '127.0.0.1' || hostname === 'localhost') return 'http://127.0.0.1:4321';
  if (hostname.startsWith('dev.') || hostname.includes('ttacommunity-dev')) return 'https://dev.ttacommunity.org';
  return 'https://ttacommunity.org';
}
