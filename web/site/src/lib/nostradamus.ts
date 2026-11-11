/** Nostradamus pages' decisions: which edition to show, their addresses, and how names, answers and points read. */

/** editionParam is the edition the address names (?edition=), or null for none. */
export function editionParam(search: string): string | null {
  const edition = (new URLSearchParams(search).get('edition') ?? '').trim();
  return edition === '' ? null : edition;
}

/** chosenEdition is the edition asked for when it is one, else the newest (editions come newest first), or null. */
export function chosenEdition(editions: readonly { slug: string }[], wanted: string | null): string | null {
  if (wanted !== null && editions.some((e) => e.slug === wanted)) return wanted;
  return editions[0]?.slug ?? null;
}

/** nostradamusHref is the address of an edition's standings. */
export function nostradamusHref(edition: string): string {
  return `/nostradamus/?${new URLSearchParams({ edition })}`;
}

/** participantHref is the address of one participant's answers in an edition. */
export function participantHref(edition: string, name: string): string {
  return `/nostradamus/participant/?${new URLSearchParams({ edition, name })}`;
}

/** shownName is the member a participant is linked to, else the name it answered under. */
export function shownName(p: { name: string; member?: string | null }): string {
  return p.member ?? p.name;
}

/** valuesText writes an answer or outcome's names, or says it is not known yet. */
export function valuesText(values: readonly string[] | null | undefined): string {
  return values == null || values.length === 0 ? 'Not known yet' : values.join(', ');
}

/** pointsText writes an answer's points signed, or a dash until they are known. */
export function pointsText(points: number | null | undefined): string {
  if (points == null) return '–';
  if (points > 0) return `+${points}`;
  return points < 0 ? `−${-points}` : '0';
}

/** places gives each total, highest first, its place: a tie shares one, and the next comes after them all. */
export function places(totals: readonly number[]): number[] {
  return totals.map((t, i) => (i > 0 && totals[i - 1] === t ? totals.indexOf(t) + 1 : i + 1));
}
