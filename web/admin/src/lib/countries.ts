/** What a country picker offers: the countries a member may have, found by name or code. */
import type { Country } from '@ttac/shared/countries';
import { matchRank, norm } from './match';

/** rank is how well a country matches the text: a code typed in full as well as its name itself, else by its name. */
function rank(c: Country, typed: string): number | null {
  return norm(typed) === c.code.toLowerCase() ? 0 : matchRank(c.name, typed);
}

/** countryChoices lists up to limit countries matching the text typed, the best first, then by name; none for no text. */
export function countryChoices(countries: readonly Country[], typed: string, limit: number): Country[] {
  if (norm(typed) === '') return [];
  return countries
    .map((c) => ({ c, r: rank(c, typed) }))
    .filter((x): x is { c: Country; r: number } => x.r !== null)
    .sort((a, b) => a.r - b.r || a.c.name.localeCompare(b.c.name))
    .slice(0, limit)
    .map((x) => x.c);
}

/** countryFromText is the code of the country the text names in full, by name or code; '' for no text; null for anything else. */
export function countryFromText(countries: readonly Country[], text: string): string | null {
  const q = norm(text);
  if (q === '') return '';
  return countries.find((c) => c.code.toLowerCase() === q || norm(c.name) === q)?.code ?? null;
}
