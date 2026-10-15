/** What a country picker offers: the countries a member may have, found by name or code. */
import { matchRank, norm } from './match';

/** Country is a country a member may have: its two-letter code and its name in English. */
export type Country = { code: string; name: string };

/** countryNames names each code in English (the code itself where the browser cannot), sorted by name. */
export function countryNames(codes: readonly string[]): Country[] {
  let names: Intl.DisplayNames | null = null;
  try {
    names = new Intl.DisplayNames(['en'], { type: 'region' });
  } catch {
    names = null;
  }
  return codes.map((code) => ({ code, name: names?.of(code) ?? code })).sort((a, b) => a.name.localeCompare(b.name));
}

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
