/** The admin page's tabs: which a role sees, and which one the address opens. Rating runs sit in Data. */

export type Role = 'admin' | 'organiser';
export type TabKey = 'tournaments' | 'data' | 'titles' | 'admins';

/** TABS are admin's tabs in order; adminOnly ones an organiser does not see. */
export const TABS: { key: TabKey; label: string; adminOnly: boolean }[] = [
  { key: 'tournaments', label: 'Tournaments', adminOnly: false },
  { key: 'data', label: 'Data', adminOnly: false },
  { key: 'titles', label: 'Titles & badges', adminOnly: true },
  { key: 'admins', label: 'Admins', adminOnly: true },
];

/** tabsFor returns the tabs role sees. */
export function tabsFor(role: Role): typeof TABS {
  return TABS.filter((t) => role === 'admin' || !t.adminOnly);
}

/** tabFrom returns the tab the address's hash names, when role sees it, else Tournaments; #runs, now in Data, opens Data. */
export function tabFrom(hash: string, role: Role): TabKey {
  const named = hash === '#runs' ? '#data' : hash;
  return tabsFor(role).find((t) => `#${t.key}` === named)?.key ?? 'tournaments';
}
