/** Groups of related tournaments in admin: what the list shows, a group's members, and what can join a group. */

/** Grouped is the part of a tournament grouping reads: its slug, name, the group it is in ("" for none), its type, and
 * whether it is a group. */
export type Grouped = { slug: string; name: string; parent: string; kind: string; is_group: boolean };

/** topLevel returns the tournaments in no group, in the list's order: each group once, or a tournament on its own. */
export function topLevel<T extends Grouped>(list: readonly T[]): T[] {
  return list.filter((t) => t.parent === '');
}

/** membersOf returns the tournaments directly in the group slug, in the list's order. */
export function membersOf<T extends Grouped>(list: readonly T[], slug: string): T[] {
  return list.filter((t) => t.parent === slug);
}

/** unrelated returns the tournaments related to nothing: in no group and holding none. */
export function unrelated<T extends Grouped>(list: readonly T[]): T[] {
  return list.filter((t) => t.parent === '' && !list.some((m) => m.parent === t.slug));
}

/** groupsAbove returns the groups slug is in, outermost first; none for a tournament in no group or not listed. */
export function groupsAbove<T extends Grouped>(list: readonly T[], slug: string): T[] {
  const above: T[] = [];
  let parent = list.find((t) => t.slug === slug)?.parent ?? '';
  while (parent !== '' && !above.some((t) => t.slug === parent)) {
    const group = list.find((t) => t.slug === parent);
    if (group === undefined) break;
    above.unshift(group);
    parent = group.parent;
  }
  return above;
}

/** groupsToJoin returns the groups slug can be put in: not slug, its own group, or one inside it. */
export function groupsToJoin<T extends Grouped>(list: readonly T[], slug: string): T[] {
  const own = list.find((t) => t.slug === slug)?.parent ?? '';
  return list.filter(
    (g) => g.is_group && g.slug !== slug && g.slug !== own && !groupsAbove(list, g.slug).some((above) => above.slug === slug),
  );
}

/**
 * candidatesFor returns what Modify offers the group: its members and what is in no group, never the group or one
 * above it; of the group's type, unless everyType or the group is a series or a group of groups.
 */
export function candidatesFor<T extends Grouped>(list: readonly T[], group: string, everyType: boolean): T[] {
  const g = list.find((t) => t.slug === group);
  if (g === undefined) return [];
  const above = new Set(groupsAbove(list, group).map((t) => t.slug));
  return list.filter(
    (t) =>
      t.slug !== group &&
      !above.has(t.slug) &&
      (t.parent === '' || t.parent === group) &&
      (everyType || g.kind === 'group' || g.kind === 'series' || t.kind === g.kind),
  );
}

/** dateSpan writes the dates of a tournament's games: from first to last, one date, or "no games". */
export function dateSpan(first: string | null, last: string | null): string {
  if (first === null || last === null) return 'no games';
  return first === last ? first : `${first} to ${last}`;
}

/** detail returns a row's description on one line, at most 140 characters: admin's, else CGE's. */
export function detail(t: { description: string; cge_about: string }): string {
  const text = (t.description || t.cge_about).replace(/\s+/g, ' ').trim();
  return text.length > 140 ? `${text.slice(0, 139)}…` : text;
}

/** Played is the part of a row group totals read. */
export type Played = { slug: string; parent: string; is_group: boolean; games: number; first_game: string | null; last_game: string | null };

/** groupTotals adds up the games of every tournament in the group slug, at any depth, with the first and last dates. */
export function groupTotals<T extends Played>(
  list: readonly T[],
  slug: string,
): { games: number; first: string | null; last: string | null } {
  let games = 0;
  let first: string | null = null;
  let last: string | null = null;
  const seen = new Set<string>();
  const walk = (group: string) => {
    for (const t of list) {
      if (t.parent !== group || seen.has(t.slug)) continue;
      seen.add(t.slug);
      if (t.is_group) {
        walk(t.slug);
        continue;
      }
      games += t.games;
      if (t.first_game !== null && (first === null || t.first_game < first)) first = t.first_game;
      if (t.last_game !== null && (last === null || t.last_game > last)) last = t.last_game;
    }
  };
  walk(slug);
  return { games, first, last };
}

/** isRated says whether slug counts as rated: a tournament by its own setting, a group when anything in it is. */
export function isRated<T extends { slug: string; parent: string; is_group: boolean; rated: boolean }>(
  list: readonly T[],
  slug: string,
): boolean {
  const t = list.find((x) => x.slug === slug);
  if (t === undefined) return false;
  if (!t.is_group) return t.rated;
  return list.some((m) => m.parent === slug && isRated(list, m.slug));
}

/** typeLabel names a row's type: a tournament's kind; a group says what it groups. */
export function typeLabel(t: { kind: string; is_group: boolean }): string {
  if (!t.is_group) return t.kind;
  if (t.kind === 'series') return 'series';
  return t.kind === 'group' ? 'group of groups' : `group of ${t.kind}s`;
}

/** rowTotals returns a row's games and dates: a group's are what is in it, a tournament's its own. */
export function rowTotals<T extends Played>(list: readonly T[], t: T): { games: number; first: string | null; last: string | null } {
  return t.is_group ? groupTotals(list, t.slug) : { games: t.games, first: t.first_game, last: t.last_game };
}

/** listed returns the top level that is rated, or that is not. */
export function listed<T extends { slug: string; parent: string; is_group: boolean; rated: boolean }>(list: readonly T[], rated: boolean): T[] {
  return list.filter((t) => t.parent === '' && isRated(list, t.slug) === rated);
}

/** moveChoices returns where slug can move: out of every group ("" first, when it is in one), then each group it can join. */
export function moveChoices<T extends Grouped>(list: readonly T[], slug: string): { parent: string; label: string }[] {
  const inOne = (list.find((t) => t.slug === slug)?.parent ?? '') !== '';
  return [
    ...(inOne ? [{ parent: '', label: 'No group: the top level' }] : []),
    ...groupsToJoin(list, slug).map((g) => ({ parent: g.slug, label: g.name })),
  ];
}

/** toggled returns chosen with slug added, or taken out when it is there. */
export function toggled(chosen: readonly string[], slug: string): string[] {
  return chosen.includes(slug) ? chosen.filter((s) => s !== slug) : [...chosen, slug];
}

type GroupForm<K extends string> = { name: string; kind: K; description: string };

/** groupChange returns what saving a group's settings sends, trimmed, or null when nothing changed or it has no name. */
export function groupChange<K extends string>(saved: GroupForm<K>, form: GroupForm<K>): GroupForm<K> | null {
  const next = { name: form.name.trim(), kind: form.kind, description: form.description.trim() };
  if (next.name === '') return null;
  return next.name === saved.name && next.kind === saved.kind && next.description === saved.description ? null : next;
}

/** newGroupBody returns what making a group sends, or null without a name. */
export function newGroupBody<K extends string>(name: string, kind: K): { name: string; kind: K } | null {
  return name.trim() === '' ? null : { name: name.trim(), kind };
}
