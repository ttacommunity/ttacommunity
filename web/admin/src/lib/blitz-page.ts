/** The blitz edition page's decisions: the edition the address names, its address, advice on its format, and its points summed up and changed. */
import type { components } from '@ttac/shared/api';
import { pointsList, type BlitzPoints } from '@ttac/shared/blitz';

type Schemas = components['schemas'];
export type AdminBlitzEntrant = Schemas['AdminBlitzEntrant'];

// The API's pattern for a tournament in a path.
const SLUG = /^[a-z][a-z0-9_]{0,63}$/;

/** editionParams returns the tournament slug (?t=) and edition label (?e=) the address names, or null. */
export function editionParams(search: URLSearchParams): { slug: string; edition: string } | null {
  const slug = search.get('t') ?? '';
  const edition = search.get('e') ?? '';
  return SLUG.test(slug) && edition !== '' ? { slug, edition } : null;
}

/** editionHref is the address of an edition's page. */
export function editionHref(slug: string, edition: string): string {
  return `/blitz/?t=${encodeURIComponent(slug)}&e=${encodeURIComponent(edition)}`;
}

/** formatAdvice is the most Swiss rounds the active entrants can always be paired for without a repeat (half of them,
 * rounded down, at least one), and warnings for a format past it or a knockout larger than the field. A warning only:
 * late entrants and wildcards raise the limit. */
export function formatAdvice(format: { swiss_rounds: number; knockout_size: number }, active: number): { limit: number; warnings: string[] } {
  const limit = Math.max(1, Math.floor(active / 2));
  const warnings: string[] = [];
  if (format.swiss_rounds > limit) {
    warnings.push(
      `${format.swiss_rounds} Swiss rounds is more than ${active} active entrants can always be paired for: past ${limit}, someone may have to meet an opponent again, and Pair refuses that. Late entrants or wildcards raise the limit.`,
    );
  }
  if (format.knockout_size > active) warnings.push(`The knockout takes the top ${format.knockout_size}, and ${active} entrants are active.`);
  return { limit, warnings };
}

/** editionFormatWarnings are formatAdvice's warnings for an edition's saved format and active entrants, shown while its Format is hidden: none once the knockout has started, when the format no longer changes. */
export function editionFormatWarnings(ed: { format: { swiss_rounds: number; knockout_size: number }; entrants: readonly { status: string }[]; rounds: readonly { kind: string }[] }): string[] {
  if (ed.rounds.some((r) => r.kind === 'knockout')) return [];
  return formatAdvice(ed.format, ed.entrants.filter((e) => e.status === 'active').length).warnings;
}

/** pointsSummary sums up an edition's points while they are hidden: FIDE's or its own, with every outcome. */
export function pointsSummary(points: BlitzPoints, fide: boolean): string {
  return `${fide ? "FIDE's" : "This edition's"}: ${pointsList(points)}`;
}

/** pointsChanged says whether the points form differs from the saved points, so Save has something to do. */
export function pointsChanged(form: BlitzPoints, saved: BlitzPoints): boolean {
  return (Object.keys(saved) as (keyof BlitzPoints)[]).some((k) => form[k] !== saved[k]);
}

/** PointsForm is the points form's fields, as typed. */
export type PointsForm = Record<keyof BlitzPoints, string>;

/** pointsForm fills the points form with an edition's points. */
export function pointsForm(p: BlitzPoints): PointsForm {
  return { win: String(p.win), draw: String(p.draw), loss: String(p.loss), forfeit_win: String(p.forfeit_win), forfeit_loss: String(p.forfeit_loss), bye: String(p.bye), skip: String(p.skip) };
}

/** pointsFrom reads the points form as the points to save; the API refuses what is out of range. */
export function pointsFrom(f: PointsForm): BlitzPoints {
  return { win: Number(f.win), draw: Number(f.draw), loss: Number(f.loss), forfeit_win: Number(f.forfeit_win), forfeit_loss: Number(f.forfeit_loss), bye: Number(f.bye), skip: Number(f.skip) };
}
